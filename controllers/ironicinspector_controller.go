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

	"github.com/go-logr/logr"
	ironic "github.com/openstack-k8s-operators/ironic-operator/pkg/ironic"
	ironicinspector "github.com/openstack-k8s-operators/ironic-operator/pkg/ironicinspector"

	common "github.com/openstack-k8s-operators/lib-common/modules/common"
	configmap "github.com/openstack-k8s-operators/lib-common/modules/common/configmap"
	endpoint "github.com/openstack-k8s-operators/lib-common/modules/common/endpoint"
	env "github.com/openstack-k8s-operators/lib-common/modules/common/env"
	job "github.com/openstack-k8s-operators/lib-common/modules/common/job"
	oko_secret "github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	"github.com/openstack-k8s-operators/lib-common/modules/common/statefulset"
	util "github.com/openstack-k8s-operators/lib-common/modules/common/util"
	database "github.com/openstack-k8s-operators/lib-common/modules/database"

	routev1 "github.com/openshift/api/route/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"

	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	labels "github.com/openstack-k8s-operators/lib-common/modules/common/labels"

	ironicv1 "github.com/openstack-k8s-operators/ironic-operator/api/v1beta1"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	rabbitmqv1 "github.com/openstack-k8s-operators/openstack-operator/apis/rabbitmq/v1beta1"

	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/route"
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// IronicInspectorReconciler reconciles a IronicInspector object
type IronicInspectorReconciler struct {
	client.Client
	Kclient kubernetes.Interface
	Log     logr.Logger
	Scheme  *runtime.Scheme
}

var (
	inspectorKeystoneServices = []map[string]string{
		{
			"name": "ironic-inspector",
			"type": "baremetal-introspection",
			"desc": "OpenStack Baremetal-Introspection Service",
		},
	}
)

// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironicinspectors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironicinspectors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironicinspectors/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;create;update;patch;delete;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;create;update;patch;delete;watch
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;delete;
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;create;update;patch;delete;watch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;create;update;patch;delete;watch
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list
// +kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbdatabases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneapis,verbs=get;list;watch
// +kubebuilder:rbac:groups=rabbitmq.openstack.org,resources=transporturls,verbs=get;list;watch;create;update;patch;delete

// Reconcile -
func (r *IronicInspectorReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (result ctrl.Result, _err error) {
	_ = log.FromContext(ctx)

	// Fetch the IronicInspector instance
	instance := &ironicv1.IronicInspector{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Request object not found, could have been deleted after
			// reconcile request.
			// Owned objects are automatically garbage collected.
			// For additional cleanup logic use finalizers. Return and don't
			// requeue.
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
			instance.Status.Conditions.MarkTrue(
				condition.ReadyCondition,
				condition.ReadyMessage)
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
			condition.UnknownCondition(
				condition.ExposeServiceReadyCondition,
				condition.InitReason,
				condition.ExposeServiceReadyInitMessage),
			condition.UnknownCondition(
				condition.InputReadyCondition,
				condition.InitReason,
				condition.InputReadyInitMessage),
			condition.UnknownCondition(
				condition.ServiceConfigReadyCondition,
				condition.InitReason,
				condition.ServiceConfigReadyInitMessage),
			condition.UnknownCondition(
				condition.DeploymentReadyCondition,
				condition.InitReason,
				condition.DeploymentReadyInitMessage),
		)

		if !instance.Spec.Standalone {
			// right now we have no dedicated KeystoneServiceReadyInitMessage
			cl = append(cl, *condition.UnknownCondition(
				condition.KeystoneServiceReadyCondition,
				condition.InitReason,
				""))
		}

		instance.Status.Conditions.Init(&cl)

		// Register overall status immediately to have an early feedback
		// e.g. in the cli
		return ctrl.Result{}, err
	}
	if instance.Status.Hash == nil {
		instance.Status.Hash = map[string]string{}
	}
	if instance.Status.ServiceIDs == nil {
		instance.Status.ServiceIDs = map[string]string{}
	}

	// Handle service delete
	if !instance.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, instance, helper)
	}

	// Handle non-deleted clusters
	return r.reconcileNormal(ctx, instance, helper)
}

// SetupWithManager sets up the controller with the Manager.
func (r *IronicInspectorReconciler) SetupWithManager(
	mgr ctrl.Manager,
) error {
	// watch for configmap where the CM owner label AND the CR.Spec.ManagingCrName label matches
	configMapFn := func(o client.Object) []reconcile.Request {
		result := []reconcile.Request{}

		// get all API CRs
		apis := &ironicv1.IronicInspectorList{}
		listOpts := []client.ListOption{
			client.InNamespace(o.GetNamespace()),
		}
		if err := r.Client.List(
			context.Background(),
			apis,
			listOpts...); err != nil {

			r.Log.Error(err, "Unable to retrieve API CRs %v")
			return nil
		}

		label := o.GetLabels()
		// TODO: Just trying to verify that the CM is owned by this CR's managing CR
		if l, ok := label[labels.GetOwnerNameLabelSelector(
			labels.GetGroupLabel(ironic.ServiceName))]; ok {

			for _, cr := range apis.Items {
				// return reconcil event for the CR where the CM owner label
				// AND the parentIronicName matches
				if l == ironic.GetOwningIronicName(&cr) {
					// return namespace and Name of CR
					name := client.ObjectKey{
						Namespace: o.GetNamespace(),
						Name:      cr.Name,
					}
					r.Log.Info(fmt.Sprintf(
						"ConfigMap object %s and CR %s marked with label: %s",
						o.GetName(), cr.Name, l))
					result = append(
						result, reconcile.Request{NamespacedName: name})
				}
			}
		}
		if len(result) > 0 {
			return result
		}
		return nil
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&ironicv1.IronicInspector{}).
		Owns(&corev1.Secret{}).
		Owns(&routev1.Route{}).
		Owns(&corev1.Service{}).
		// watch the config CMs we don't own
		Watches(
			&source.Kind{Type: &corev1.ConfigMap{}},
			handler.EnqueueRequestsFromMapFunc(configMapFn)).
		Complete(r)
}

func (r *IronicInspectorReconciler) reconcileNormal(
	ctx context.Context,
	instance *ironicv1.IronicInspector,
	helper *helper.Helper,
) (ctrl.Result, error) {
	r.Log.Info("Reconciling Ironic Inspector")

	// ConfigMap
	configMapVars := make(map[string]env.Setter)

	// transportURL
	if instance.Spec.RPCTransport == "oslo" {
		//
		// Create RabbitMQ transport URL CR and get the actual URL from the
		// associted secret that is created
		//
		transportURL, op, err := r.transportURLCreateOrUpdate(instance)

		if err != nil {
			instance.Status.Conditions.Set(condition.FalseCondition(
				ironicv1.IronicRabbitMqTransportURLReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				ironicv1.IronicRabbitMqTransportURLReadyErrorMessage,
				err.Error(),
			))
			return ctrl.Result{}, err
		}

		if op != controllerutil.OperationResultNone {
			r.Log.Info(fmt.Sprintf(
				"TransportURL %s successfully reconciled - operation: %s",
				transportURL.Name, string(op)))
		}

		instance.Status.TransportURLSecret = transportURL.Status.SecretName

		if instance.Status.TransportURLSecret == "" {
			r.Log.Info(fmt.Sprintf(
				"Waiting for TransportURL %s secret to be created",
				transportURL.Name))
			instance.Status.Conditions.Set(condition.FalseCondition(
				ironicv1.IronicRabbitMqTransportURLReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				ironicv1.IronicRabbitMqTransportURLReadyRunningMessage))
			return ctrl.Result{RequeueAfter: time.Second * 10}, nil
		}

		instance.Status.Conditions.MarkTrue(
			ironicv1.IronicRabbitMqTransportURLReadyCondition,
			ironicv1.IronicRabbitMqTransportURLReadyMessage)
	} else {
		instance.Status.TransportURLSecret = ""
		instance.Status.Conditions.MarkTrue(
			ironicv1.IronicRabbitMqTransportURLReadyCondition,
			ironicv1.IronicRabbitMqTransportURLDisabledMessage)
	}
	// transportURL - end

	//
	// check for required OpenStack secret holding passwords for
	// service/admin user and add hash to the vars map
	//
	ospSecret, hash, err := oko_secret.GetSecret(
		ctx,
		helper,
		instance.Spec.Secret,
		instance.Namespace)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			instance.Status.Conditions.Set(
				condition.FalseCondition(
					condition.InputReadyCondition,
					condition.RequestedReason,
					condition.SeverityInfo,
					condition.InputReadyWaitingMessage))
			return ctrl.Result{RequeueAfter: time.Second * 10},
				fmt.Errorf("OpenStack secret %s not found",
					instance.Spec.Secret)
		}
		instance.Status.Conditions.Set(
			condition.FalseCondition(
				condition.InputReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.InputReadyErrorMessage,
				err.Error()))
		return ctrl.Result{}, err
	}
	configMapVars[ospSecret.Name] = env.SetValue(hash)

	instance.Status.Conditions.MarkTrue(
		condition.InputReadyCondition,
		condition.InputReadyMessage)
	// run check OpenStack secret - end

	//
	// Create ConfigMaps and Secrets required as input for the Service and
	// calculate an overall hash of hashes
	//

	//
	// create Configmap required for ironic input
	// - %-scripts configmap holding scripts to e.g. bootstrap the service
	// - %-config configmap holding minimal ironic config required to get the
	//   service up, user can add additional files to be added to the service
	// - parameters which has passwords gets added from the OpenStack secret
	//   via the init container
	//
	err = r.generateServiceConfigMaps(
		ctx,
		instance,
		helper,
		&configMapVars)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ServiceConfigReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ServiceConfigReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

	//
	// create hash over all the different input resources to identify if any
	// those changed and a restart/recreate is required.
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

	instance.Status.Conditions.MarkTrue(
		condition.ServiceConfigReadyCondition,
		condition.ServiceConfigReadyMessage)

	// Create ConfigMaps and Secrets - end

	//
	// TODO check when/if Init, Update, or Upgrade should/could be skipped
	//

	serviceLabels := map[string]string{
		common.AppSelector:       ironic.ServiceName,
		ironic.ComponentSelector: ironic.InspectorComponent,
	}

	ingressDomain := r.GetIngressDomain(ctx, helper)

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

	// Define a new StatefulSet object
	ssDef, err := ironicinspector.StatefulSet(
		instance, inputHash, serviceLabels, ingressDomain)
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

	// Handle service init
	ctrlResult, err = r.reconcileServices(ctx, instance, helper, serviceLabels)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	instance.Status.ReadyCount = ss.GetStatefulSet().Status.ReadyReplicas
	if instance.Status.ReadyCount > 0 {
		instance.Status.Conditions.MarkTrue(
			condition.DeploymentReadyCondition,
			condition.DeploymentReadyMessage)
	}
	// create Statefulset - end
	instance.Status.Networks = instance.Spec.NetworkAttachments

	r.Log.Info("Reconciled Ironic Inspector successfully")
	return ctrl.Result{}, nil
}

func (r *IronicInspectorReconciler) reconcileDelete(
	ctx context.Context,
	instance *ironicv1.IronicInspector,
	helper *helper.Helper,
) (ctrl.Result, error) {
	r.Log.Info("Reconciling Ironic Inspector delete")

	for _, ksSvc := range inspectorKeystoneServices {
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

	// remove db finalizer first
	db, err := database.GetDatabaseByName(ctx, helper, instance.Name)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	if !k8s_errors.IsNotFound(err) {
		if err := db.DeleteFinalizer(ctx, helper); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Service is deleted so remove the finalizer.
	controllerutil.RemoveFinalizer(instance, helper.GetFinalizer())
	r.Log.Info("Reconciled Ironic Inspector delete successfully")

	return ctrl.Result{}, nil
}

func (r *IronicInspectorReconciler) reconcileInit(
	ctx context.Context,
	instance *ironicv1.IronicInspector,
	helper *helper.Helper,
	serviceLabels map[string]string,
) (ctrl.Result, error) {
	r.Log.Info("Reconciling Ironic Inspector init")

	//
	// create service DB instance
	//
	databaseName := strings.Replace(instance.Name, "-", "_", -1)
	db := database.NewDatabase(
		databaseName,
		instance.Spec.DatabaseUser,
		instance.Spec.Secret,
		map[string]string{
			"dbName": instance.Spec.DatabaseInstance,
		},
	)

	// create or patch the DB
	ctrlResult, err := db.CreateOrPatchDBByName(
		ctx,
		helper,
		instance.Spec.DatabaseInstance,
	)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DBReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DBReadyRunningMessage))
		return ctrlResult, nil
	}

	// wait for the DB to be setup
	ctrlResult, err = db.WaitForDBCreatedWithTimeout(ctx, helper, time.Second*10)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DBReadyErrorMessage,
			err.Error()))
		return ctrlResult, err
	}
	if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DBReadyRunningMessage))
		return ctrlResult, nil
	}
	// update Status.DatabaseHostname, used to bootstrap/config the service
	instance.Status.DatabaseHostname = db.GetDatabaseHostname()
	instance.Status.Conditions.MarkTrue(
		condition.DBReadyCondition,
		condition.DBReadyMessage)

	// create service DB - end

	//
	// run ironic-inspector db sync
	//
	dbSyncHash := instance.Status.Hash[ironicv1.DbSyncHash]
	jobDef := ironicinspector.DbSyncJob(instance, serviceLabels)
	dbSyncjob := job.NewJob(
		jobDef,
		ironicv1.DbSyncHash,
		instance.Spec.PreserveJobs,
		5,
		dbSyncHash,
	)
	ctrlResult, err = dbSyncjob.DoJob(
		ctx,
		helper,
	)
	if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBSyncReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DBSyncReadyRunningMessage))
		return ctrlResult, nil
	}
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBSyncReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DBSyncReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	if dbSyncjob.HasChanged() {
		instance.Status.Hash[ironicv1.DbSyncHash] = dbSyncjob.GetHash()
		r.Log.Info(fmt.Sprintf(
			"Job %s hash added - %s",
			jobDef.Name,
			instance.Status.Hash[ironicv1.DbSyncHash]))
	}
	instance.Status.Conditions.MarkTrue(
		condition.DBSyncReadyCondition,
		condition.DBSyncReadyMessage)

	// run ironic-inspector db sync - end

	//
	// expose the service (create service, route and return the created endpoint URLs)
	//

	// V1
	data := map[endpoint.Endpoint]endpoint.Data{
		endpoint.EndpointPublic: {
			Port: ironicinspector.IronicInspectorPublicPort,
		},
		endpoint.EndpointAdmin: {
			Port: ironicinspector.IronicInspectorAdminPort,
		},
		endpoint.EndpointInternal: {
			Port: ironicinspector.IronicInspectorInternalPort,
		},
	}

	inspectorServiceName := ironic.ServiceName + "-" + ironic.InspectorComponent
	apiEndpoints, ctrlResult, err := endpoint.ExposeEndpoints(
		ctx,
		helper,
		inspectorServiceName,
		serviceLabels,
		data,
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
	if instance.Status.APIEndpoints == nil {
		instance.Status.APIEndpoints = map[string]map[string]string{}
	}
	instance.Status.APIEndpoints[inspectorServiceName] = apiEndpoints
	// V1 - end

	instance.Status.Conditions.MarkTrue(condition.ExposeServiceReadyCondition, condition.ExposeServiceReadyMessage)

	// expose service - end

	//
	// create users and endpoints
	// TODO: rework this
	//
	if instance.Status.ServiceIDs == nil {
		instance.Status.ServiceIDs = map[string]string{}
	}

	if !instance.Spec.Standalone {

		r.Log.Info("Create Inspector Keystone Service and Endpoints")
		for _, ksSvc := range inspectorKeystoneServices {
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

			instance.Status.ServiceIDs[ksSvc["name"]] = ksSvcObj.GetServiceID()

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
			// // mirror the Status, Reason, Severity and Message of the latest keystoneendpoint condition
			// // into a local condition with the type condition.KeystoneEndpointReadyCondition
			c = ksEndpt.GetConditions().Mirror(condition.KeystoneEndpointReadyCondition)
			if c != nil {
				instance.Status.Conditions.Set(c)
			}

			if (ctrlResult != ctrl.Result{}) {
				return ctrlResult, nil
			}
		}
	}

	r.Log.Info("Reconciled Ironic Inspector init successfully")
	return ctrl.Result{}, nil
}

func (r *IronicInspectorReconciler) reconcileUpdate(
	ctx context.Context,
	instance *ironicv1.IronicInspector,
	helper *helper.Helper,
) (ctrl.Result, error) {
	r.Log.Info("Reconciling Ironic Inspector Service update")

	// TODO: should have minor update tasks if required
	// - delete dbsync hash from status to rerun it?

	return ctrl.Result{}, nil
}

func (r *IronicInspectorReconciler) reconcileUpgrade(
	ctx context.Context,
	instance *ironicv1.IronicInspector,
	helper *helper.Helper,
) (ctrl.Result, error) {
	r.Log.Info("Reconciling Ironic Inspector Service upgrade")

	// TODO: should have major version upgrade tasks
	// -delete dbsync hash from status to rerun it?

	return ctrl.Result{}, nil
}

// transportURLCreateOrUpdate - creates or updates rabbitmq transport URL
func (r *IronicInspectorReconciler) transportURLCreateOrUpdate(
	instance *ironicv1.IronicInspector,
) (*rabbitmqv1.TransportURL, controllerutil.OperationResult, error) {
	transportURL := &rabbitmqv1.TransportURL{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-transport", instance.Name),
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(
		context.TODO(),
		r.Client,
		transportURL,
		func() error {

			transportURL.Spec.RabbitmqClusterName = instance.Spec.RabbitMqClusterName

			err := controllerutil.SetControllerReference(
				instance, transportURL, r.Scheme)
			return err
		})

	return transportURL, op, err
}

// generateServiceConfigMaps - create create configmaps which hold scripts and service configuration
// TODO add DefaultConfigOverwrite
func (r *IronicInspectorReconciler) generateServiceConfigMaps(
	ctx context.Context,
	instance *ironicv1.IronicInspector,
	h *helper.Helper,
	envVars *map[string]env.Setter,
) error {
	//
	// create Configmap/Secret required for ironic-inspector input
	// - %-scripts configmap holding scripts to e.g. bootstrap the service
	// - %-config configmap holding minimal ironic-inspector config required
	//   to get the service up, user can add additional files to be added to
	//   the service
	// - parameters which has passwords gets added from the ospSecret via the
	//   init container
	//
	cmLabels := labels.GetLabels(
		instance,
		labels.GetGroupLabel(ironic.ServiceName),
		map[string]string{})

	// customData hold any customization for the service.
	// custom.conf is going to /etc/ironic-inspector/inspector.conf.d
	// all other files get placed into /etc/ironic-inspector to allow
	// overwrite of e.g. policy.json.
	// TODO: make sure custom.conf can not be overwritten
	customData := map[string]string{
		common.CustomServiceConfigFileName: instance.Spec.CustomServiceConfig}
	for key, data := range instance.Spec.DefaultConfigOverwrite {
		customData[key] = data
	}
	templateParameters := make(map[string]interface{})
	if !instance.Spec.Standalone {

		keystoneAPI, err := keystonev1.GetKeystoneAPI(
			ctx, h, instance.Namespace, map[string]string{})
		if err != nil {
			return err
		}
		authURL, err := keystoneAPI.GetEndpoint(endpoint.EndpointInternal)
		if err != nil {
			return err
		}

		templateParameters["ServiceUser"] = instance.Spec.ServiceUser
		templateParameters["KeystoneInternalURL"] = authURL
	}
	templateParameters["DHCPRanges"] = instance.Spec.DHCPRanges
	templateParameters["Standalone"] = instance.Spec.Standalone

	cms := []util.Template{
		// ScriptsConfigMap
		{
			Name:         fmt.Sprintf("%s-scripts", instance.Name),
			Namespace:    instance.Namespace,
			Type:         util.TemplateTypeScripts,
			InstanceType: instance.Kind,
			AdditionalTemplate: map[string]string{
				"common.sh":  "/common/common.sh",
				"imagetter":  "/common/imagetter",
				"get_net_ip": "/common/get_net_ip",
			},
			Labels: cmLabels,
		},
		// ConfigMap
		{
			Name:          fmt.Sprintf("%s-config-data", instance.Name),
			Namespace:     instance.Namespace,
			Type:          util.TemplateTypeConfig,
			InstanceType:  instance.Kind,
			CustomData:    customData,
			ConfigOptions: templateParameters,
			Labels:        cmLabels,
		},
	}
	return configmap.EnsureConfigMaps(ctx, h, instance, cms, envVars)
}

// createHashOfInputHashes - creates a hash of hashes which gets added to the
// resources which requires a restart if any of the input resources change,
// like configs, passwords, ...
//
// returns the hash, whether the hash changed (as a bool) and any error
func (r *IronicInspectorReconciler) createHashOfInputHashes(
	ctx context.Context,
	instance *ironicv1.IronicInspector,
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

// GetIngressDomain - Get the Ingress Domain of cluster
func (r *IronicInspectorReconciler) GetIngressDomain(
	ctx context.Context,
	helper *helper.Helper,
) string {
	ingress := &unstructured.Unstructured{}
	ingress.SetGroupVersionKind(
		schema.GroupVersionKind{
			Group:   "operator.openshift.io",
			Version: "v1",
			Kind:    "IngressController",
		},
	)
	err := helper.GetClient().Get(
		context.Background(),
		client.ObjectKey{
			Namespace: "openshift-ingress-operator",
			Name:      "default",
		},
		ingress,
	)
	if err != nil {
		r.Log.Error(err, "Unable to retrieve Ingress Domain %v")
		return ""
	}
	ingressDomain := ""

	ingressStatus := ingress.UnstructuredContent()["status"]
	ingressStatusMap, ok := ingressStatus.(map[string]interface{})
	if !ok {
		r.Log.Info(fmt.Sprintf("Wanted type map[string]interface{}; got %T", ingressStatus))
	}
	for k, v := range ingressStatusMap {
		if k == "domain" {
			ingressDomain = v.(string)
			// Break out of the loop, we got what we need
			break
		}
	}
	if ingressDomain != "" {
		r.Log.Info(fmt.Sprintf("Found ingress domain: %s", ingressDomain))
	} else {
		r.Log.Info("Unable to get the ingress domain.")
	}

	return ingressDomain
}

func (r *IronicInspectorReconciler) reconcileServices(
	ctx context.Context,
	instance *ironicv1.IronicInspector,
	helper *helper.Helper,
	serviceLabels map[string]string,
) (ctrl.Result, error) {
	r.Log.Info("Reconciling Inspector Services")

	podList, err := ironicinspector.InspectorPods(
		ctx, instance, helper, serviceLabels)
	if err != nil {
		return ctrl.Result{}, err
	}

	for _, inspectorPod := range podList.Items {
		// Create the inspector pod service if none exists
		inspectorServiceLabels := map[string]string{
			common.AppSelector:       ironic.ServiceName,
			ironic.ComponentSelector: ironic.InspectorComponent,
		}
		inspectorService := ironicinspector.Service(
			inspectorPod.Name,
			instance,
			inspectorServiceLabels)
		if inspectorService != nil {
			svc := service.NewService(inspectorService, inspectorServiceLabels, 5)
			ctrlResult, err := svc.CreateOrPatch(ctx, helper)
			if err != nil {
				return ctrl.Result{}, err
			} else if (ctrlResult != ctrl.Result{}) {
				return ctrl.Result{}, nil
			}
		}
		// create service - end

		if instance.Spec.InspectionNetwork == "" {
			// Create the inspector pod route to enable traffic to the
			// httpboot service, only when there is no inspection network
			inspectorRouteLabels := map[string]string{
				common.AppSelector:       ironic.ServiceName,
				ironic.ComponentSelector: ironic.InspectorComponent + "-" + ironic.HttpbootComponent,
			}
			route := route.NewRoute(
				ironicinspector.Route(
					inspectorPod.Name,
					instance,
					inspectorRouteLabels),
				inspectorRouteLabels,
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
	if instance.Status.ServiceIDs == nil {
		instance.Status.ServiceIDs = map[string]string{}
	}

	r.Log.Info("Reconciled Inspector Services successfully")
	return ctrl.Result{}, nil
}
