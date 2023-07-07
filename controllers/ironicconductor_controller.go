/*
Copyright 2022 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	k8s_types "k8s.io/apimachinery/pkg/types"

	routev1 "github.com/openshift/api/route/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/go-logr/logr"
	ironicv1 "github.com/openstack-k8s-operators/ironic-operator/api/v1beta1"
	ironic "github.com/openstack-k8s-operators/ironic-operator/pkg/ironic"
	ironicconductor "github.com/openstack-k8s-operators/ironic-operator/pkg/ironicconductor"
	"github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/configmap"
	"github.com/openstack-k8s-operators/lib-common/modules/common/endpoint"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/labels"
	nad "github.com/openstack-k8s-operators/lib-common/modules/common/networkattachment"
	"github.com/openstack-k8s-operators/lib-common/modules/common/pvc"
	common_rbac "github.com/openstack-k8s-operators/lib-common/modules/common/rbac"
	"github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	"github.com/openstack-k8s-operators/lib-common/modules/common/statefulset"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
)

// IronicConductorReconciler reconciles a IronicConductor object
type IronicConductorReconciler struct {
	client.Client
	Kclient kubernetes.Interface
	Log     logr.Logger
	Scheme  *runtime.Scheme
}

// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironicconductors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironicconductors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironicconductors/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;create;update;patch;delete;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;create;update;patch;delete;watch
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;delete;
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;create;update;patch;delete;watch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;create;update;patch;delete;watch
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list
// +kubebuilder:rbac:groups=operator.openshift.io,resources=ingresscontrollers,verbs=get;list
// +kubebuilder:rbac:groups=k8s.cni.cncf.io,resources=network-attachment-definitions,verbs=get;list;watch

// service account, role, rolebinding
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=roles,verbs=get;list;watch;create;update
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=rolebindings,verbs=get;list;watch;create;update
// service account permissions that are needed to grant permission to the above
// +kubebuilder:rbac:groups="security.openshift.io",resourceNames=anyuid;privileged,resources=securitycontextconstraints,verbs=use
// +kubebuilder:rbac:groups="",resources=pods,verbs=create;delete;get;list;patch;update;watch

// Reconcile -
func (r *IronicConductorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, _err error) {
	_ = log.FromContext(ctx)

	// Fetch the IronicConductor instance
	instance := &ironicv1.IronicConductor{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected.
			// For additional cleanup logic use finalizers. Return and don't requeue.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	helper, err := helper.NewHelper(
		instance,
		r.Client,
		r.Kclient,
		r.Scheme,
		r.Log,
	)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Always patch the instance status when exiting this function so we can persist any changes.
	defer func() {
		// update the Ready condition based on the sub conditions
		if instance.Status.Conditions.AllSubConditionIsTrue() {
			instance.Status.Conditions.MarkTrue(
				condition.ReadyCondition, condition.ReadyMessage)
		} else {
			// something is not ready so reset the Ready condition
			instance.Status.Conditions.MarkUnknown(
				condition.ReadyCondition, condition.InitReason, condition.ReadyInitMessage)
			// and recalculate it based on the state of the rest of the conditions
			instance.Status.Conditions.Set(
				instance.Status.Conditions.Mirror(condition.ReadyCondition))
		}
		err := helper.PatchInstance(ctx, instance)
		if err != nil {
			_err = err
			return
		}
	}()

	// If we're not deleting this and the service object doesn't have our finalizer, add it.
	if instance.DeletionTimestamp.IsZero() && controllerutil.AddFinalizer(instance, helper.GetFinalizer()) {
		return ctrl.Result{}, nil
	}

	//
	// initialize status
	//
	if instance.Status.Conditions == nil {
		instance.Status.Conditions = condition.Conditions{}
		// initialize conditions used later as Status=Unknown
		cl := condition.CreateList(
			condition.UnknownCondition(condition.InputReadyCondition, condition.InitReason, condition.InputReadyInitMessage),
			condition.UnknownCondition(condition.ServiceConfigReadyCondition, condition.InitReason, condition.ServiceConfigReadyInitMessage),
			condition.UnknownCondition(condition.DeploymentReadyCondition, condition.InitReason, condition.DeploymentReadyInitMessage),
			condition.UnknownCondition(condition.NetworkAttachmentsReadyCondition, condition.InitReason, condition.NetworkAttachmentsReadyInitMessage),
			// service account, role, rolebinding conditions
			condition.UnknownCondition(condition.ServiceAccountReadyCondition, condition.InitReason, condition.ServiceAccountReadyInitMessage),
			condition.UnknownCondition(condition.RoleReadyCondition, condition.InitReason, condition.RoleReadyInitMessage),
			condition.UnknownCondition(condition.RoleBindingReadyCondition, condition.InitReason, condition.RoleBindingReadyInitMessage),
		)

		instance.Status.Conditions.Init(&cl)

		// Register overall status immediately to have an early feedback e.g. in the cli
		return ctrl.Result{}, nil
	}
	if instance.Status.Hash == nil {
		instance.Status.Hash = make(map[string]string)
	}
	if instance.Status.NetworkAttachments == nil {
		instance.Status.NetworkAttachments = map[string][]string{}
	}

	// Define a new PVC object
	// TODO: Once conditions added to PVC lib-common logic, handle
	//       the returned condition here
	pvc := pvc.NewPvc(
		ironicconductor.Pvc(instance),
		5,
	)

	ctrlResult, err := pvc.CreateOrPatch(ctx, helper)

	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}
	// End PVC creation/patch

	// Handle service delete
	if !instance.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, instance, helper)
	}

	// Handle non-deleted clusters
	return r.reconcileNormal(ctx, instance, helper)
}

// SetupWithManager sets up the controller with the Manager.
func (r *IronicConductorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// watch for configmap where the CM owner label AND the CR.Spec.ManagingCrName label matches
	configMapFn := func(o client.Object) []reconcile.Request {
		result := []reconcile.Request{}

		// get all API CRs
		apis := &ironicv1.IronicConductorList{}
		listOpts := []client.ListOption{
			client.InNamespace(o.GetNamespace()),
		}
		if err := r.Client.List(context.Background(), apis, listOpts...); err != nil {
			r.Log.Error(err, "Unable to retrieve API CRs %v")
			return nil
		}

		label := o.GetLabels()
		// TODO: Just trying to verify that the CM is owned by this CR's managing CR
		if l, ok := label[labels.GetOwnerNameLabelSelector(labels.GetGroupLabel(ironic.ServiceName))]; ok {
			for _, cr := range apis.Items {
				// return reconcil event for the CR where the CM owner label AND the parentIronicName matches
				if l == ironicv1.GetOwningIronicName(&cr) {
					// return namespace and Name of CR
					name := client.ObjectKey{
						Namespace: o.GetNamespace(),
						Name:      cr.Name,
					}
					r.Log.Info(fmt.Sprintf("ConfigMap object %s and CR %s marked with label: %s", o.GetName(), cr.Name, l))
					result = append(result, reconcile.Request{NamespacedName: name})
				}
			}
		}
		if len(result) > 0 {
			return result
		}
		return nil
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&ironicv1.IronicConductor{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Secret{}).
		Owns(&routev1.Route{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		// watch the config CMs we don't own
		Watches(&source.Kind{Type: &corev1.ConfigMap{}},
			handler.EnqueueRequestsFromMapFunc(configMapFn)).
		Complete(r)
}

func (r *IronicConductorReconciler) reconcileDelete(ctx context.Context, instance *ironicv1.IronicConductor, helper *helper.Helper) (ctrl.Result, error) {
	r.Log.Info("Reconciling Conductor delete")

	// Service is deleted so remove the finalizer.
	controllerutil.RemoveFinalizer(instance, helper.GetFinalizer())
	r.Log.Info("Reconciled Conductor delete successfully")

	return ctrl.Result{}, nil
}

func (r *IronicConductorReconciler) reconcileServices(
	ctx context.Context,
	instance *ironicv1.IronicConductor,
	helper *helper.Helper,
	serviceLabels map[string]string,
) (ctrl.Result, error) {
	r.Log.Info("Reconciling Conductor Services")

	podList, err := ironicconductor.ConductorPods(ctx, instance, helper, serviceLabels)
	if err != nil {
		return ctrl.Result{}, err
	}

	for _, conductorPod := range podList.Items {
		//
		// Create the conductor pod service if none exists
		//
		conductorServiceLabels := map[string]string{
			common.AppSelector:            ironic.ServiceName,
			common.ComponentSelector:      ironic.ConductorComponent,
			ironic.ConductorGroupSelector: ironicv1.ConductorGroupNull,
		}
		if instance.Spec.ConductorGroup != "" {
			conductorServiceLabels[ironic.ConductorGroupSelector] = strings.ToLower(instance.Spec.ConductorGroup)
		}
		conductorService := ironicconductor.Service(conductorPod.Name, instance, conductorServiceLabels)
		if conductorService != nil {
			err = controllerutil.SetOwnerReference(&conductorPod, conductorService, helper.GetScheme())
			if err != nil {
				return ctrl.Result{}, err
			}
			err = r.Get(
				ctx,
				k8s_types.NamespacedName{
					Name:      conductorService.Name,
					Namespace: conductorService.Namespace,
				},
				conductorService,
			)
			if err != nil && k8s_errors.IsNotFound(err) {
				r.Log.Info(fmt.Sprintf("Service port %s does not exist, creating it", conductorService.Name))
				err = r.Create(ctx, conductorService)
				if err != nil {
					return ctrl.Result{}, err
				}
			} else {
				r.Log.Info(fmt.Sprintf("Service port %s exists, updating it", conductorService.Name))
				err = r.Update(ctx, conductorService)
				if err != nil {
					return ctrl.Result{}, err
				}
			}
		}
		// create service - end

		if instance.Spec.ProvisionNetwork == "" {
			//
			// Create the conductor pod route to enable traffic to the
			// httpboot service, only when there is no provisioning network
			//
			conductorRouteLabels := map[string]string{
				common.AppSelector:            ironic.ServiceName,
				common.ComponentSelector:      ironic.HttpbootComponent,
				ironic.ConductorGroupSelector: ironicv1.ConductorGroupNull,
			}
			if instance.Spec.ConductorGroup != "" {
				conductorRouteLabels[ironic.ConductorGroupSelector] = strings.ToLower(instance.Spec.ConductorGroup)
			}

			conductorRoute := ironicconductor.Route(conductorPod.Name, instance, conductorRouteLabels)
			err = controllerutil.SetOwnerReference(&conductorPod, conductorRoute, helper.GetScheme())
			if err != nil {
				return ctrl.Result{}, err
			}
			err = r.Get(
				ctx,
				k8s_types.NamespacedName{
					Name:      conductorRoute.Name,
					Namespace: conductorRoute.Namespace,
				},
				conductorRoute,
			)
			if err != nil && k8s_errors.IsNotFound(err) {
				r.Log.Info(fmt.Sprintf("Route %s does not exist, creating it", conductorRoute.Name))
				err = r.Create(ctx, conductorRoute)
				if err != nil {
					return ctrl.Result{}, err
				}
			} else {
				r.Log.Info(fmt.Sprintf("Route %s exists, updating it", conductorRoute.Name))
				err = r.Update(ctx, conductorRoute)
				if err != nil {
					return ctrl.Result{}, err
				}
			}
		}
	}

	//
	// create users and endpoints
	// TODO: rework this
	//

	r.Log.Info("Reconciled Conductor Services successfully")
	return ctrl.Result{}, nil
}

func (r *IronicConductorReconciler) reconcileNormal(ctx context.Context, instance *ironicv1.IronicConductor, helper *helper.Helper) (ctrl.Result, error) {
	r.Log.Info("Reconciling Conductor")

	if ironicv1.GetOwningIronicName(instance) == "" {
		// Service account, role, binding
		rbacResult, err := common_rbac.ReconcileRbac(ctx, helper, instance, getCommonRbacRules())
		if err != nil {
			return rbacResult, err
		} else if (rbacResult != ctrl.Result{}) {
			return rbacResult, nil
		}
	} else {
		// TODO(hjensas): Mirror conditions from parent, or check resource exist first
		instance.RbacConditionsSet(condition.TrueCondition(
			condition.ServiceAccountReadyCondition,
			condition.ServiceAccountReadyMessage))
		instance.RbacConditionsSet(condition.TrueCondition(
			condition.RoleReadyCondition,
			condition.RoleReadyMessage))
		instance.RbacConditionsSet(condition.TrueCondition(
			condition.RoleBindingReadyCondition,
			condition.RoleBindingReadyMessage))
	}

	// ConfigMap
	configMapVars := make(map[string]env.Setter)

	//
	// check for required OpenStack secret holding passwords for service/admin user and add hash to the vars map
	//
	ospSecret, hash, err := secret.GetSecret(ctx, helper, instance.Spec.Secret, instance.Namespace)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.InputReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				condition.InputReadyWaitingMessage))
			return ctrl.Result{RequeueAfter: time.Second * 10}, fmt.Errorf("OpenStack secret %s not found", instance.Spec.Secret)
		}
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.InputReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.InputReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	configMapVars[ospSecret.Name] = env.SetValue(hash)
	// run check OpenStack secret - end

	//
	// check for required TransportURL secret holding transport URL string
	//
	if instance.Spec.RPCTransport == "oslo" {
		transportURLSecret, hash, err := secret.GetSecret(ctx, helper, instance.Spec.TransportURLSecret, instance.Namespace)
		if err != nil {
			if k8s_errors.IsNotFound(err) {
				instance.Status.Conditions.Set(condition.FalseCondition(
					condition.InputReadyCondition,
					condition.RequestedReason,
					condition.SeverityInfo,
					condition.InputReadyWaitingMessage))
				return ctrl.Result{RequeueAfter: time.Second * 10}, fmt.Errorf("TransportURL secret %s not found", instance.Spec.TransportURLSecret)
			}
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.InputReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.InputReadyErrorMessage,
				err.Error()))
			return ctrl.Result{}, err
		}
		configMapVars[transportURLSecret.Name] = env.SetValue(hash)
	}
	// run check TransportURL secret - end

	instance.Status.Conditions.MarkTrue(condition.InputReadyCondition, condition.InputReadyMessage)

	//
	// Create ConfigMaps required as input for the Service and calculate an overall hash of hashes
	//

	//
	// create custom Configmap for this ironic volume service
	//
	err = r.generateServiceConfigMaps(ctx, helper, instance, &configMapVars)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ServiceConfigReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ServiceConfigReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	// Create ConfigMaps - end

	//
	// create hash over all the different input resources to identify if any those changed
	// and a restart/recreate is required.
	//
	inputHash, hashChanged, err := r.createHashOfInputHashes(ctx, instance, configMapVars)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ServiceConfigReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ServiceConfigReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	} else if hashChanged {
		// Hash changed and instance status should be updated (which will be done by main defer func),
		// so we need to return and reconcile again
		return ctrl.Result{}, nil
	}
	instance.Status.Conditions.MarkTrue(condition.ServiceConfigReadyCondition, condition.ServiceConfigReadyMessage)
	// Create ConfigMaps and Secrets - end

	//
	// TODO check when/if Init, Update, or Upgrade should/could be skipped
	//

	// Handle service update
	ctrlResult, err := r.reconcileUpdate(ctx, instance, helper)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// Handle service upgrade
	ctrlResult, err = r.reconcileUpgrade(ctx, instance, helper)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	//
	// normal reconcile tasks
	//

	serviceLabels := map[string]string{
		common.AppSelector:            ironic.ServiceName,
		common.ComponentSelector:      ironic.ConductorComponent,
		ironic.ConductorGroupSelector: ironicv1.ConductorGroupNull,
	}
	if instance.Spec.ConductorGroup != "" {
		serviceLabels[ironic.ConductorGroupSelector] = strings.ToLower(instance.Spec.ConductorGroup)
	}

	// networks to attach to
	for _, netAtt := range instance.Spec.NetworkAttachments {
		_, err := nad.GetNADWithName(ctx, helper, netAtt, instance.Namespace)
		if err != nil {
			if k8s_errors.IsNotFound(err) {
				instance.Status.Conditions.Set(condition.FalseCondition(
					condition.NetworkAttachmentsReadyCondition,
					condition.RequestedReason,
					condition.SeverityInfo,
					condition.NetworkAttachmentsReadyWaitingMessage,
					netAtt))
				return ctrl.Result{RequeueAfter: time.Second * 10}, fmt.Errorf("network-attachment-definition %s not found", netAtt)
			}
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.NetworkAttachmentsReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.NetworkAttachmentsReadyErrorMessage,
				err.Error()))
			return ctrl.Result{}, err
		}
	}

	serviceAnnotations, err := nad.CreateNetworksAnnotation(instance.Namespace, instance.Spec.NetworkAttachments)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed create network annotation from %s: %w",
			instance.Spec.NetworkAttachments, err)
	}

	ingressDomain, err := ironic.GetIngressDomain(ctx, helper)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Define a new StatefulSet object
	ss := statefulset.NewStatefulSet(
		ironicconductor.StatefulSet(instance, inputHash, serviceLabels, ingressDomain, serviceAnnotations),
		time.Duration(5)*time.Second,
	)

	ctrlResult, err = ss.CreateOrPatch(ctx, helper)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DeploymentReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DeploymentReadyErrorMessage,
			err.Error()))
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DeploymentReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DeploymentReadyRunningMessage))
		return ctrlResult, nil
	}
	// create StatefulSet - end

	// Handle service init
	ctrlResult, err = r.reconcileServices(ctx, instance, helper, serviceLabels)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	instance.Status.ReadyCount = ss.GetStatefulSet().Status.ReadyReplicas

	// verify if network attachment matches expectations
	networkReady, networkAttachmentStatus, err := nad.VerifyNetworkStatusFromAnnotation(ctx, helper, instance.Spec.NetworkAttachments, serviceLabels, instance.Status.ReadyCount)
	if err != nil {
		return ctrl.Result{}, err
	}

	instance.Status.NetworkAttachments = networkAttachmentStatus
	if networkReady {
		instance.Status.Conditions.MarkTrue(condition.NetworkAttachmentsReadyCondition, condition.NetworkAttachmentsReadyMessage)
	} else {
		err := fmt.Errorf("not all pods have interfaces with ips as configured in NetworkAttachments: %s", instance.Spec.NetworkAttachments)
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.NetworkAttachmentsReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.NetworkAttachmentsReadyErrorMessage,
			err.Error()))

		return ctrl.Result{}, err
	}

	if instance.Status.ReadyCount > 0 {
		instance.Status.Conditions.MarkTrue(condition.DeploymentReadyCondition, condition.DeploymentReadyMessage)
	}

	r.Log.Info("Reconciled Conductor successfully")
	return ctrl.Result{}, nil
}

func (r *IronicConductorReconciler) reconcileUpdate(ctx context.Context, instance *ironicv1.IronicConductor, helper *helper.Helper) (ctrl.Result, error) {
	// r.Log.Info("Reconciling Service update")

	// r.Log.Info("Reconciled Service update successfully")
	return ctrl.Result{}, nil
}

func (r *IronicConductorReconciler) reconcileUpgrade(ctx context.Context, instance *ironicv1.IronicConductor, helper *helper.Helper) (ctrl.Result, error) {
	// r.Log.Info("Reconciling Service upgrade")

	// r.Log.Info("Reconciled Service upgrade successfully")
	return ctrl.Result{}, nil
}

// generateServiceConfigMaps - create custom configmap to hold service-specific config
// TODO add DefaultConfigOverwrite
func (r *IronicConductorReconciler) generateServiceConfigMaps(
	ctx context.Context,
	h *helper.Helper,
	instance *ironicv1.IronicConductor,
	envVars *map[string]env.Setter,
) error {
	//
	// create custom Configmap for ironic-conductor-specific config input
	// - %-config-data configmap holding custom config for the service's ironic.conf
	//

	cmLabels := labels.GetLabels(instance, labels.GetGroupLabel(ironic.ServiceName), map[string]string{})

	// customData hold any customization for the service.
	// custom.conf is going to be merged into /etc/ironic/ironic.conf
	// TODO: make sure custom.conf can not be overwritten
	customData := map[string]string{common.CustomServiceConfigFileName: instance.Spec.CustomServiceConfig}

	for key, data := range instance.Spec.DefaultConfigOverwrite {
		customData[key] = data
	}

	customData[common.CustomServiceConfigFileName] = instance.Spec.CustomServiceConfig

	templateParameters := make(map[string]interface{})
	if !instance.Spec.Standalone {
		templateParameters["KeystoneInternalURL"] = instance.Spec.KeystoneVars["keystoneInternalURL"]
		templateParameters["KeystonePublicURL"] = instance.Spec.KeystoneVars["keystonePublicURL"]
		templateParameters["ServiceUser"] = instance.Spec.ServiceUser
	} else {
		ironicAPI, err := ironicv1.GetIronicAPI(
			ctx, h, instance.Namespace, map[string]string{})
		if err != nil {
			return err
		}
		ironicPublicURL, err := ironicAPI.GetEndpoint(endpoint.EndpointPublic)
		if err != nil {
			return err
		}
		templateParameters["IronicPublicURL"] = ironicPublicURL
	}
	dhcpRanges, err := ironic.PrefixOrNetmaskFromCIDR(instance.Spec.DHCPRanges)
	if err != nil {
		r.Log.Error(err, "Failed to get Prefix or Netmask from IP network Prefix (CIDR)")
	}
	templateParameters["DHCPRanges"] = dhcpRanges
	templateParameters["Standalone"] = instance.Spec.Standalone
	templateParameters["ConductorGroup"] = instance.Spec.ConductorGroup
	templateParameters["LogPath"] = ironicconductor.LogPath

	cms := []util.Template{
		// Scripts ConfigMap
		{
			Name:         fmt.Sprintf("%s-scripts", instance.Name),
			Namespace:    instance.Namespace,
			Type:         util.TemplateTypeScripts,
			InstanceType: instance.Kind,
			AdditionalTemplate: map[string]string{
				"common.sh":  "/common/bin/common.sh",
				"get_net_ip": "/common/bin/get_net_ip",
				"init.sh":    "/common/bin/ironic-init.sh",
			},
			Labels: cmLabels,
		},
		// Custom ConfigMap
		{
			Name:          fmt.Sprintf("%s-config-data", instance.Name),
			Namespace:     instance.Namespace,
			Type:          util.TemplateTypeConfig,
			InstanceType:  instance.Kind,
			CustomData:    customData,
			ConfigOptions: templateParameters,
			AdditionalTemplate: map[string]string{
				"ironic.conf":  "/common/config/ironic.conf",
				"dnsmasq.conf": "/common/config/dnsmasq.conf",
			},
			Labels: cmLabels,
		},
	}

	return configmap.EnsureConfigMaps(ctx, h, instance, cms, envVars)
}

// createHashOfInputHashes - creates a hash of hashes which gets added to the resources which requires a restart
// if any of the input resources change, like configs, passwords, ...
//
// returns the hash, whether the hash changed (as a bool) and any error
func (r *IronicConductorReconciler) createHashOfInputHashes(
	ctx context.Context,
	instance *ironicv1.IronicConductor,
	envVars map[string]env.Setter,
) (string, bool, error) {
	var hashMap map[string]string
	changed := false
	mergedMapVars := env.MergeEnvs([]corev1.EnvVar{}, envVars)
	hash, err := util.ObjectHash(mergedMapVars)
	if err != nil {
		return hash, changed, err
	}
	if hashMap, changed = util.SetHash(instance.Status.Hash, common.InputHashName, hash); changed {
		instance.Status.Hash = hashMap
		r.Log.Info(fmt.Sprintf("Input maps hash %s - %s", common.InputHashName, hash))
	}
	return hash, changed, nil
}
