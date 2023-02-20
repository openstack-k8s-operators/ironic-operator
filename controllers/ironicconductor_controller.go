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

	routev1 "github.com/openshift/api/route/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/labels"
	"github.com/openstack-k8s-operators/lib-common/modules/common/pvc"
	"github.com/openstack-k8s-operators/lib-common/modules/common/route"
	"github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
	"github.com/openstack-k8s-operators/lib-common/modules/common/statefulset"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
)

// GetClient -
func (r *IronicConductorReconciler) GetClient() client.Client {
	return r.Client
}

// GetKClient -
func (r *IronicConductorReconciler) GetKClient() kubernetes.Interface {
	return r.Kclient
}

// GetLogger -
func (r *IronicConductorReconciler) GetLogger() logr.Logger {
	return r.Log
}

// GetScheme -
func (r *IronicConductorReconciler) GetScheme() *runtime.Scheme {
	return r.Scheme
}

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
		// update the overall status condition if service is ready
		if instance.IsReady() {
			instance.Status.Conditions.MarkTrue(condition.ReadyCondition, condition.ReadyMessage)
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
			condition.UnknownCondition(condition.ExposeServiceReadyCondition, condition.InitReason, condition.ExposeServiceReadyInitMessage),
			condition.UnknownCondition(condition.InputReadyCondition, condition.InitReason, condition.InputReadyInitMessage),
			condition.UnknownCondition(condition.ServiceConfigReadyCondition, condition.InitReason, condition.ServiceConfigReadyInitMessage),
			condition.UnknownCondition(condition.DeploymentReadyCondition, condition.InitReason, condition.DeploymentReadyInitMessage),
		)

		if !instance.Spec.Standalone {
			// right now we have no dedicated KeystoneServiceReadyInitMessage
			cl = append(cl, *condition.UnknownCondition(condition.KeystoneServiceReadyCondition, condition.InitReason, ""))
		}

		instance.Status.Conditions.Init(&cl)

		// Register overall status immediately to have an early feedback e.g. in the cli
		return ctrl.Result{}, nil
	}
	if instance.Status.Hash == nil {
		instance.Status.Hash = make(map[string]string)
	}
	// TODO: We don't use ServiceIDs in conductor controller
	if instance.Status.ServiceIDs == nil {
		instance.Status.ServiceIDs = make(map[string]string)
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
				if l == ironic.GetOwningIronicName(&cr) {
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
			ironic.ComponentSelector:      ironic.ConductorComponent,
			ironic.ConductorGroupSelector: ironicv1.ConductorGroupNull,
		}
		if instance.Spec.ConductorGroup != "" {
			conductorServiceLabels[ironic.ConductorGroupSelector] = strings.ToLower(instance.Spec.ConductorGroup)
		}
		conductorService := ironicconductor.Service(conductorPod.Name, instance, conductorServiceLabels)
		if conductorService != nil {
			svc := service.NewService(conductorService, conductorServiceLabels, 5)
			ctrlResult, err := svc.CreateOrPatch(ctx, helper)
			if err != nil {
				return ctrl.Result{}, err
			} else if (ctrlResult != ctrl.Result{}) {
				return ctrl.Result{}, nil
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
				ironic.ComponentSelector:      ironic.HttpbootComponent,
				ironic.ConductorGroupSelector: ironicv1.ConductorGroupNull,
			}
			if instance.Spec.ConductorGroup != "" {
				conductorRouteLabels[ironic.ConductorGroupSelector] = strings.ToLower(instance.Spec.ConductorGroup)
			}
			route := route.NewRoute(
				ironicconductor.Route(conductorPod.Name, instance, conductorRouteLabels),
				conductorRouteLabels,
				5,
			)
			_, err := route.CreateOrPatch(ctx, helper)
			if err != nil {
				return ctrl.Result{}, err
			}
			// create service - end
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

	//
	// check for required Ironic config maps that should have been created by parent Ironic CR
	//

	parentIronicName := ironic.GetOwningIronicName(instance)

	configMaps := []string{
		fmt.Sprintf("%s-scripts", parentIronicName),     //ScriptsConfigMap
		fmt.Sprintf("%s-config-data", parentIronicName), //ConfigMap
	}

	_, err = configmap.GetConfigMaps(ctx, helper, instance, configMaps, instance.Namespace, &configMapVars)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.InputReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				condition.InputReadyWaitingMessage))
			return ctrl.Result{RequeueAfter: time.Second * 10}, fmt.Errorf("could not find all config maps for parent Ironic CR %s", parentIronicName)
		}
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.InputReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.InputReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	instance.Status.Conditions.MarkTrue(condition.InputReadyCondition, condition.InputReadyMessage)
	// run check parent Ironic CR config maps - end

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
		ironic.ComponentSelector:      ironic.ConductorComponent,
		ironic.ConductorGroupSelector: ironicv1.ConductorGroupNull,
	}
	if instance.Spec.ConductorGroup != "" {
		serviceLabels[ironic.ConductorGroupSelector] = strings.ToLower(instance.Spec.ConductorGroup)
	}

	ingressDomain, err := ironic.GetIngressDomain(ctx, helper)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Define a new StatefulSet object
	ssDef, err := ironicconductor.StatefulSet(instance, inputHash, serviceLabels, ingressDomain)
	if err != nil {
		return ctrl.Result{}, err
	}
	ss := statefulset.NewStatefulSet(ssDef, 5)

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
	if instance.Status.ReadyCount > 0 {
		instance.Status.Conditions.MarkTrue(condition.DeploymentReadyCondition, condition.DeploymentReadyMessage)
	}
	instance.Status.Networks = instance.Spec.NetworkAttachments

	r.Log.Info("Reconciled Conductor successfully")
	return ctrl.Result{}, nil
}

func (r *IronicConductorReconciler) reconcileUpdate(ctx context.Context, instance *ironicv1.IronicConductor, helper *helper.Helper) (ctrl.Result, error) {
	// r.Log.Info("Reconciling Service update")

	// TODO: should have minor update tasks if required
	// - delete dbsync hash from status to rerun it?

	// r.Log.Info("Reconciled Service update successfully")
	return ctrl.Result{}, nil
}

func (r *IronicConductorReconciler) reconcileUpgrade(ctx context.Context, instance *ironicv1.IronicConductor, helper *helper.Helper) (ctrl.Result, error) {
	// r.Log.Info("Reconciling Service upgrade")

	// TODO: should have major version upgrade tasks
	// -delete dbsync hash from status to rerun it?

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
	// create custom Configmap for ironic-api-specific config input
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
	}
	dhcpRanges, err := ironic.PrefixOrNetmaskFromCIDR(instance.Spec.DHCPRanges)
	if err != nil {
		r.Log.Error(err, "Failed to get Prefix or Netmask from IP network Prefix (CIDR)")
	}
	templateParameters["DHCPRanges"] = dhcpRanges
	templateParameters["Standalone"] = instance.Spec.Standalone
	templateParameters["ConductorGroup"] = instance.Spec.ConductorGroup

	cms := []util.Template{
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
