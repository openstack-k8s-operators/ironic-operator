/*
Copyright 2023 Red Hat Inc.

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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	rabbitmqv1 "github.com/openstack-k8s-operators/infra-operator/apis/rabbitmq/v1beta1"
	ironicv1 "github.com/openstack-k8s-operators/ironic-operator/api/v1beta1"
	ironic "github.com/openstack-k8s-operators/ironic-operator/pkg/ironic"
	"github.com/openstack-k8s-operators/ironic-operator/pkg/ironicneutronagent"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	endpoint "github.com/openstack-k8s-operators/lib-common/modules/common/endpoint"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"

	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/deployment"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/labels"
	common_rbac "github.com/openstack-k8s-operators/lib-common/modules/common/rbac"
	"github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
)

// IronicNeutronAgentReconciler reconciles a IronicNeutronAgent object
type IronicNeutronAgentReconciler struct {
	client.Client
	Kclient kubernetes.Interface
	Scheme  *runtime.Scheme
}

// GetLogger returns a logger object with a prefix of "controller.name" and additional controller context fields
func (r *IronicNeutronAgentReconciler) GetLogger(ctx context.Context) logr.Logger {
	return log.FromContext(ctx).WithName("Controllers").WithName("IronicNeutronAgent")
}

// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironicneutronagents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironicneutronagents/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironicneutronagents/finalizers,verbs=update;patch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;create;update;patch;delete;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;create;update;patch;delete;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;create;update;patch;delete;watch
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneservices,verbs=get;list;watch
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneendpoints,verbs=get;list;watch

// service account, role, rolebinding
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=roles,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=rolebindings,verbs=get;list;watch;create;update;patch
// service account permissions that are needed to grant permission to the above
// +kubebuilder:rbac:groups="security.openshift.io",resourceNames=anyuid;privileged,resources=securitycontextconstraints,verbs=use
// +kubebuilder:rbac:groups="",resources=pods,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups=topology.openstack.org,resources=topologies,verbs=get;list;watch;update

// Reconcile -
func (r *IronicNeutronAgentReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (result ctrl.Result, _err error) {
	Log := r.GetLogger(ctx)

	// Fetch the IronicNeutronAgent instance
	instance := &ironicv1.IronicNeutronAgent{}
	err := r.Get(ctx, req.NamespacedName, instance)
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

	// Save a copy of the conditions so that we can restore the LastTransitionTime
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
	// initialize conditions used later as Status=Unknown
	cl := condition.CreateList(
		condition.UnknownCondition(
			condition.InputReadyCondition,
			condition.InitReason,
			condition.InputReadyInitMessage),
		condition.UnknownCondition(
			condition.RabbitMqTransportURLReadyCondition,
			condition.InitReason,
			condition.RabbitMqTransportURLReadyInitMessage),
		condition.UnknownCondition(
			condition.ServiceConfigReadyCondition,
			condition.InitReason,
			condition.ServiceConfigReadyInitMessage),
		condition.UnknownCondition(
			condition.DeploymentReadyCondition,
			condition.InitReason,
			condition.DeploymentReadyInitMessage),
		condition.UnknownCondition(
			condition.TLSInputReadyCondition,
			condition.InitReason,
			condition.InputReadyInitMessage),
		// service account, role, rolebinding conditions
		condition.UnknownCondition(
			condition.ServiceAccountReadyCondition,
			condition.InitReason,
			condition.ServiceAccountReadyInitMessage),
		condition.UnknownCondition(
			condition.RoleReadyCondition,
			condition.InitReason,
			condition.RoleReadyInitMessage),
		condition.UnknownCondition(
			condition.RoleBindingReadyCondition,
			condition.InitReason,
			condition.RoleBindingReadyInitMessage),
	)

	instance.Status.Conditions.Init(&cl)
	instance.Status.ObservedGeneration = instance.Generation

	// If we're not deleting this and the service object doesn't have our finalizer, add it.
	if instance.DeletionTimestamp.IsZero() && controllerutil.AddFinalizer(instance, helper.GetFinalizer()) || isNewInstance {
		return ctrl.Result{}, nil
	}

	if instance.Status.Hash == nil {
		instance.Status.Hash = map[string]string{}
	}

	// Init Topology condition if there's a reference
	if instance.Spec.TopologyRef != nil {
		c := condition.UnknownCondition(condition.TopologyReadyCondition, condition.InitReason, condition.TopologyReadyInitMessage)
		cl.Set(c)
	}

	// Handle service delete
	if !instance.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, instance, helper)
	}

	// Handle non-delete
	return r.reconcileNormal(ctx, instance, helper)
}

// SetupWithManager - sets up the controller with the Manager.
func (r *IronicNeutronAgentReconciler) SetupWithManager(
	ctx context.Context, mgr ctrl.Manager,
) error {
	// index passwordSecretField
	if err := mgr.GetFieldIndexer().IndexField(ctx, &ironicv1.IronicNeutronAgent{}, passwordSecretField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*ironicv1.IronicNeutronAgent)
		if cr.Spec.Secret == "" {
			return nil
		}
		return []string{cr.Spec.Secret}
	}); err != nil {
		return err
	}

	// index caBundleSecretNameField
	if err := mgr.GetFieldIndexer().IndexField(ctx, &ironicv1.IronicNeutronAgent{}, caBundleSecretNameField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*ironicv1.IronicNeutronAgent)
		if cr.Spec.TLS.CaBundleSecretName == "" {
			return nil
		}
		return []string{cr.Spec.TLS.CaBundleSecretName}
	}); err != nil {
		return err
	}

	// index topologyField
	if err := mgr.GetFieldIndexer().IndexField(ctx, &ironicv1.IronicNeutronAgent{}, topologyField, func(rawObj client.Object) []string {
		// Extract the topology name from the spec, if one is provided
		cr := rawObj.(*ironicv1.IronicNeutronAgent)
		if cr.Spec.TopologyRef == nil {
			return nil
		}
		return []string{cr.Spec.TopologyRef.Name}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&ironicv1.IronicNeutronAgent{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&rabbitmqv1.TransportURL{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectsForSrc),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(&topologyv1.Topology{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectsForSrc),
			builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&keystonev1.KeystoneAPI{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectForSrc),
			builder.WithPredicates(keystonev1.KeystoneAPIStatusChangedPredicate)).
		Complete(r)
}

func (r *IronicNeutronAgentReconciler) findObjectsForSrc(ctx context.Context, src client.Object) []reconcile.Request {
	requests := []reconcile.Request{}

	Log := r.GetLogger(ctx)

	for _, field := range ironicNeutronAgentWatchFields {
		crList := &ironicv1.IronicNeutronAgentList{}
		listOps := &client.ListOptions{
			FieldSelector: fields.OneTermEqualSelector(field, src.GetName()),
			Namespace:     src.GetNamespace(),
		}
		err := r.List(ctx, crList, listOps)
		if err != nil {
			Log.Error(err, fmt.Sprintf("listing %s for field: %s - %s", crList.GroupVersionKind().Kind, field, src.GetNamespace()))
			return requests
		}

		for _, item := range crList.Items {
			Log.Info(fmt.Sprintf("input source %s changed, reconcile: %s - %s", src.GetName(), item.GetName(), item.GetNamespace()))

			requests = append(requests,
				reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      item.GetName(),
						Namespace: item.GetNamespace(),
					},
				},
			)
		}
	}

	return requests
}

func (r *IronicNeutronAgentReconciler) findObjectForSrc(ctx context.Context, src client.Object) []reconcile.Request {
	requests := []reconcile.Request{}

	Log := r.GetLogger(ctx)

	crList := &ironicv1.IronicNeutronAgentList{}
	listOps := &client.ListOptions{
		Namespace: src.GetNamespace(),
	}
	err := r.List(ctx, crList, listOps)
	if err != nil {
		Log.Error(err, fmt.Sprintf("listing %s for namespace: %s", crList.GroupVersionKind().Kind, src.GetNamespace()))
		return requests
	}

	for _, item := range crList.Items {
		Log.Info(fmt.Sprintf("input source %s changed, reconcile: %s - %s", src.GetName(), item.GetName(), item.GetNamespace()))

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

func (r *IronicNeutronAgentReconciler) getTransportURL(
	ctx context.Context,
	h *helper.Helper,
	instance *ironicv1.IronicNeutronAgent,
) (string, error) {
	transportURLSecret, _, err := secret.GetSecret(ctx, h, instance.Status.TransportURLSecret, instance.Namespace)
	if err != nil {
		return "", err
	}
	transportURL, ok := transportURLSecret.Data["transport_url"]
	if !ok {
		return "", fmt.Errorf("transport_url %w Transport Secret", util.ErrNotFound)
	}
	return string(transportURL), nil
}

func (r *IronicNeutronAgentReconciler) reconcileTransportURL(
	ctx context.Context,
	instance *ironicv1.IronicNeutronAgent,
	helper *helper.Helper,
) (ctrl.Result, error) {
	// Create RabbitMQ transport URL CR and get the actual URL from the
	// associated secret that is created
	//
	Log := r.GetLogger(ctx)

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
		Log.Info(fmt.Sprintf(
			"TransportURL %s successfully reconciled - operation: %s",
			transportURL.Name, string(op)))
	}
	instance.Status.TransportURLSecret = transportURL.Status.SecretName
	if instance.Status.TransportURLSecret == "" {
		Log.Info(fmt.Sprintf(
			"Waiting for TransportURL %s secret to be created",
			transportURL.Name))
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.RabbitMqTransportURLReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.RabbitMqTransportURLReadyRunningMessage))
		return ctrl.Result{RequeueAfter: time.Second * 10}, nil
	}
	instance.Status.Conditions.MarkTrue(
		condition.RabbitMqTransportURLReadyCondition,
		condition.RabbitMqTransportURLReadyMessage)

	return ctrl.Result{}, nil
}

func (r *IronicNeutronAgentReconciler) reconcileConfigMapsAndSecrets(
	ctx context.Context,
	instance *ironicv1.IronicNeutronAgent,
	helper *helper.Helper,
) (ctrl.Result, string, error) {
	Log := r.GetLogger(ctx)
	// ConfigMap
	configMapVars := make(map[string]env.Setter)

	// check for required OpenStack secret holding passwords for service/admin user and add hash to the vars map
	ospSecret, hash, err := secret.GetSecret(ctx, helper, instance.Spec.Secret, instance.Namespace)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			Log.Info(fmt.Sprintf("OpenStack secret %s not found", instance.Spec.Secret))
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.InputReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				condition.InputReadyWaitingMessage))
			return ctrl.Result{RequeueAfter: time.Second * 10}, "", nil
		}
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.InputReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.InputReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, "", err
	}
	configMapVars[ospSecret.Name] = env.SetValue(hash)

	// check for required TransportURL secret and add hash to the vars map
	if instance.Status.TransportURLSecret != "" {
		transportURLSecret, hash, err := secret.GetSecret(ctx, helper, instance.Status.TransportURLSecret, instance.Namespace)
		if err != nil {
			if k8s_errors.IsNotFound(err) {
				Log.Info(fmt.Sprintf("TransportURL secret %s not found", instance.Status.TransportURLSecret))
				instance.Status.Conditions.Set(condition.FalseCondition(
					condition.InputReadyCondition,
					condition.RequestedReason,
					condition.SeverityInfo,
					condition.InputReadyWaitingMessage))
				return ctrl.Result{RequeueAfter: time.Second * 10}, "", nil
			}
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.InputReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.InputReadyErrorMessage,
				err.Error()))
			return ctrl.Result{}, "", err
		}
		configMapVars[transportURLSecret.Name] = env.SetValue(hash)
	}

	instance.Status.Conditions.MarkTrue(
		condition.InputReadyCondition,
		condition.InputReadyMessage)
	// run check secrets - end

	//
	// TLS input validation
	//
	// Validate the CA cert secret if provided
	if instance.Spec.TLS.CaBundleSecretName != "" {
		hash, err := tls.ValidateCACertSecret(
			ctx,
			helper.GetClient(),
			types.NamespacedName{
				Name:      instance.Spec.TLS.CaBundleSecretName,
				Namespace: instance.Namespace,
			},
		)
		if err != nil {
			if k8s_errors.IsNotFound(err) {
				instance.Status.Conditions.Set(condition.FalseCondition(
					condition.TLSInputReadyCondition,
					condition.RequestedReason,
					condition.SeverityInfo,
					condition.TLSInputReadyWaitingMessage, instance.Spec.TLS.CaBundleSecretName))
				return ctrl.Result{RequeueAfter: time.Second * 10}, "", nil
			}
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.TLSInputReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.TLSInputErrorMessage,
				err.Error()))
			return ctrl.Result{}, "", err
		}

		if hash != "" {
			configMapVars[tls.CABundleKey] = env.SetValue(hash)
		}
	}
	// all cert input checks out so report InputReady
	instance.Status.Conditions.MarkTrue(condition.TLSInputReadyCondition, condition.InputReadyMessage)

	// Create Secrets required as input for the Service and calculate an overall hash of hashes
	//

	//
	// create Secret required for ironicneutronagent input. It contains minimal ironicneutronagent config required
	// to get the service up, user can add additional files to be added to the service.
	err = r.generateServiceSecrets(ctx, helper, instance, &configMapVars)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ServiceConfigReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ServiceConfigReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, "", err
	}

	// Create ConfigMaps and Secrets - end

	// create hash over all the different input resources to identify if any those changed
	// and a restart/recreate is required.
	inputHash, hashChanged, err := r.createHashOfInputHashes(ctx, instance, configMapVars)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ServiceConfigReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ServiceConfigReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, "", err
	} else if hashChanged {
		// Hash changed and instance status should be updated (which will be done by main defer func),
		// so we need to return and reconcile again
		return ctrl.Result{RequeueAfter: time.Second * 5}, "", nil
	}
	instance.Status.Conditions.MarkTrue(
		condition.ServiceConfigReadyCondition,
		condition.ServiceConfigReadyMessage)

	return ctrl.Result{}, inputHash, nil
}

func (r *IronicNeutronAgentReconciler) reconcileDeployment(
	ctx context.Context,
	instance *ironicv1.IronicNeutronAgent,
	helper *helper.Helper,
	inputHash string,
	serviceLabels map[string]string,
) (ctrl.Result, error) {

	//
	// Handle Topology
	//
	topology, err := ensureTopology(
		ctx,
		helper,
		instance,      // topologyHandler
		instance.Name, // finalizer
		&instance.Status.Conditions,
		labels.GetLabelSelector(serviceLabels),
	)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.TopologyReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.TopologyReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, fmt.Errorf("waiting for Topology requirements: %w", err)
	}

	// Define a new Deployment object
	deplomentDef := ironicneutronagent.Deployment(
		instance,
		inputHash,
		serviceLabels,
		topology,
	)
	depl := deployment.NewDeployment(deplomentDef, 5)
	ctrlResult, err := depl.CreateOrPatch(ctx, helper)
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

	// Only check ReadyCount if controller sees the last version of the CR
	deploy := depl.GetDeployment()
	if deploy.Generation == deploy.Status.ObservedGeneration {
		instance.Status.ReadyCount = deploy.Status.ReadyReplicas

		// Mark the Deployment as Ready only if the number of Replicas is equals
		// to the Deployed instances (ReadyCount), and the the Status.Replicas
		// match Status.ReadyReplicas. If a deployment update is in progress,
		// Replicas > ReadyReplicas.
		if deployment.IsReady(deploy) {
			instance.Status.Conditions.MarkTrue(condition.DeploymentReadyCondition, condition.DeploymentReadyMessage)
		} else {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.DeploymentReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				condition.DeploymentReadyRunningMessage))
		}
	}

	return ctrl.Result{}, nil
}

func (r *IronicNeutronAgentReconciler) reconcileNormal(
	ctx context.Context,
	instance *ironicv1.IronicNeutronAgent,
	helper *helper.Helper,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info("Reconciling IronicNeutronAgent")

	if ironicv1.GetOwningIronicName(instance) == "" {
		// Service account, role, binding
		rbacResult, err := common_rbac.ReconcileRbac(ctx, helper, instance, getCommonRbacRules())
		if err != nil {
			return rbacResult, err
		} else if (rbacResult != ctrl.Result{}) {
			return rbacResult, nil
		}
	} else {
		ctrlResult, err := r.checkParentResourceExist(ctx, instance, helper)
		if err != nil {
			return ctrlResult, err
		} else if (ctrlResult != ctrl.Result{}) {
			return ctrlResult, nil
		}

		ctrlResult, err = r.checkParentRbacConditions(ctx, instance, helper)
		if err != nil {
			return ctrlResult, err
		} else if (ctrlResult != ctrl.Result{}) {
			return ctrlResult, nil
		}
	}

	ctrlResult, err := r.reconcileTransportURL(ctx, instance, helper)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	ctrlResult, inputHash, err := r.reconcileConfigMapsAndSecrets(ctx, instance, helper)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	serviceLabels := map[string]string{
		common.AppSelector:       ironicneutronagent.ServiceName,
		common.ComponentSelector: ironicneutronagent.ServiceName,
	}

	// Handle service init
	ctrlResult, err = r.reconcileInit(ctx)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	//
	// normal reconcile tasks
	//
	ctrlResult, err = r.reconcileDeployment(ctx, instance, helper, inputHash, serviceLabels)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// We reached the end of the Reconcile, update the Ready condition based on
	// the sub conditions
	if instance.Status.Conditions.AllSubConditionIsTrue() {
		instance.Status.Conditions.MarkTrue(
			condition.ReadyCondition, condition.ReadyMessage)
	}
	Log.Info("Reconciled IronicNeutronAgent Service successfully")
	return ctrl.Result{}, nil
}

func (r *IronicNeutronAgentReconciler) reconcileInit(
	ctx context.Context,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)

	Log.Info("Reconciling IronicNeutronAgent init")
	Log.Info("Reconciled IronicNeutronAgent init successfully")

	return ctrl.Result{}, nil
}

func (r *IronicNeutronAgentReconciler) reconcileDelete(
	ctx context.Context,
	instance *ironicv1.IronicNeutronAgent,
	helper *helper.Helper,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)

	// Remove finalizer on the Topology CR
	if ctrlResult, err := topologyv1.EnsureDeletedTopologyRef(
		ctx,
		helper,
		instance.Status.LastAppliedTopology,
		instance.Name,
	); err != nil {
		return ctrlResult, err
	}
	Log.Info("Reconciling IronicNeutronAgent delete")
	// Service is deleted so remove the finalizer.
	controllerutil.RemoveFinalizer(instance, helper.GetFinalizer())
	Log.Info("Reconciled IronicNeutronAgent delete successfully")

	return ctrl.Result{}, nil
}

// generateServiceSecrets - create secrets which service configuration
func (r *IronicNeutronAgentReconciler) generateServiceSecrets(
	ctx context.Context,
	h *helper.Helper,
	instance *ironicv1.IronicNeutronAgent,
	envVars *map[string]env.Setter,
) error {
	// Create/update secrets from templates
	cmLabels := labels.GetLabels(instance, labels.GetGroupLabel(ironicneutronagent.ServiceName), map[string]string{})

	// customData hold any customization for the service.
	// 02-ironic_neutron_agent-custom.conf is going to /etc/neutron/neutron.conf.d
	// 01-ironic_neutron_agent.conf is going to /etc/neutron/neutron.conf.d such that it gets loaded before custom one
	customData := map[string]string{
		"02-ironic_neutron_agent-custom.conf": instance.Spec.CustomServiceConfig,
	}
	for key, data := range instance.Spec.DefaultConfigOverwrite {
		customData[key] = data
	}

	keystoneAPI, err := keystonev1.GetKeystoneAPI(ctx, h, instance.Namespace, map[string]string{})
	if err != nil {
		return err
	}
	keystoneInternalURL, err := keystoneAPI.GetEndpoint(endpoint.EndpointInternal)
	if err != nil {
		return err
	}
	keystonePublicURL, err := keystoneAPI.GetEndpoint(endpoint.EndpointPublic)
	if err != nil {
		return err
	}

	transportURL, err := r.getTransportURL(ctx, h, instance)
	if err != nil {
		return err
	}

	quorumQueues, err := getQuorumQueues(ctx, h, instance.Status.TransportURLSecret, instance.Namespace)
	if err != nil {
		return err
	}

	ospSecret, _, err := secret.GetSecret(ctx, h, instance.Spec.Secret, instance.Namespace)
	if err != nil {
		return err
	}

	templateParameters := make(map[string]interface{})
	templateParameters["ServiceUser"] = instance.Spec.ServiceUser
	templateParameters["KeystoneInternalURL"] = keystoneInternalURL
	templateParameters["KeystonePublicURL"] = keystonePublicURL
	templateParameters["TransportURL"] = transportURL
	templateParameters["QuorumQueues"] = quorumQueues

	// Other OpenStack services
	servicePassword := string(ospSecret.Data[instance.Spec.PasswordSelectors.Service])
	templateParameters["ServicePassword"] = servicePassword
	templateParameters["keystone_authtoken"] = servicePassword
	templateParameters["service_catalog"] = servicePassword
	templateParameters["ironic"] = servicePassword

	cms := []util.Template{
		{
			Name:          fmt.Sprintf("%s-config-data", instance.Name),
			Namespace:     instance.Namespace,
			Type:          util.TemplateTypeConfig,
			InstanceType:  instance.Kind,
			CustomData:    customData,
			Labels:        cmLabels,
			ConfigOptions: templateParameters,
		},
	}
	return secret.EnsureSecrets(ctx, h, instance, cms, envVars)
}

// createHashOfInputHashes - creates a hash of hashes which gets added to the resources which requires a restart
// if any of the input resources change, like configs, passwords, ...
//
// returns the hash, whether the hash changed (as a bool) and any error
func (r *IronicNeutronAgentReconciler) createHashOfInputHashes(
	ctx context.Context,
	instance *ironicv1.IronicNeutronAgent,
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

func (r *IronicNeutronAgentReconciler) checkParentResourceExist(
	ctx context.Context,
	instance *ironicv1.IronicNeutronAgent,
	helper *helper.Helper,
) (ctrl.Result, error) {
	parentName := ironicv1.GetOwningIronicName(instance)
	parentIronic := &ironicv1.Ironic{}

	rbacConditions := []condition.Type{
		condition.ServiceAccountReadyCondition,
		condition.RoleReadyCondition,
		condition.RoleBindingReadyCondition,
	}

	// checks for existence of parent resource
	err := helper.GetClient().Get(
		ctx,
		types.NamespacedName{
			Name:      parentName,
			Namespace: instance.Namespace,
		},
		parentIronic,
	)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			for _, condType := range rbacConditions {
				instance.RbacConditionsSet(condition.FalseCondition(
					condType,
					condition.RequestedReason,
					condition.SeverityInfo,
					"Parent Ironic resource not found"))
			}
			return ctrl.Result{RequeueAfter: time.Second * 10}, nil
		}
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *IronicNeutronAgentReconciler) checkParentRbacConditions(
	ctx context.Context,
	instance *ironicv1.IronicNeutronAgent,
	helper *helper.Helper,
) (ctrl.Result, error) {
	parentName := ironicv1.GetOwningIronicName(instance)
	parentIronic := &ironicv1.Ironic{}

	rbacConditions := []condition.Type{
		condition.ServiceAccountReadyCondition,
		condition.RoleReadyCondition,
		condition.RoleBindingReadyCondition,
	}

	err := helper.GetClient().Get(
		ctx,
		types.NamespacedName{
			Name:      parentName,
			Namespace: instance.Namespace,
		},
		parentIronic,
	)
	if err != nil {
		return ctrl.Result{}, err
	}

	allConditionsReady := true
	for _, condType := range rbacConditions {
		if parentCondition := parentIronic.Status.Conditions.Get(condType); parentCondition != nil {
			instance.RbacConditionsSet(parentCondition)
			if parentCondition.Status != corev1.ConditionTrue {
				allConditionsReady = false
			}
		} else {
			instance.RbacConditionsSet(condition.UnknownCondition(
				condType,
				condition.InitReason,
				"Parent RBAC condition not yet available"))
			allConditionsReady = false
		}
	}

	if !allConditionsReady {
		return ctrl.Result{RequeueAfter: time.Second * 10}, nil
	}

	return ctrl.Result{}, nil
}
