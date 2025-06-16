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

	common "github.com/openstack-k8s-operators/lib-common/modules/common"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	endpoint "github.com/openstack-k8s-operators/lib-common/modules/common/endpoint"
	env "github.com/openstack-k8s-operators/lib-common/modules/common/env"
	helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	job "github.com/openstack-k8s-operators/lib-common/modules/common/job"
	labels "github.com/openstack-k8s-operators/lib-common/modules/common/labels"
	common_rbac "github.com/openstack-k8s-operators/lib-common/modules/common/rbac"
	oko_secret "github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	util "github.com/openstack-k8s-operators/lib-common/modules/common/util"

	ironicv1 "github.com/openstack-k8s-operators/ironic-operator/api/v1beta1"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"

	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// IronicReconciler reconciles a Ironic object
type IronicReconciler struct {
	client.Client
	Kclient kubernetes.Interface
	Scheme  *runtime.Scheme
}

// GetLogger returns a logger object with a prefix of "conroller.name" and aditional controller context fields
func (r *IronicReconciler) GetLogger(ctx context.Context) logr.Logger {
	return log.FromContext(ctx).WithName("Controllers").WithName("Ironic")
}

// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironics,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironics/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironics/finalizers,verbs=update;patch
// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironicapis,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironicapis/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironicapis/finalizers,verbs=update;patch
// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironicconductors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironicconductors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironicconductors/finalizers,verbs=update;patch
// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironicinspectors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironicinspectors/finalizers,verbs=update;patch
// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironicneutronagents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironicneutronagents/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironicneutronagents/finalizers,verbs=update;patch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;create;update;patch;delete;watch
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;delete;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;create;update;patch;delete;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;create;update;patch;delete;watch
// +kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbdatabases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbaccounts/finalizers,verbs=update;patch
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneapis,verbs=get;list;watch
// +kubebuilder:rbac:groups=rabbitmq.openstack.org,resources=transporturls,verbs=get;list;watch;create;update;patch;delete

// service account, role, rolebinding
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=roles,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=rolebindings,verbs=get;list;watch;create;update;patch
// service account permissions that are needed to grant permission to the above
// +kubebuilder:rbac:groups="security.openshift.io",resourceNames=anyuid;privileged,resources=securitycontextconstraints,verbs=use
// +kubebuilder:rbac:groups="",resources=pods,verbs=create;delete;get;list;patch;update;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Ironic object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.1/pkg/reconcile
func (r *IronicReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, _err error) {
	Log := r.GetLogger(ctx)

	// Fetch the Ironic instance
	instance := &ironicv1.Ironic{}
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
		Log,
	)
	if err != nil {
		return ctrl.Result{}, err
	}

	// initialize status if Conditions is nil, but do not reset if it already
	// exists
	isNewInstance := instance.Status.Conditions == nil
	if isNewInstance {
		instance.Status.Conditions = condition.Conditions{}
	}

	// Save a copy of the condtions so that we can restore the LastTransitionTime
	// when a condition's state doesn't change.
	savedConditions := instance.Status.Conditions.DeepCopy()

	// Always patch the instance status when exiting this function so we can
	// persist any changes.
	defer func() {
		// Don't update the status, if reconciler Panics
		if r := recover(); r != nil {
			Log.Info(fmt.Sprintf("panic during reconcile %v\n", r))
			panic(r)
		}
		condition.RestoreLastTransitionTimes(
			&instance.Status.Conditions, savedConditions)
		if instance.Status.Conditions.IsUnknown(condition.ReadyCondition) {
			instance.Status.Conditions.Set(
				instance.Status.Conditions.Mirror(condition.ReadyCondition))
		}
		err := helper.PatchInstance(ctx, instance)
		if err != nil {
			_err = err
			return
		}
	}()

	//
	// initialize status
	//
	cl := condition.CreateList(
		condition.UnknownCondition(condition.ReadyCondition, condition.InitReason, condition.ReadyInitMessage),
		condition.UnknownCondition(condition.DBReadyCondition, condition.InitReason, condition.DBReadyInitMessage),
		condition.UnknownCondition(condition.DBSyncReadyCondition, condition.InitReason, condition.DBSyncReadyInitMessage),
		condition.UnknownCondition(condition.InputReadyCondition, condition.InitReason, condition.InputReadyInitMessage),
		condition.UnknownCondition(condition.ServiceConfigReadyCondition, condition.InitReason, condition.ServiceConfigReadyInitMessage),
		condition.UnknownCondition(ironicv1.IronicAPIReadyCondition, condition.InitReason, ironicv1.IronicAPIReadyInitMessage),
		condition.UnknownCondition(ironicv1.IronicConductorReadyCondition, condition.InitReason, ironicv1.IronicConductorReadyInitMessage),
		condition.UnknownCondition(ironicv1.IronicInspectorReadyCondition, condition.InitReason, ironicv1.IronicInspectorReadyInitMessage),
		condition.UnknownCondition(ironicv1.IronicNeutronAgentReadyCondition, condition.InitReason, ironicv1.IronicNeutronAgentReadyInitMessage),
		condition.UnknownCondition(condition.RabbitMqTransportURLReadyCondition, condition.InitReason, condition.RabbitMqTransportURLReadyInitMessage),
		// service account, role, rolebinding conditions
		condition.UnknownCondition(condition.ServiceAccountReadyCondition, condition.InitReason, condition.ServiceAccountReadyInitMessage),
		condition.UnknownCondition(condition.RoleReadyCondition, condition.InitReason, condition.RoleReadyInitMessage),
		condition.UnknownCondition(condition.RoleBindingReadyCondition, condition.InitReason, condition.RoleBindingReadyInitMessage),
	)

	instance.Status.Conditions.Init(&cl)
	instance.Status.ObservedGeneration = instance.Generation

	// If we're not deleting this and the service object doesn't have our finalizer, add it.
	if instance.DeletionTimestamp.IsZero() && controllerutil.AddFinalizer(instance, helper.GetFinalizer()) || isNewInstance {
		return ctrl.Result{}, nil
	}

	if instance.Status.Hash == nil {
		instance.Status.Hash = make(map[string]string)
	}
	if instance.Status.APIEndpoints == nil {
		instance.Status.APIEndpoints = make(map[string]map[string]string)
	}

	// Handle service delete
	if !instance.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, instance, helper)
	}

	// Handle non-deleted clusters
	return r.reconcileNormal(ctx, instance, helper)
}

// SetupWithManager sets up the controller with the Manager.
func (r *IronicReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ironicv1.Ironic{}).
		Owns(&ironicv1.IronicConductor{}).
		Owns(&ironicv1.IronicAPI{}).
		Owns(&ironicv1.IronicInspector{}).
		Owns(&ironicv1.IronicNeutronAgent{}).
		Owns(&mariadbv1.MariaDBDatabase{}).
		Owns(&mariadbv1.MariaDBAccount{}).
		Owns(&batchv1.Job{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Watches(&keystonev1.KeystoneAPI{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectForSrc),
			builder.WithPredicates(keystonev1.KeystoneAPIStatusChangedPredicate)).
		Complete(r)
}

func (r *IronicReconciler) findObjectForSrc(ctx context.Context, src client.Object) []reconcile.Request {
	requests := []reconcile.Request{}

	l := log.FromContext(ctx).WithName("Controllers").WithName("Ironic")

	crList := &ironicv1.IronicList{}
	listOps := &client.ListOptions{
		Namespace: src.GetNamespace(),
	}
	err := r.Client.List(ctx, crList, listOps)
	if err != nil {
		l.Error(err, fmt.Sprintf("listing %s for namespace: %s", crList.GroupVersionKind().Kind, src.GetNamespace()))
		return requests
	}

	for _, item := range crList.Items {
		l.Info(fmt.Sprintf("input source %s changed, reconcile: %s - %s", src.GetName(), item.GetName(), item.GetNamespace()))

		requests = append(requests,
			reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      item.GetName(),
					Namespace: item.GetNamespace(),
				},
			},
		)
	}

	return requests
}

func (r *IronicReconciler) reconcileDelete(ctx context.Context, instance *ironicv1.Ironic, helper *helper.Helper) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)

	Log.Info("Reconciling Ironic delete")

	// remove db finalizer first
	db, err := mariadbv1.GetDatabaseByNameAndAccount(ctx, helper, ironic.DatabaseCRName, instance.Spec.DatabaseAccount, instance.Namespace)
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
	Log.Info("Reconciled Ironic delete successfully")

	return ctrl.Result{}, nil
}

func (r *IronicReconciler) reconcileNormal(ctx context.Context, instance *ironicv1.Ironic, helper *helper.Helper) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)

	Log.Info("Reconciling Service")

	// Service account, role, binding
	rbacResult, err := common_rbac.ReconcileRbac(ctx, helper, instance, getCommonRbacRules())
	if err != nil {
		return rbacResult, err
	} else if (rbacResult != ctrl.Result{}) {
		return rbacResult, nil
	}

	// ConfigMap
	configMapVars := make(map[string]env.Setter)

	// Initialize the IronicConductorReadyCount map
	instance.Status.IronicConductorReadyCount = make(map[string]int32)

	if instance.Spec.RPCTransport == "oslo" {
		//
		// Create RabbitMQ transport URL CR and get the actual URL from the associted secret that is created
		//
		transportURL, op, err := ironic.TransportURLCreateOrUpdate(
			instance.Name,
			instance.Namespace,
			instance.Spec.RabbitMqClusterName,
			instance,
			helper,
		)
		if err != nil {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.RabbitMqTransportURLReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.RabbitMqTransportURLReadyErrorMessage,
				err.Error(),
			))
			return ctrl.Result{}, err
		}

		if op != controllerutil.OperationResultNone {
			Log.Info(fmt.Sprintf("TransportURL %s successfully reconciled - operation: %s", transportURL.Name, string(op)))
		}

		instance.Status.TransportURLSecret = transportURL.Status.SecretName

		if instance.Status.TransportURLSecret == "" {
			Log.Info(fmt.Sprintf("Waiting for TransportURL %s secret to be created", transportURL.Name))
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.RabbitMqTransportURLReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				condition.RabbitMqTransportURLReadyRunningMessage))
			return ctrl.Result{RequeueAfter: time.Second * 10}, nil
		}

		instance.Status.Conditions.MarkTrue(condition.RabbitMqTransportURLReadyCondition, condition.RabbitMqTransportURLReadyMessage)
	} else {
		instance.Status.TransportURLSecret = ""
		instance.Status.Conditions.MarkTrue(condition.RabbitMqTransportURLReadyCondition, ironicv1.RabbitMqTransportURLDisabledMessage)
	}
	// end transportURL

	//
	// check for required OpenStack secret holding passwords for service/admin user and add hash to the vars map
	//
	ospSecret, hash, err := oko_secret.GetSecret(ctx, helper, instance.Spec.Secret, instance.Namespace)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			Log.Info(fmt.Sprintf("OpenStack secret %s not found", instance.Spec.Secret))
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.InputReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				condition.InputReadyWaitingMessage))
			return ctrl.Result{RequeueAfter: time.Second * 10}, nil
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

	instance.Status.Conditions.MarkTrue(condition.InputReadyCondition, condition.InputReadyMessage)
	// run check OpenStack secret - end

	// Get Keystone endpoints
	keystoneEndpoints := ironicv1.KeystoneEndpoints{}
	if !instance.Spec.Standalone {
		keystoneAPI, err := keystonev1.GetKeystoneAPI(ctx, helper, instance.Namespace, map[string]string{})
		if err != nil {
			return ctrl.Result{}, err
		}
		keystoneEndpoints.Internal, err = keystoneAPI.GetEndpoint(endpoint.EndpointInternal)
		if err != nil {
			return ctrl.Result{}, err
		}
		keystoneEndpoints.Public, err = keystoneAPI.GetEndpoint(endpoint.EndpointPublic)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	//
	// create service DB instance
	//
	db, result, err := r.ensureDB(ctx, helper, instance)
	if err != nil {
		return ctrl.Result{}, err
	} else if (result != ctrl.Result{}) {
		return result, nil
	}
	// create service DB - end

	//
	// Create ConfigMaps and Secrets required as input for the Service and calculate an overall hash of hashes
	//

	//
	// create Configmap required for ironic input
	// - %-scripts configmap holding scripts to e.g. bootstrap the service
	// - %-config configmap holding minimal ironic config required to get the service up, user can add additional files to be added to the service
	// - parameters which has passwords gets added from the OpenStack secret via the init container
	//
	err = r.generateServiceConfigMaps(ctx, instance, helper, &configMapVars, &keystoneEndpoints, db)
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
	// create hash over all the different input resources to identify if any those changed
	// and a restart/recreate is required.
	//
	_, hashChanged, err := r.createHashOfInputHashes(ctx, instance, configMapVars)
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
	// Create ConfigMaps and Secrets - end

	instance.Status.Conditions.MarkTrue(condition.ServiceConfigReadyCondition, condition.ServiceConfigReadyMessage)

	//
	// TODO check when/if Init, Update, or Upgrade should/could be skipped
	//

	serviceLabels := map[string]string{
		common.AppSelector: ironic.ServiceName,
	}

	// Handle service update
	ctrlResult, err := r.reconcileUpdate()
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// Handle service upgrade
	ctrlResult, err = r.reconcileUpgrade(ctx, instance, helper, serviceLabels)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	//
	// normal reconcile tasks
	//

	// TODO: Should validate and refuse to continue if instance.Spec.IronicConductors
	//       container multiple elements with the same ConductorGroup defined.
	// deploy ironic-conductors
	for _, conductorSpec := range instance.Spec.IronicConductors {

		ironicConductor, op, err := r.conductorDeploymentCreateOrUpdate(
			instance,
			conductorSpec,
			&keystoneEndpoints,
		)
		if err != nil {
			instance.Status.Conditions.Set(condition.FalseCondition(
				ironicv1.IronicConductorReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				ironicv1.IronicConductorReadyErrorMessage,
				err.Error()))
			return ctrl.Result{}, err
		}

		// Check the observed Generation and mirror the condition from the
		// underlying resource reconciliation
		cndObsGen, err := r.checkIronicConductorGeneration(instance)
		if err != nil {
			instance.Status.Conditions.Set(condition.FalseCondition(
				ironicv1.IronicConductorReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				ironicv1.IronicConductorReadyErrorMessage,
				err.Error()))
			return ctrl.Result{}, err
		}
		if !cndObsGen {
			instance.Status.Conditions.Set(condition.UnknownCondition(
				ironicv1.IronicConductorReadyCondition,
				condition.InitReason,
				ironicv1.IronicConductorReadyInitMessage,
			))
		} else {
			// Mirror IronicConductor status' ReadyCount to this parent CR
			condGrp := conductorSpec.ConductorGroup
			if conductorSpec.ConductorGroup == "" {
				condGrp = ironicv1.ConductorGroupNull
			}
			instance.Status.IronicConductorReadyCount[condGrp] = ironicConductor.Status.ReadyCount
			// Mirror IronicConductor's condition status
			c := ironicConductor.Status.Conditions.Mirror(ironicv1.IronicConductorReadyCondition)
			if c != nil {
				instance.Status.Conditions.Set(c)
			}
			if op != controllerutil.OperationResultNone {
				Log.Info(fmt.Sprintf("Deployment %s successfully reconciled - operation: %s", ironicConductor.Name, string(op)))
			}
		}
	}

	// deploy ironic-api
	ironicAPI, op, err := r.apiDeploymentCreateOrUpdate(instance, &keystoneEndpoints)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			ironicv1.IronicAPIReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			ironicv1.IronicAPIReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

	// Check the observed Generation and mirror the condition from the
	// underlying resource reconciliation
	apiObsGen, err := r.checkIronicAPIGeneration(instance)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			ironicv1.IronicAPIReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			ironicv1.IronicAPIReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	// Only mirror the underlying condition if the observedGeneration is
	// the last seen
	if !apiObsGen {
		instance.Status.Conditions.Set(condition.UnknownCondition(
			ironicv1.IronicAPIReadyCondition,
			condition.InitReason,
			ironicv1.IronicAPIReadyInitMessage,
		))
	} else {
		// Mirror IronicAPI status' APIEndpoints and ReadyCount to this parent CR
		for k, v := range ironicAPI.Status.APIEndpoints {
			instance.Status.APIEndpoints[k] = v
		}
		instance.Status.IronicAPIReadyCount = ironicAPI.Status.ReadyCount

		// Mirror IronicAPI's condition status
		c := ironicAPI.Status.Conditions.Mirror(ironicv1.IronicAPIReadyCondition)
		if c != nil {
			instance.Status.Conditions.Set(c)
		}
		if op != controllerutil.OperationResultNone {
			Log.Info(fmt.Sprintf("Deployment %s successfully reconciled - operation: %s", ironicAPI.Name, string(op)))
		}
	}

	// deploy ironic-inspector
	if *(instance.Spec.IronicInspector.Replicas) != 0 {
		ironicInspector, op, err := r.inspectorDeploymentCreateOrUpdate(instance)
		if err != nil {
			instance.Status.Conditions.Set(
				condition.FalseCondition(
					ironicv1.IronicInspectorReadyCondition,
					condition.ErrorReason,
					condition.SeverityWarning,
					ironicv1.IronicInspectorReadyErrorMessage,
					err.Error()))
			return ctrl.Result{}, err
		}

		// Check the observed Generation and mirror the condition from the
		// underlying resource reconciliation
		nspObsGen, err := r.checkIronicInspectorGeneration(instance)
		if err != nil {
			instance.Status.Conditions.Set(
				condition.FalseCondition(
					ironicv1.IronicInspectorReadyCondition,
					condition.ErrorReason,
					condition.SeverityWarning,
					ironicv1.IronicInspectorReadyErrorMessage,
					err.Error()))
			return ctrl.Result{}, err
		}
		// Only mirror the underlying condition if the observedGeneration is
		// the last seen
		if !nspObsGen {
			instance.Status.Conditions.Set(condition.UnknownCondition(
				ironicv1.IronicInspectorReadyCondition,
				condition.InitReason,
				ironicv1.IronicInspectorReadyInitMessage,
			))
		} else {
			// Mirror IronicInspector status APIEndpoints and ReadyCount to this parent CR
			for k, v := range ironicInspector.Status.APIEndpoints {
				instance.Status.APIEndpoints[k] = v
			}
			instance.Status.InspectorReadyCount = ironicInspector.Status.ReadyCount

			// Mirror IronicInspector's condition status
			c := ironicInspector.Status.Conditions.Mirror(ironicv1.IronicInspectorReadyCondition)
			if c != nil {
				instance.Status.Conditions.Set(c)
			}
			if op != controllerutil.OperationResultNone {
				Log.Info(fmt.Sprintf("Deployment %s successfully reconciled - operation: %s", ironicInspector.Name, string(op)))
			}
		}
	} else {
		err := r.inspectorDeploymentDelete(ctx, instance)
		if err != nil {
			instance.Status.Conditions.Set(
				condition.FalseCondition(
					ironicv1.IronicInspectorReadyCondition,
					condition.ErrorReason,
					condition.SeverityWarning,
					ironicv1.IronicInspectorReadyErrorMessage,
					err.Error()))
			return ctrl.Result{}, err
		}
		// TODO: We do not have a specific message for not-requested services
		instance.Status.Conditions.MarkTrue(ironicv1.IronicInspectorReadyCondition, "")
	}

	// deploy ironic-neutron-agent (ML2 baremetal agent)
	if *(instance.Spec.IronicNeutronAgent.Replicas) != 0 {
		ironicNeutronAgent, op, err := r.ironicNeutronAgentDeploymentCreateOrUpdate(instance)
		if err != nil {
			instance.Status.Conditions.Set(
				condition.FalseCondition(
					ironicv1.IronicNeutronAgentReadyCondition,
					condition.ErrorReason,
					condition.SeverityWarning,
					ironicv1.IronicNeutronAgentReadyErrorMessage,
					err.Error()))
			return ctrl.Result{}, err
		}

		// Check the observed Generation and mirror the condition from the
		// underlying resource reconciliation
		agObsGen, err := r.checkNeutronAgentGeneration(instance)
		if err != nil {
			instance.Status.Conditions.Set(
				condition.FalseCondition(
					ironicv1.IronicNeutronAgentReadyCondition,
					condition.ErrorReason,
					condition.SeverityWarning,
					ironicv1.IronicNeutronAgentReadyErrorMessage,
					err.Error()))
			return ctrl.Result{}, err
		}
		// Only mirror the underlying condition if the observedGeneration is
		// the last seen
		if !agObsGen {
			instance.Status.Conditions.Set(condition.UnknownCondition(
				ironicv1.IronicNeutronAgentReadyCondition,
				condition.InitReason,
				ironicv1.IronicNeutronAgentReadyInitMessage,
			))
		} else {
			// Mirror IronicNeutronAgent status ReadyCount to this parent CR
			instance.Status.IronicNeutronAgentReadyCount = ironicNeutronAgent.Status.ReadyCount
			// Mirror IronicNeutronAgent's condition status
			c := ironicNeutronAgent.Status.Conditions.Mirror(ironicv1.IronicNeutronAgentReadyCondition)
			if c != nil {
				instance.Status.Conditions.Set(c)
			}
			if op != controllerutil.OperationResultNone {
				Log.Info(fmt.Sprintf("Deployment %s successfully reconciled - operation: %s", ironicNeutronAgent.Name, string(op)))
			}
		}
	} else {
		err := r.ironicNeutronAgentDeploymentDelete(ctx, instance)
		if err != nil {
			instance.Status.Conditions.Set(
				condition.FalseCondition(
					ironicv1.IronicNeutronAgentReadyCondition,
					condition.ErrorReason,
					condition.SeverityWarning,
					ironicv1.IronicNeutronAgentReadyErrorMessage,
					err.Error()))
			return ctrl.Result{}, err
		}
		// TODO: We do not have a specific message for not-requested services
		instance.Status.Conditions.MarkTrue(ironicv1.IronicNeutronAgentReadyCondition, "")
	}

	err = mariadbv1.DeleteUnusedMariaDBAccountFinalizers(ctx, helper, ironic.DatabaseCRName, instance.Spec.DatabaseAccount, instance.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	// We reached the end of the Reconcile, update the Ready condition based on
	// the sub conditions
	if instance.Status.Conditions.AllSubConditionIsTrue() {
		instance.Status.Conditions.MarkTrue(
			condition.ReadyCondition, condition.ReadyMessage)
	}
	Log.Info("Reconciled Ironic successfully")
	return ctrl.Result{}, nil
}

func (r *IronicReconciler) reconcileUpdate() (ctrl.Result, error) {
	// Log.Info("Reconciling Ironic update")

	// Log.Info("Reconciled Ironic update successfully")
	return ctrl.Result{}, nil
}

func (r *IronicReconciler) reconcileUpgrade(
	ctx context.Context,
	instance *ironicv1.Ironic,
	helper *helper.Helper,
	serviceLabels map[string]string,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)

	Log.Info("Reconciling Ironic upgrade")

	//
	// run ironic db sync
	//
	dbSyncHash := instance.Status.Hash[ironicv1.DbSyncHash]
	jobDef := ironic.DbSyncJob(instance, serviceLabels)
	dbSyncjob := job.NewJob(
		jobDef,
		ironicv1.DbSyncHash,
		instance.Spec.PreserveJobs,
		time.Second*5,
		dbSyncHash,
	)
	ctrlResult, err := dbSyncjob.DoJob(
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
		Log.Info(fmt.Sprintf("Job %s hash added - %s", jobDef.Name, instance.Status.Hash[ironicv1.DbSyncHash]))
	}
	instance.Status.Conditions.MarkTrue(condition.DBSyncReadyCondition, condition.DBSyncReadyMessage)

	// run ironic db sync - end

	Log.Info("Reconciled Ironic upgrade successfully")
	return ctrl.Result{}, nil
}

func (r *IronicReconciler) conductorDeploymentCreateOrUpdate(
	instance *ironicv1.Ironic,
	conductorSpec ironicv1.IronicConductorTemplate,
	keystoneEndpoints *ironicv1.KeystoneEndpoints,
) (*ironicv1.IronicConductor, controllerutil.OperationResult, error) {
	name := fmt.Sprintf("%s-%s", instance.Name, ironic.ConductorComponent)
	if conductorSpec.ConductorGroup != "" {
		name = strings.ToLower(fmt.Sprintf("%s-%s", name, conductorSpec.ConductorGroup))
	}

	IronicConductorSpec := ironicv1.IronicConductorSpec{
		IronicConductorTemplate: conductorSpec,
		ContainerImage:          instance.Spec.Images.Conductor,
		PxeContainerImage:       instance.Spec.Images.Pxe,
		IronicPythonAgentImage:  instance.Spec.Images.IronicPythonAgent,
		Standalone:              instance.Spec.Standalone,
		RPCTransport:            instance.Spec.RPCTransport,
		Secret:                  instance.Spec.Secret,
		PasswordSelectors:       instance.Spec.PasswordSelectors,
		ServiceUser:             instance.Spec.ServiceUser,
		DatabaseAccount:         instance.Spec.DatabaseAccount,
		DatabaseHostname:        instance.Status.DatabaseHostname,
		TransportURLSecret:      instance.Status.TransportURLSecret,
		KeystoneEndpoints:       *keystoneEndpoints,
		TLS:                     instance.Spec.IronicAPI.TLS.Ca,
	}

	if IronicConductorSpec.NodeSelector == nil {
		IronicConductorSpec.NodeSelector = instance.Spec.NodeSelector
	}

	// If topology is not present in the underlying IronicConductorSpec Spec,
	// inherit from the top-level CR
	if IronicConductorSpec.TopologyRef == nil {
		IronicConductorSpec.TopologyRef = instance.Spec.TopologyRef
	}
	deployment := &ironicv1.IronicConductor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(context.TODO(), r.Client, deployment, func() error {
		deployment.Spec = IronicConductorSpec
		if deployment.Spec.StorageClass == "" {
			deployment.Spec.StorageClass = instance.Spec.StorageClass
		}
		err := controllerutil.SetControllerReference(instance, deployment, r.Scheme)
		if err != nil {
			return err
		}

		return nil
	})

	return deployment, op, err
}

func (r *IronicReconciler) apiDeploymentCreateOrUpdate(
	instance *ironicv1.Ironic,
	keystoneEndpoints *ironicv1.KeystoneEndpoints,
) (*ironicv1.IronicAPI, controllerutil.OperationResult, error) {
	IronicAPISpec := ironicv1.IronicAPISpec{
		IronicAPITemplate:  instance.Spec.IronicAPI,
		ContainerImage:     instance.Spec.Images.API,
		Standalone:         instance.Spec.Standalone,
		RPCTransport:       instance.Spec.RPCTransport,
		Secret:             instance.Spec.Secret,
		PasswordSelectors:  instance.Spec.PasswordSelectors,
		ServiceUser:        instance.Spec.ServiceUser,
		DatabaseAccount:    instance.Spec.DatabaseAccount,
		DatabaseHostname:   instance.Status.DatabaseHostname,
		TransportURLSecret: instance.Status.TransportURLSecret,
		KeystoneEndpoints:  *keystoneEndpoints,
	}

	if IronicAPISpec.NodeSelector == nil {
		IronicAPISpec.NodeSelector = instance.Spec.NodeSelector
	}

	// If topology is not present in the underlying IronicAPI Spec,
	// inherit from the top-level CR
	if IronicAPISpec.TopologyRef == nil {
		IronicAPISpec.TopologyRef = instance.Spec.TopologyRef
	}

	if IronicAPISpec.APITimeout == 0 {
		IronicAPISpec.APITimeout = instance.Spec.APITimeout
	}

	deployment := &ironicv1.IronicAPI{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-api", instance.Name),
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(context.TODO(), r.Client, deployment, func() error {
		deployment.Spec = IronicAPISpec
		err := controllerutil.SetControllerReference(instance, deployment, r.Scheme)
		if err != nil {
			return err
		}

		return nil
	})

	return deployment, op, err
}

// generateServiceConfigMaps - create create configmaps which hold scripts and service configuration
// TODO add DefaultConfigOverwrite
func (r *IronicReconciler) generateServiceConfigMaps(
	ctx context.Context,
	instance *ironicv1.Ironic,
	h *helper.Helper,
	envVars *map[string]env.Setter,
	keystoneEndpoints *ironicv1.KeystoneEndpoints,
	db *mariadbv1.Database,
) error {
	//
	// create Configmap/Secret required for ironic input
	// - %-scripts configmap holding scripts to e.g. bootstrap the service
	// - %-config configmap holding minimal ironic config required to get the service up, user can add additional files to be added to the service
	// - parameters which has passwords gets added from the ospSecret via the init container
	//

	cmLabels := labels.GetLabels(instance, labels.GetGroupLabel(ironic.ServiceName), map[string]string{})

	var tlsCfg *tls.Service
	if instance.Spec.IronicAPI.TLS.Ca.CaBundleSecretName != "" {
		tlsCfg = &tls.Service{}
	}

	// customData hold any customization for the service.
	// custom.conf is going to /etc/ironic/ironic.conf.d
	// all other files get placed into /etc/ironic to allow overwrite of e.g. policy.json
	// TODO: make sure custom.conf can not be overwritten
	customData := map[string]string{
		"01-ironic-custom.conf": instance.Spec.CustomServiceConfig,
		"my.cnf":                db.GetDatabaseClientConfig(tlsCfg), //(mschuppert) for now just get the default my.cnf

	}
	for key, data := range instance.Spec.DefaultConfigOverwrite {
		customData[key] = data
	}

	templateParameters := make(map[string]interface{})
	// Initialize ConductorGroup key to ensure template rendering does not fail
	templateParameters["ConductorGroup"] = nil

	if !instance.Spec.Standalone {
		templateParameters["KeystoneInternalURL"] = keystoneEndpoints.Internal
		templateParameters["KeystonePublicURL"] = keystoneEndpoints.Public
		templateParameters["ServiceUser"] = instance.Spec.ServiceUser
	} else {
		templateParameters["IronicPublicURL"] = ""
	}
	templateParameters["Standalone"] = instance.Spec.Standalone
	templateParameters["LogPath"] = ironic.LogPath
	templateParameters["APITimeout"] = instance.Spec.APITimeout

	databaseAccount := db.GetAccount()
	dbSecret := db.GetSecret()

	templateParameters["DatabaseConnection"] = fmt.Sprintf("mysql+pymysql://%s:%s@%s/%s?read_default_file=/etc/my.cnf",
		databaseAccount.Spec.UserName,
		string(dbSecret.Data[mariadbv1.DatabasePasswordSelector]),
		instance.Status.DatabaseHostname,
		ironic.DatabaseName,
	)

	cms := []util.Template{
		// Scripts ConfigMap
		{
			Name:         fmt.Sprintf("%s-scripts", instance.Name),
			Namespace:    instance.Namespace,
			Type:         util.TemplateTypeScripts,
			InstanceType: instance.Kind,
			AdditionalTemplate: map[string]string{
				"common.sh": "/common/bin/common.sh",
				"init.sh":   "/common/bin/ironic-init.sh",
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
			AdditionalTemplate: map[string]string{
				"ironic.conf": "/common/config/ironic.conf",
			},
			Labels: cmLabels,
		},
	}

	return oko_secret.EnsureSecrets(ctx, h, instance, cms, envVars)
}

// createHashOfInputHashes - creates a hash of hashes which gets added to the resources which requires a restart
// if any of the input resources change, like configs, passwords, ...
//
// returns the hash, whether the hash changed (as a bool) and any error
func (r *IronicReconciler) createHashOfInputHashes(
	ctx context.Context,
	instance *ironicv1.Ironic,
	envVars map[string]env.Setter,
) (string, bool, error) {
	Log := r.GetLogger(ctx)

	var hashMap map[string]string
	changed := false
	mergedMapVars := env.MergeEnvs([]corev1.EnvVar{}, envVars)
	hash, err := util.ObjectHash(mergedMapVars)
	if err != nil {
		return hash, changed, err
	}
	if hashMap, changed = util.SetHash(instance.Status.Hash, common.InputHashName, hash); changed {
		instance.Status.Hash = hashMap
		Log.Info(fmt.Sprintf("Input maps hash %s - %s", common.InputHashName, hash))
	}
	return hash, changed, nil
}

func (r *IronicReconciler) inspectorDeploymentCreateOrUpdate(
	instance *ironicv1.Ironic,
) (*ironicv1.IronicInspector, controllerutil.OperationResult, error) {
	// TODO(tkajinam): Should we support using seprate DB/MQ for inspector ?
	IronicInspectorSpec := ironicv1.IronicInspectorSpec{
		IronicInspectorTemplate: instance.Spec.IronicInspector,
		ContainerImage:          instance.Spec.Images.Inspector,
		PxeContainerImage:       instance.Spec.Images.Pxe,
		IronicPythonAgentImage:  instance.Spec.Images.IronicPythonAgent,
		Standalone:              instance.Spec.Standalone,
		RPCTransport:            instance.Spec.RPCTransport,
		DatabaseInstance:        instance.Spec.DatabaseInstance,
		RabbitMqClusterName:     instance.Spec.RabbitMqClusterName,
		Secret:                  instance.Spec.Secret,
	}

	if IronicInspectorSpec.NodeSelector == nil {
		IronicInspectorSpec.NodeSelector = instance.Spec.NodeSelector
	}

	// If topology is not present in the underlying IronicInspector Spec,
	// inherit from the top-level CR
	if IronicInspectorSpec.TopologyRef == nil {
		IronicInspectorSpec.TopologyRef = instance.Spec.TopologyRef
	}

	if IronicInspectorSpec.APITimeout == 0 {
		IronicInspectorSpec.APITimeout = instance.Spec.APITimeout
	}

	deployment := &ironicv1.IronicInspector{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-inspector", instance.Name),
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(
		context.TODO(), r.Client, deployment, func() error {
			deployment.Spec = IronicInspectorSpec
			err := controllerutil.SetControllerReference(
				instance, deployment, r.Scheme)
			if err != nil {
				return err
			}

			return nil
		})

	return deployment, op, err
}

func (r *IronicReconciler) inspectorDeploymentDelete(
	ctx context.Context,
	instance *ironicv1.Ironic,
) error {
	deployment := &ironicv1.IronicInspector{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-inspector", instance.Name),
			Namespace: instance.Namespace,
		},
	}
	err := controllerutil.SetControllerReference(instance, deployment, r.Scheme)
	if err != nil {
		return err
	}
	deploymentObjectKey := client.ObjectKeyFromObject(deployment)
	if err := r.Client.Get(ctx, deploymentObjectKey, deployment); err != nil {
		if k8s_errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if err := r.Client.Delete(ctx, deployment); err != nil {
		return err
	}
	// Remove inspector APIEndpoints, Services and set ReadyCount 0
	delete(instance.Status.APIEndpoints, "ironic-inspector")
	instance.Status.InspectorReadyCount = 0
	// Remove IronicInspectorReadyCondition
	instance.Status.Conditions.Remove(ironicv1.IronicInspectorReadyCondition)

	return nil
}

func (r *IronicReconciler) ironicNeutronAgentDeploymentCreateOrUpdate(
	instance *ironicv1.Ironic,
) (*ironicv1.IronicNeutronAgent, controllerutil.OperationResult, error) {
	IronicNeutronAgentSpec := ironicv1.IronicNeutronAgentSpec{
		IronicNeutronAgentTemplate: instance.Spec.IronicNeutronAgent,
		ContainerImage:             instance.Spec.Images.NeutronAgent,
		Secret:                     instance.Spec.Secret,
		PasswordSelectors:          instance.Spec.PasswordSelectors,
		ServiceUser:                instance.Spec.ServiceUser,
		TLS:                        instance.Spec.IronicAPI.TLS.Ca,
	}

	if IronicNeutronAgentSpec.NodeSelector == nil {
		IronicNeutronAgentSpec.NodeSelector = instance.Spec.NodeSelector
	}

	// If topology is not present in the underlying IronicNeutronAgent Spec,
	// inherit from the top-level CR
	if IronicNeutronAgentSpec.TopologyRef == nil {
		IronicNeutronAgentSpec.TopologyRef = instance.Spec.TopologyRef
	}

	deployment := &ironicv1.IronicNeutronAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-ironic-neutron-agent", instance.Name),
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(
		context.TODO(), r.Client, deployment, func() error {
			deployment.Spec = IronicNeutronAgentSpec
			err := controllerutil.SetControllerReference(
				instance, deployment, r.Scheme)
			if err != nil {
				return err
			}

			return nil
		})

	return deployment, op, err
}

func (r *IronicReconciler) ironicNeutronAgentDeploymentDelete(
	ctx context.Context,
	instance *ironicv1.Ironic,
) error {
	deployment := &ironicv1.IronicNeutronAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-ironic-neutron-agent", instance.Name),
			Namespace: instance.Namespace,
		},
	}
	err := controllerutil.SetControllerReference(instance, deployment, r.Scheme)
	if err != nil {
		return err
	}
	deploymentObjectKey := client.ObjectKeyFromObject(deployment)
	if err := r.Client.Get(ctx, deploymentObjectKey, deployment); err != nil {
		if k8s_errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if err := r.Client.Delete(ctx, deployment); err != nil {
		return err
	}
	// Set ReadyCount 0
	instance.Status.IronicNeutronAgentReadyCount = 0
	// Remove IronicNeutronAgentReadyCondition
	instance.Status.Conditions.Remove(ironicv1.IronicNeutronAgentReadyCondition)

	return nil
}

func (r *IronicReconciler) ensureDB(
	ctx context.Context,
	h *helper.Helper,
	instance *ironicv1.Ironic,
) (*mariadbv1.Database, ctrl.Result, error) {

	// ensure MariaDBAccount exists.  This account record may be created by
	// openstack-operator or the cloud operator up front without a specific
	// MariaDBDatabase configured yet.   Otherwise, a MariaDBAccount CR is
	// created here with a generated username as well as a secret with
	// generated password.   The MariaDBAccount is created without being
	// yet associated with any MariaDBDatabase.
	_, _, err := mariadbv1.EnsureMariaDBAccount(
		ctx, h, instance.Spec.DatabaseAccount,
		instance.Namespace, false, "ironic",
	)

	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			mariadbv1.MariaDBAccountReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			mariadbv1.MariaDBAccountNotReadyMessage,
			err.Error()))

		return nil, ctrl.Result{}, err
	}
	instance.Status.Conditions.MarkTrue(
		mariadbv1.MariaDBAccountReadyCondition,
		mariadbv1.MariaDBAccountReadyMessage,
	)

	//
	// create service DB instance
	//
	db := mariadbv1.NewDatabaseForAccount(
		instance.Spec.DatabaseInstance, // mariadb/galera service to target
		ironic.DatabaseName,            // name used in CREATE DATABASE in mariadb
		ironic.DatabaseCRName,          // CR name for MariaDBDatabase
		instance.Spec.DatabaseAccount,  // CR name for MariaDBAccount
		instance.Namespace,             // namespace
	)

	// create or patch the DB
	ctrlResult, err := db.CreateOrPatchAll(ctx, h)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DBReadyErrorMessage,
			err.Error()))
		return db, ctrl.Result{}, err
	}
	if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DBReadyRunningMessage))
		return db, ctrlResult, nil
	}
	// wait for the DB to be setup
	// (ksambor) should we use WaitForDBCreatedWithTimeout instead?
	ctrlResult, err = db.WaitForDBCreated(ctx, h)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DBReadyErrorMessage,
			err.Error()))
		return db, ctrlResult, err
	}
	if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DBReadyRunningMessage))
		return db, ctrlResult, nil
	}

	// update Status.DatabaseHostname, used to config the service
	instance.Status.DatabaseHostname = db.GetDatabaseHostname()
	instance.Status.Conditions.MarkTrue(condition.DBReadyCondition, condition.DBReadyMessage)
	return db, ctrlResult, nil
}

// checkIronicAPIGeneration -
func (r *IronicReconciler) checkIronicAPIGeneration(
	instance *ironicv1.Ironic,
) (bool, error) {
	Log := r.GetLogger(context.Background())
	api := &ironicv1.IronicAPIList{}
	listOpts := []client.ListOption{
		client.InNamespace(instance.Namespace),
	}
	if err := r.Client.List(context.Background(), api, listOpts...); err != nil {
		Log.Error(err, "Unable to retrieve IronicAPI CR %w")
		return false, err
	}
	for _, item := range api.Items {
		if item.Generation != item.Status.ObservedGeneration {
			return false, nil
		}
	}
	return true, nil
}

// checkIronicConductorGeneration -
func (r *IronicReconciler) checkIronicConductorGeneration(
	instance *ironicv1.Ironic,
) (bool, error) {
	Log := r.GetLogger(context.Background())
	cnd := &ironicv1.IronicConductorList{}
	listOpts := []client.ListOption{
		client.InNamespace(instance.Namespace),
	}
	if err := r.Client.List(context.Background(), cnd, listOpts...); err != nil {
		Log.Error(err, "Unable to retrieve IronicConductor CR %w")
		return false, err
	}
	for _, item := range cnd.Items {
		if item.Generation != item.Status.ObservedGeneration {
			return false, nil
		}
	}
	return true, nil
}

// checkIronicInspectorGeneration -
func (r *IronicReconciler) checkIronicInspectorGeneration(
	instance *ironicv1.Ironic,
) (bool, error) {
	Log := r.GetLogger(context.Background())
	nsp := &ironicv1.IronicInspectorList{}
	listOpts := []client.ListOption{
		client.InNamespace(instance.Namespace),
	}
	if err := r.Client.List(context.Background(), nsp, listOpts...); err != nil {
		Log.Error(err, "Unable to retrieve IronicInspector CR %w")
		return false, err
	}
	for _, item := range nsp.Items {
		if item.Generation != item.Status.ObservedGeneration {
			return false, nil
		}
	}
	return true, nil
}

// checkNeutronAgentGeneration -
func (r *IronicReconciler) checkNeutronAgentGeneration(
	instance *ironicv1.Ironic,
) (bool, error) {
	Log := r.GetLogger(context.Background())
	ag := &ironicv1.IronicNeutronAgentList{}
	listOpts := []client.ListOption{
		client.InNamespace(instance.Namespace),
	}
	if err := r.Client.List(context.Background(), ag, listOpts...); err != nil {
		Log.Error(err, "Unable to retrieve IronicNeutronAgent CR %w")
		return false, err
	}
	for _, item := range ag.Items {
		if item.Generation != item.Status.ObservedGeneration {
			return false, nil
		}
	}
	return true, nil
}
