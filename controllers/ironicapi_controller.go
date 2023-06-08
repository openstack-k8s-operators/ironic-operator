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
	ironicapi "github.com/openstack-k8s-operators/ironic-operator/pkg/ironicapi"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/configmap"
	"github.com/openstack-k8s-operators/lib-common/modules/common/deployment"
	"github.com/openstack-k8s-operators/lib-common/modules/common/endpoint"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/labels"
	nad "github.com/openstack-k8s-operators/lib-common/modules/common/networkattachment"
	"github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
)

// GetClient -
func (r *IronicAPIReconciler) GetClient() client.Client {
	return r.Client
}

// GetKClient -
func (r *IronicAPIReconciler) GetKClient() kubernetes.Interface {
	return r.Kclient
}

// GetLogger -
func (r *IronicAPIReconciler) GetLogger() logr.Logger {
	return r.Log
}

// GetScheme -
func (r *IronicAPIReconciler) GetScheme() *runtime.Scheme {
	return r.Scheme
}

// IronicAPIReconciler reconciles a IronicAPI object
type IronicAPIReconciler struct {
	client.Client
	Kclient kubernetes.Interface
	Log     logr.Logger
	Scheme  *runtime.Scheme
}

var (
	keystoneServices = []map[string]string{
		{
			"type": ironic.ServiceType,
			"name": ironic.ServiceName,
			"desc": "Ironic baremetal provisioning service",
		},
	}
)

// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironicapis,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironicapis/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironicapis/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;create;update;patch;delete;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;create;update;patch;delete;watch
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;create;update;patch;delete;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;create;update;patch;delete;watch
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneendpoints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=k8s.cni.cncf.io,resources=network-attachment-definitions,verbs=get;list;watch

// Reconcile -
func (r *IronicAPIReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, _err error) {
	_ = log.FromContext(ctx)

	// Fetch the IronicAPI instance
	instance := &ironicv1.IronicAPI{}
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
			condition.UnknownCondition(condition.ExposeServiceReadyCondition, condition.InitReason, condition.ExposeServiceReadyInitMessage),
			condition.UnknownCondition(condition.InputReadyCondition, condition.InitReason, condition.InputReadyInitMessage),
			condition.UnknownCondition(condition.ServiceConfigReadyCondition, condition.InitReason, condition.ServiceConfigReadyInitMessage),
			condition.UnknownCondition(condition.DeploymentReadyCondition, condition.InitReason, condition.DeploymentReadyInitMessage),
			condition.UnknownCondition(condition.NetworkAttachmentsReadyCondition, condition.InitReason, condition.NetworkAttachmentsReadyInitMessage),
		)

		if !instance.Spec.Standalone {
			// right now we have no dedicated KeystoneServiceReadyInitMessage and KeystoneEndpointReadyInitMessage
			cl = append(cl,
				*condition.UnknownCondition(condition.KeystoneServiceReadyCondition, condition.InitReason, ""),
				*condition.UnknownCondition(condition.KeystoneEndpointReadyCondition, condition.InitReason, ""))
		}

		instance.Status.Conditions.Init(&cl)

		// Register overall status immediately to have an early feedback e.g. in the cli
		return ctrl.Result{}, nil
	}
	if instance.Status.Hash == nil {
		instance.Status.Hash = make(map[string]string)
	}
	if instance.Status.APIEndpoints == nil {
		instance.Status.APIEndpoints = make(map[string]map[string]string)
	}
	if instance.Status.NetworkAttachments == nil {
		instance.Status.NetworkAttachments = map[string][]string{}
	}

	// Handle service delete
	if !instance.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, instance, helper)
	}

	// Handle non-deleted clusters
	return r.reconcileNormal(ctx, instance, helper)
}

// SetupWithManager sets up the controller with the Manager.
func (r *IronicAPIReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// watch for configmap where the CM owner label AND the CR.Spec.ManagingCrName label matches
	configMapFn := func(o client.Object) []reconcile.Request {
		result := []reconcile.Request{}

		// get all API CRs
		apis := &ironicv1.IronicAPIList{}
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
		For(&ironicv1.IronicAPI{}).
		Owns(&keystonev1.KeystoneService{}).
		Owns(&keystonev1.KeystoneEndpoint{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Secret{}).
		Owns(&routev1.Route{}).
		Owns(&corev1.Service{}).
		// watch the config CMs we don't own
		Watches(&source.Kind{Type: &corev1.ConfigMap{}},
			handler.EnqueueRequestsFromMapFunc(configMapFn)).
		Complete(r)
}

func (r *IronicAPIReconciler) reconcileDelete(ctx context.Context, instance *ironicv1.IronicAPI, helper *helper.Helper) (ctrl.Result, error) {
	r.Log.Info("Reconciling API delete")

	for _, ksSvc := range keystoneServices {
		// Remove the finalizer from our KeystoneEndpoint CR
		keystoneEndpoint, err := keystonev1.GetKeystoneEndpointWithName(ctx, helper, ksSvc["name"], instance.Namespace)
		if err != nil && !k8s_errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}

		if err == nil {
			controllerutil.RemoveFinalizer(keystoneEndpoint, helper.GetFinalizer())
			if err = helper.GetClient().Update(ctx, keystoneEndpoint); err != nil && !k8s_errors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
			util.LogForObject(helper, "Removed finalizer from our KeystoneEndpoint", instance)
		}

		// Remove the finalizer from our KeystoneService CR
		keystoneService, err := keystonev1.GetKeystoneServiceWithName(ctx, helper, ksSvc["name"], instance.Namespace)
		if err != nil && !k8s_errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}

		if err == nil {
			controllerutil.RemoveFinalizer(keystoneService, helper.GetFinalizer())
			if err = helper.GetClient().Update(ctx, keystoneService); err != nil && !k8s_errors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
			util.LogForObject(helper, "Removed finalizer from our KeystoneService", instance)
		}
	}

	// Service is deleted so remove the finalizer.
	controllerutil.RemoveFinalizer(instance, helper.GetFinalizer())
	r.Log.Info("Reconciled API delete successfully")

	return ctrl.Result{}, nil
}

func (r *IronicAPIReconciler) reconcileInit(
	ctx context.Context,
	instance *ironicv1.IronicAPI,
	helper *helper.Helper,
	serviceLabels map[string]string,
) (ctrl.Result, error) {
	r.Log.Info("Reconciling API init")

	//
	// expose the service (create service, route and return the created endpoint URLs)
	//

	// V1
	data := map[endpoint.Endpoint]endpoint.Data{
		endpoint.EndpointPublic: {
			Port: ironic.IronicPublicPort,
		},
		endpoint.EndpointInternal: {
			Port: ironic.IronicInternalPort,
		},
	}

	for _, metallbcfg := range instance.Spec.ExternalEndpoints {
		portCfg := data[metallbcfg.Endpoint]
		portCfg.MetalLB = &endpoint.MetalLBData{
			IPAddressPool:   metallbcfg.IPAddressPool,
			SharedIP:        metallbcfg.SharedIP,
			SharedIPKey:     metallbcfg.SharedIPKey,
			LoadBalancerIPs: metallbcfg.LoadBalancerIPs,
		}

		data[metallbcfg.Endpoint] = portCfg
	}

	apiEndpoints, ctrlResult, err := endpoint.ExposeEndpoints(
		ctx,
		helper,
		ironic.ServiceName,
		serviceLabels,
		data,
		time.Duration(5)*time.Second,
	)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ExposeServiceReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ExposeServiceReadyErrorMessage,
			err.Error()))
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ExposeServiceReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.ExposeServiceReadyRunningMessage))
		return ctrlResult, nil
	}

	//
	// Update instance status with service endpoint url from route host information for v2
	//
	// TODO: need to support https default here
	instance.Status.APIEndpoints[ironic.ServiceName] = apiEndpoints
	// V1 - end

	instance.Status.Conditions.MarkTrue(condition.ExposeServiceReadyCondition, condition.ExposeServiceReadyMessage)

	// expose service - end

	//
	// create users and endpoints
	// TODO: rework this
	//
	if !instance.Spec.Standalone {

		for _, ksSvc := range keystoneServices {
			r.Log.Info("Reconciled API init successfully")
			ksSvcSpec := keystonev1.KeystoneServiceSpec{
				ServiceType:        ksSvc["type"],
				ServiceName:        ksSvc["name"],
				ServiceDescription: ksSvc["desc"],
				Enabled:            true,
				ServiceUser:        instance.Spec.ServiceUser,
				Secret:             instance.Spec.Secret,
				PasswordSelector:   instance.Spec.PasswordSelectors.Service,
			}

			ksSvcObj := keystonev1.NewKeystoneService(ksSvcSpec, instance.Namespace, serviceLabels, 10)
			ctrlResult, err = ksSvcObj.CreateOrPatch(ctx, helper)
			if err != nil {
				return ctrlResult, err
			}

			// mirror the Status, Reason, Severity and Message of the latest keystoneservice condition
			// into a local condition with the type condition.KeystoneServiceReadyCondition
			c := ksSvcObj.GetConditions().Mirror(condition.KeystoneServiceReadyCondition)
			if c != nil {
				instance.Status.Conditions.Set(c)
			}

			if (ctrlResult != ctrl.Result{}) {
				return ctrlResult, nil
			}

			//
			// register endpoints
			//
			ksEndptSpec := keystonev1.KeystoneEndpointSpec{
				ServiceName: ksSvc["name"],
				Endpoints:   instance.Status.APIEndpoints[ksSvc["name"]],
			}
			ksEndpt := keystonev1.NewKeystoneEndpoint(
				ksSvc["name"],
				instance.Namespace,
				ksEndptSpec,
				serviceLabels,
				10)
			ctrlResult, err = ksEndpt.CreateOrPatch(ctx, helper)
			if err != nil {
				return ctrlResult, err
			}
			// mirror the Status, Reason, Severity and Message of the latest keystoneendpoint condition
			// into a local condition with the type condition.KeystoneEndpointReadyCondition
			c = ksEndpt.GetConditions().Mirror(condition.KeystoneEndpointReadyCondition)
			if c != nil {
				instance.Status.Conditions.Set(c)
			}

			if (ctrlResult != ctrl.Result{}) {
				return ctrlResult, nil
			}
		}
	}

	r.Log.Info("Reconciled API init successfully")
	return ctrl.Result{}, nil
}

func (r *IronicAPIReconciler) reconcileNormal(ctx context.Context, instance *ironicv1.IronicAPI, helper *helper.Helper) (ctrl.Result, error) {
	r.Log.Info("Reconciling Service")

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

	serviceLabels := map[string]string{
		common.AppSelector:       ironic.ServiceName,
		common.ComponentSelector: ironic.APIComponent,
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

	// Handle service init
	ctrlResult, err := r.reconcileInit(ctx, instance, helper, serviceLabels)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// Handle service update
	ctrlResult, err = r.reconcileUpdate(ctx, instance, helper)
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

	// Define a new Deployment object
	depl := deployment.NewDeployment(
		ironicapi.Deployment(instance, inputHash, serviceLabels, serviceAnnotations),
		time.Duration(5)*time.Second,
	)

	ctrlResult, err = depl.CreateOrPatch(ctx, helper)
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
	instance.Status.ReadyCount = depl.GetDeployment().Status.ReadyReplicas

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
	// create Deployment - end

	r.Log.Info("Reconciled API successfully")
	return ctrl.Result{}, nil
}

func (r *IronicAPIReconciler) reconcileUpdate(ctx context.Context, instance *ironicv1.IronicAPI, helper *helper.Helper) (ctrl.Result, error) {
	r.Log.Info("Reconciling API update")

	// TODO: should have minor update tasks if required
	// - delete dbsync hash from status to rerun it?

	r.Log.Info("Reconciled API update successfully")
	return ctrl.Result{}, nil
}

func (r *IronicAPIReconciler) reconcileUpgrade(ctx context.Context, instance *ironicv1.IronicAPI, helper *helper.Helper) (ctrl.Result, error) {
	r.Log.Info("Reconciling API upgrade")

	// TODO: should have major version upgrade tasks
	// -delete dbsync hash from status to rerun it?

	r.Log.Info("Reconciled API upgrade successfully")
	return ctrl.Result{}, nil
}

// generateServiceConfigMaps - create custom configmap to hold service-specific config
// TODO add DefaultConfigOverwrite
func (r *IronicAPIReconciler) generateServiceConfigMaps(
	ctx context.Context,
	h *helper.Helper,
	instance *ironicv1.IronicAPI,
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

	cms := []util.Template{
		// Custom ConfigMap
		{
			Name:         fmt.Sprintf("%s-config-data", instance.Name),
			Namespace:    instance.Namespace,
			Type:         util.TemplateTypeConfig,
			InstanceType: instance.Kind,
			CustomData:   customData,
			Labels:       cmLabels,
		},
	}

	return configmap.EnsureConfigMaps(ctx, h, instance, cms, envVars)
}

// createHashOfInputHashes - creates a hash of hashes which gets added to the resources which requires a restart
// if any of the input resources change, like configs, passwords, ...
//
// returns the hash, whether the hash changed (as a bool) and any error
func (r *IronicAPIReconciler) createHashOfInputHashes(
	ctx context.Context,
	instance *ironicv1.IronicAPI,
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
