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

	"github.com/go-logr/logr"
	ironic "github.com/openstack-k8s-operators/ironic-operator/pkg/ironic"
	ironicinspector "github.com/openstack-k8s-operators/ironic-operator/pkg/ironicinspector"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"

	common "github.com/openstack-k8s-operators/lib-common/modules/common"
	endpoint "github.com/openstack-k8s-operators/lib-common/modules/common/endpoint"
	env "github.com/openstack-k8s-operators/lib-common/modules/common/env"
	job "github.com/openstack-k8s-operators/lib-common/modules/common/job"
	nad "github.com/openstack-k8s-operators/lib-common/modules/common/networkattachment"
	common_rbac "github.com/openstack-k8s-operators/lib-common/modules/common/rbac"
	oko_secret "github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
	"github.com/openstack-k8s-operators/lib-common/modules/common/statefulset"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	util "github.com/openstack-k8s-operators/lib-common/modules/common/util"

	routev1 "github.com/openshift/api/route/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	k8s_types "k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	labels "github.com/openstack-k8s-operators/lib-common/modules/common/labels"

	ironicv1 "github.com/openstack-k8s-operators/ironic-operator/api/v1beta1"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"

	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// IronicInspectorReconciler reconciles a IronicInspector object
type IronicInspectorReconciler struct {
	client.Client
	Kclient kubernetes.Interface
	Scheme  *runtime.Scheme
}

var inspectorKeystoneServices = []map[string]string{
	{
		"name": "ironic-inspector",
		"type": "baremetal-introspection",
		"desc": "OpenStack Baremetal-Introspection Service",
	},
}

// GetLogger returns a logger object with a prefix of "conroller.name" and aditional controller context fields
func (r *IronicInspectorReconciler) GetLogger(ctx context.Context) logr.Logger {
	return log.FromContext(ctx).WithName("Controllers").WithName("IronicInspector")
}

// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironicinspectors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironicinspectors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironicinspectors/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;create;update;patch;delete;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;create;update;patch;delete;watch
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;delete;
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;create;update;patch;delete;watch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;create;update;patch;delete;watch
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list
// +kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbdatabases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneapis,verbs=get;list;watch
// +kubebuilder:rbac:groups=rabbitmq.openstack.org,resources=transporturls,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=k8s.cni.cncf.io,resources=network-attachment-definitions,verbs=get;list;watch
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;create;update;patch;delete

// service account, role, rolebinding
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=roles,verbs=get;list;watch;create;update
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=rolebindings,verbs=get;list;watch;create;update
// service account permissions that are needed to grant permission to the above
// +kubebuilder:rbac:groups="security.openshift.io",resourceNames=anyuid;privileged,resources=securitycontextconstraints,verbs=use
// +kubebuilder:rbac:groups="",resources=pods,verbs=create;delete;get;list;patch;update;watch

// Reconcile -
func (r *IronicInspectorReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (result ctrl.Result, _err error) {
	Log := r.GetLogger(ctx)

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
		condition.UnknownCondition(
			condition.NetworkAttachmentsReadyCondition,
			condition.InitReason,
			condition.NetworkAttachmentsReadyInitMessage),
		condition.UnknownCondition(
			condition.RabbitMqTransportURLReadyCondition,
			condition.InitReason,
			condition.RabbitMqTransportURLReadyInitMessage),
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

	if !instance.Spec.Standalone {
		// right now we have no dedicated KeystoneServiceReadyInitMessage and KeystoneEndpointReadyInitMessage
		cl = append(cl,
			*condition.UnknownCondition(
				condition.KeystoneServiceReadyCondition,
				condition.InitReason,
				""),
			*condition.UnknownCondition(
				condition.KeystoneEndpointReadyCondition,
				condition.InitReason, ""),
		)
	}
	instance.Status.Conditions.Init(&cl)

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
func (r *IronicInspectorReconciler) SetupWithManager(
	ctx context.Context, mgr ctrl.Manager,
) error {
	// watch for configmap where the CM owner label AND the CR.Spec.ManagingCrName label matches
	configMapFn := func(ctx context.Context, o client.Object) []reconcile.Request {
		Log := r.GetLogger(ctx)

		result := []reconcile.Request{}

		// get all API CRs
		apis := &ironicv1.IronicInspectorList{}
		listOpts := []client.ListOption{
			client.InNamespace(o.GetNamespace()),
		}
		if err := r.Client.List(
			ctx,
			apis,
			listOpts...); err != nil {

			Log.Error(err, "Unable to retrieve API CRs %v")
			return nil
		}

		label := o.GetLabels()
		// TODO: Just trying to verify that the CM is owned by this CR's managing CR
		if l, ok := label[labels.GetOwnerNameLabelSelector(
			labels.GetGroupLabel(ironic.ServiceName))]; ok {
			for _, cr := range apis.Items {
				// return reconcil event for the CR where the CM owner label
				// AND the parentIronicName matches
				if l == ironicv1.GetOwningIronicName(&cr) {
					// return namespace and Name of CR
					name := client.ObjectKey{
						Namespace: o.GetNamespace(),
						Name:      cr.Name,
					}
					Log.Info(fmt.Sprintf(
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

	// index passwordSecretField
	if err := mgr.GetFieldIndexer().IndexField(ctx, &ironicv1.IronicInspector{}, passwordSecretField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*ironicv1.IronicInspector)
		if cr.Spec.Secret == "" {
			return nil
		}
		return []string{cr.Spec.Secret}
	}); err != nil {
		return err
	}

	// index caBundleSecretNameField
	if err := mgr.GetFieldIndexer().IndexField(ctx, &ironicv1.IronicInspector{}, caBundleSecretNameField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*ironicv1.IronicInspector)
		if cr.Spec.TLS.CaBundleSecretName == "" {
			return nil
		}
		return []string{cr.Spec.TLS.CaBundleSecretName}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&ironicv1.IronicInspector{}).
		Owns(&corev1.Secret{}).
		Owns(&routev1.Route{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Owns(&appsv1.StatefulSet{}).
		// watch the config CMs we don't own
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(configMapFn)).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectsForSrc),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Complete(r)
}

func (r *IronicInspectorReconciler) findObjectsForSrc(ctx context.Context, src client.Object) []reconcile.Request {
	requests := []reconcile.Request{}

	l := log.FromContext(ctx).WithName("Controllers").WithName("IronicInspector")

	for _, field := range ironicInspectorWatchFields {
		crList := &ironicv1.IronicInspectorList{}
		listOps := &client.ListOptions{
			FieldSelector: fields.OneTermEqualSelector(field, src.GetName()),
			Namespace:     src.GetNamespace(),
		}
		err := r.List(ctx, crList, listOps)
		if err != nil {
			return []reconcile.Request{}
		}

		for _, item := range crList.Items {
			l.Info(fmt.Sprintf("input source %s changed, reconcile: %s - %s", src.GetName(), item.GetName(), item.GetNamespace()))

			requests = append(requests,
				reconcile.Request{
					NamespacedName: k8s_types.NamespacedName{
						Name:      item.GetName(),
						Namespace: item.GetNamespace(),
					},
				},
			)
		}
	}

	return requests
}

func (r *IronicInspectorReconciler) reconcileTransportURL(
	ctx context.Context,
	instance *ironicv1.IronicInspector,
	helper *helper.Helper,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)

	if instance.Spec.RPCTransport == "oslo" {
		//
		// Create RabbitMQ transport URL CR and get the actual URL from the
		// associted secret that is created
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
			return ctrl.Result{}, nil
		}

		instance.Status.Conditions.MarkTrue(
			condition.RabbitMqTransportURLReadyCondition,
			condition.RabbitMqTransportURLReadyMessage)
	} else {
		instance.Status.TransportURLSecret = ""
		instance.Status.Conditions.MarkTrue(
			condition.RabbitMqTransportURLReadyCondition,
			ironicv1.RabbitMqTransportURLDisabledMessage)
	}
	// transportURL - end

	return ctrl.Result{}, nil
}

func (r *IronicInspectorReconciler) reconcileConfigMapsAndSecrets(
	ctx context.Context,
	instance *ironicv1.IronicInspector,
	helper *helper.Helper,
	db *mariadbv1.Database,
) (ctrl.Result, string, error) {
	// ConfigMap
	configMapVars := make(map[string]env.Setter)

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
			return ctrl.Result{RequeueAfter: time.Second * 10}, "",
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
		return ctrl.Result{}, "", err
	}
	configMapVars[ospSecret.Name] = env.SetValue(hash)

	instance.Status.Conditions.MarkTrue(
		condition.InputReadyCondition,
		condition.InputReadyMessage)
	// run check OpenStack secret - end

	//
	// TLS input validation
	//
	// Validate the CA cert secret if provided
	if instance.Spec.TLS.CaBundleSecretName != "" {
		hash, ctrlResult, err := tls.ValidateCACertSecret(
			ctx,
			helper.GetClient(),
			k8s_types.NamespacedName{
				Name:      instance.Spec.TLS.CaBundleSecretName,
				Namespace: instance.Namespace,
			},
		)
		if err != nil {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.TLSInputReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.TLSInputErrorMessage,
				err.Error()))
			return ctrl.Result{}, "", err
		} else if (ctrlResult != ctrl.Result{}) {
			return ctrlResult, "", nil
		}

		if hash != "" {
			configMapVars[tls.CABundleKey] = env.SetValue(hash)
		}
	}

	// Validate API service certs secrets
	certsHash, ctrlResult, err := instance.Spec.TLS.API.ValidateCertSecrets(ctx, helper, instance.Namespace)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.TLSInputReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.TLSInputErrorMessage,
			err.Error()))
		return ctrl.Result{}, "", err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, "", nil
	}
	configMapVars[tls.TLSHashName] = env.SetValue(certsHash)

	// all cert input checks out so report InputReady
	instance.Status.Conditions.MarkTrue(condition.TLSInputReadyCondition, condition.InputReadyMessage)

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
		&configMapVars,
		db)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ServiceConfigReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ServiceConfigReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, "", err
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
		return ctrl.Result{}, "", err
	} else if hashChanged {
		// Hash changed and instance status should be updated (which will be done by main defer func),
		// so we need to return and reconcile again
		return ctrl.Result{RequeueAfter: time.Second * 5}, "", nil
	}

	instance.Status.Conditions.MarkTrue(
		condition.ServiceConfigReadyCondition,
		condition.ServiceConfigReadyMessage)

	// Create ConfigMaps and Secrets - end

	return ctrl.Result{}, inputHash, nil
}

func (r *IronicInspectorReconciler) reconcileStatefulSet(
	ctx context.Context,
	instance *ironicv1.IronicInspector,
	helper *helper.Helper,
	inputHash string,
	serviceLabels map[string]string,
	serviceAnnotations map[string]string,
) (ctrl.Result, error) {
	ingressDomain, err := ironic.GetIngressDomain(ctx, helper)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Define a new StatefulSet object
	ssDef, err := ironicinspector.StatefulSet(instance, inputHash, serviceLabels, ingressDomain, serviceAnnotations)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DeploymentReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DeploymentReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	ss := statefulset.NewStatefulSet(
		ssDef,
		time.Duration(5)*time.Second,
	)

	ctrlResult, err := ss.CreateOrPatch(ctx, helper)
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
		instance.Status.Conditions.MarkTrue(
			condition.DeploymentReadyCondition,
			condition.DeploymentReadyMessage)
	} else {
		return ctrl.Result{RequeueAfter: time.Second * 10}, nil
	}
	// create Statefulset - end

	return ctrl.Result{}, nil
}

func (r *IronicInspectorReconciler) reconcileNormal(
	ctx context.Context,
	instance *ironicv1.IronicInspector,
	helper *helper.Helper,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)

	Log.Info("Reconciling Ironic Inspector")

	serviceLabels := map[string]string{
		common.AppSelector:       ironic.ServiceName,
		common.ComponentSelector: ironic.InspectorComponent,
	}

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

	ctrlResult, err := r.reconcileTransportURL(ctx, instance, helper)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	db, ctrlResult, err := r.reconcileServiceDBinstance(ctx, instance, helper)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	ctrlResult, inputHash, err := r.reconcileConfigMapsAndSecrets(ctx, instance, helper, db)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	//
	// TODO check when/if Init, Update, or Upgrade should/could be skipped
	//

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
	ctrlResult, err = r.reconcileInit(ctx, instance, helper, serviceLabels)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// Handle service update
	ctrlResult, err = r.reconcileUpdate(ctx)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// Handle service upgrade
	ctrlResult, err = r.reconcileUpgrade(ctx)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	//
	// normal reconcile tasks
	//
	ctrlResult, err = r.reconcileStatefulSet(ctx, instance, helper, inputHash, serviceLabels, serviceAnnotations)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// Handle service init
	ctrlResult, err = r.reconcileServices(ctx, instance, helper, serviceLabels)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	err = mariadbv1.DeleteUnusedMariaDBAccountFinalizers(ctx, helper, ironicinspector.DatabaseCRName, instance.Spec.DatabaseAccount, instance.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	// We reached the end of the Reconcile, update the Ready condition based on
	// the sub conditions
	if instance.Status.Conditions.AllSubConditionIsTrue() {
		instance.Status.Conditions.MarkTrue(
			condition.ReadyCondition, condition.ReadyMessage)
	}
	Log.Info("Reconciled Ironic Inspector successfully")
	return ctrl.Result{}, nil
}

func (r *IronicInspectorReconciler) reconcileDeleteKeystoneServices(
	ctx context.Context,
	instance *ironicv1.IronicInspector,
	helper *helper.Helper,
) (ctrl.Result, error) {
	for _, ksSvc := range inspectorKeystoneServices {
		// Remove the finalizer from our KeystoneEndpoint CR
		keystoneEndpoint, err := keystonev1.GetKeystoneEndpointWithName(ctx, helper, ksSvc["name"], instance.Namespace)
		if err != nil && !k8s_errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}

		if err == nil {
			if controllerutil.RemoveFinalizer(keystoneEndpoint, helper.GetFinalizer()) {
				err = r.Update(ctx, keystoneEndpoint)
				if err != nil && !k8s_errors.IsNotFound(err) {
					return ctrl.Result{}, err
				}
				util.LogForObject(helper, "Removed finalizer from our KeystoneEndpoint", instance)
			}
		}

		// Remove the finalizer from our KeystoneService CR
		keystoneService, err := keystonev1.GetKeystoneServiceWithName(ctx, helper, ksSvc["name"], instance.Namespace)
		if err != nil && !k8s_errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}

		if err == nil {
			if controllerutil.RemoveFinalizer(keystoneService, helper.GetFinalizer()) {
				err = r.Update(ctx, keystoneService)
				if err != nil && !k8s_errors.IsNotFound(err) {
					return ctrl.Result{}, err
				}
				util.LogForObject(helper, "Removed finalizer from our KeystoneService", instance)
			}
		}
	}
	return ctrl.Result{}, nil
}

func (r *IronicInspectorReconciler) reconcileDeleteDatabase(
	ctx context.Context,
	instance *ironicv1.IronicInspector,
	helper *helper.Helper,
) (ctrl.Result, error) {
	// remove db finalizer first
	db, err := mariadbv1.GetDatabaseByNameAndAccount(ctx, helper, ironicinspector.DatabaseCRName, instance.Spec.DatabaseAccount, instance.Namespace)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}
	if !k8s_errors.IsNotFound(err) {
		if err := db.DeleteFinalizer(ctx, helper); err != nil {
			return ctrl.Result{}, err
		}
		util.LogForObject(helper, "Removed finalizer from our Database", instance)
	}
	return ctrl.Result{}, nil
}

func (r *IronicInspectorReconciler) reconcileDelete(
	ctx context.Context,
	instance *ironicv1.IronicInspector,
	helper *helper.Helper,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)

	Log.Info("Reconciling Ironic Inspector delete")

	ctrlResult, err := r.reconcileDeleteKeystoneServices(ctx, instance, helper)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	ctrlResult, err = r.reconcileDeleteDatabase(ctx, instance, helper)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// Service is deleted so remove the finalizer.
	controllerutil.RemoveFinalizer(instance, helper.GetFinalizer())
	Log.Info("Reconciled Ironic Inspector delete successfully")

	return ctrl.Result{}, nil
}

func (r *IronicInspectorReconciler) reconcileServiceDBinstance(
	ctx context.Context,
	instance *ironicv1.IronicInspector,
	helper *helper.Helper,
) (*mariadbv1.Database, ctrl.Result, error) {

	// ensure MariaDBAccount exists.  This account record may be created by
	// openstack-operator or the cloud operator up front without a specific
	// MariaDBDatabase configured yet.   Otherwise, a MariaDBAccount CR is
	// created here with a generated username as well as a secret with
	// generated password.   The MariaDBAccount is created without being
	// yet associated with any MariaDBDatabase.
	_, _, err := mariadbv1.EnsureMariaDBAccount(
		ctx, helper, instance.Spec.DatabaseAccount,
		instance.Namespace, false, "ironic_inspector",
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

	db := mariadbv1.NewDatabaseForAccount(
		instance.Spec.DatabaseInstance, // mariadb/galera service to target
		ironicinspector.DatabaseName,   // name used in CREATE DATABASE in mariadb
		ironicinspector.DatabaseCRName, // CR name for MariaDBDatabase
		instance.Spec.DatabaseAccount,  // CR name for MariaDBAccount
		instance.Namespace,             // namespace
	)

	// create or patch the DB
	ctrlResult, err := db.CreateOrPatchAll(ctx, helper)

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
	ctrlResult, err = db.WaitForDBCreatedWithTimeout(ctx, helper, time.Second*10)
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

	// update Status.DatabaseName and Status.DatabaseHostname, used to
	// bootstrap/config the service
	instance.Status.DatabaseHostname = db.GetDatabaseHostname()
	instance.Status.Conditions.MarkTrue(
		condition.DBReadyCondition,
		condition.DBReadyMessage)

	return db, ctrl.Result{}, nil
}

func (r *IronicInspectorReconciler) reconcileServiceDBsync(
	ctx context.Context,
	instance *ironicv1.IronicInspector,
	helper *helper.Helper,
	serviceLabels map[string]string,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	dbSyncHash := instance.Status.Hash[ironicv1.DbSyncHash]
	jobDef := ironicinspector.DbSyncJob(instance, serviceLabels)
	dbSyncjob := job.NewJob(
		jobDef,
		ironicv1.DbSyncHash,
		instance.Spec.PreserveJobs,
		time.Second*2,
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
		Log.Info(fmt.Sprintf(
			"Job %s hash added - %s",
			jobDef.Name,
			instance.Status.Hash[ironicv1.DbSyncHash]))
	}
	instance.Status.Conditions.MarkTrue(
		condition.DBSyncReadyCondition,
		condition.DBSyncReadyMessage)

	return ctrl.Result{}, nil
}

func (r *IronicInspectorReconciler) reconcileExposeService(
	ctx context.Context,
	instance *ironicv1.IronicInspector,
	helper *helper.Helper,
	serviceLabels map[string]string,
) (ctrl.Result, error) {
	//
	// expose the service (create service and return the created endpoint URLs)
	//

	// V1
	data := map[service.Endpoint]endpoint.Data{
		service.EndpointPublic: {
			Port: ironicinspector.IronicInspectorPublicPort,
		},
		service.EndpointInternal: {
			Port: ironicinspector.IronicInspectorInternalPort,
		},
	}

	apiEndpoints := make(map[string]string)
	inspectorServiceName := ironic.ServiceName + "-" + ironic.InspectorComponent

	for endpointType, data := range data {
		endpointTypeStr := string(endpointType)
		endpointName := inspectorServiceName + "-" + endpointTypeStr

		svcOverride := instance.Spec.Override.Service[endpointType]
		if svcOverride.EmbeddedLabelsAnnotations == nil {
			svcOverride.EmbeddedLabelsAnnotations = &service.EmbeddedLabelsAnnotations{}
		}

		exportLabels := util.MergeStringMaps(
			serviceLabels,
			map[string]string{
				service.AnnotationEndpointKey: endpointTypeStr,
			},
		)

		// Create the service
		svc, err := service.NewService(
			service.GenericService(&service.GenericServiceDetails{
				Name:      endpointName,
				Namespace: instance.Namespace,
				Labels:    exportLabels,
				Selector:  serviceLabels,
				Port: service.GenericServicePort{
					Name:     endpointName,
					Port:     data.Port,
					Protocol: corev1.ProtocolTCP,
				},
			}),
			5,
			&svcOverride.OverrideSpec,
		)
		if err != nil {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.ExposeServiceReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.ExposeServiceReadyErrorMessage,
				err.Error()))

			return ctrl.Result{}, err
		}

		svc.AddAnnotation(map[string]string{
			service.AnnotationEndpointKey: endpointTypeStr,
		})

		// add Annotation to whether creating an ingress is required or not
		if endpointType == service.EndpointPublic && svc.GetServiceType() == corev1.ServiceTypeClusterIP {
			svc.AddAnnotation(map[string]string{
				service.AnnotationIngressCreateKey: "true",
			})
		} else {
			svc.AddAnnotation(map[string]string{
				service.AnnotationIngressCreateKey: "false",
			})
			if svc.GetServiceType() == corev1.ServiceTypeLoadBalancer {
				svc.AddAnnotation(map[string]string{
					service.AnnotationHostnameKey: svc.GetServiceHostname(), // add annotation to register service name in dnsmasq
				})
			}
		}

		ctrlResult, err := svc.CreateOrPatch(ctx, helper)
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
		// create service - end

		// if TLS is enabled
		if instance.Spec.TLS.API.Enabled(endpointType) {
			// set endpoint protocol to https
			data.Protocol = ptr.To(service.ProtocolHTTPS)
		}

		apiEndpoints[string(endpointType)], err = svc.GetAPIEndpoint(
			svcOverride.EndpointURL, data.Protocol, data.Path)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	//
	// Update instance status with service endpoint url information for v2
	//
	instance.Status.APIEndpoints[inspectorServiceName] = apiEndpoints
	// V1 - end

	instance.Status.Conditions.MarkTrue(condition.ExposeServiceReadyCondition, condition.ExposeServiceReadyMessage)

	return ctrl.Result{}, nil
}

func (r *IronicInspectorReconciler) reconcileUsersAndEndpoints(
	ctx context.Context,
	instance *ironicv1.IronicInspector,
	helper *helper.Helper,
	serviceLabels map[string]string,
) (ctrl.Result, error) {
	//
	// create users and endpoints
	// TODO: rework this
	//
	if !instance.Spec.Standalone {
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
			ctrlResult, err := ksSvcObj.CreateOrPatch(ctx, helper)
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

	return ctrl.Result{}, nil
}

func (r *IronicInspectorReconciler) reconcileInit(
	ctx context.Context,
	instance *ironicv1.IronicInspector,
	helper *helper.Helper,
	serviceLabels map[string]string,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)

	Log.Info("Reconciling Ironic Inspector init")

	ctrlResult, err := r.reconcileServiceDBsync(ctx, instance, helper, serviceLabels)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	ctrlResult, err = r.reconcileExposeService(ctx, instance, helper, serviceLabels)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	ctrlResult, err = r.reconcileUsersAndEndpoints(ctx, instance, helper, serviceLabels)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	Log.Info("Reconciled Ironic Inspector init successfully")
	return ctrl.Result{}, nil
}

func (r *IronicInspectorReconciler) reconcileUpdate(
	ctx context.Context,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)

	Log.Info("Reconciling Ironic Inspector Service update")

	// TODO: should have minor update tasks if required
	// - delete dbsync hash from status to rerun it?

	return ctrl.Result{}, nil
}

func (r *IronicInspectorReconciler) reconcileUpgrade(
	ctx context.Context,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info("Reconciling Ironic Inspector Service upgrade")

	// TODO: should have major version upgrade tasks
	// -delete dbsync hash from status to rerun it?

	return ctrl.Result{}, nil
}

// generateServiceConfigMaps - create create configmaps which hold scripts and service configuration
// TODO add DefaultConfigOverwrite
func (r *IronicInspectorReconciler) generateServiceConfigMaps(
	ctx context.Context,
	instance *ironicv1.IronicInspector,
	h *helper.Helper,
	envVars *map[string]env.Setter,
	db *mariadbv1.Database,
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
	Log := r.GetLogger(ctx)
	var tlsCfg *tls.Service
	if instance.Spec.TLS.Ca.CaBundleSecretName != "" {
		tlsCfg = &tls.Service{}
	}
	// customData hold any customization for the service.
	// custom.conf is going to /etc/ironic-inspector/inspector.conf.d
	// all other files get placed into /etc/ironic-inspector to allow
	// overwrite of e.g. policy.json.
	// TODO: make sure custom.conf can not be overwritten
	customData := map[string]string{
		common.CustomServiceConfigFileName: instance.Spec.CustomServiceConfig,
		"my.cnf":                           db.GetDatabaseClientConfig(tlsCfg), //(mschuppert) for now just get the default my.cnf
	}
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
		keystoneInternalURL, err := keystoneAPI.GetEndpoint(endpoint.EndpointInternal)
		if err != nil {
			return err
		}
		keystonePublicURL, err := keystoneAPI.GetEndpoint(endpoint.EndpointPublic)
		if err != nil {
			return err
		}

		templateParameters["ServiceUser"] = instance.Spec.ServiceUser
		templateParameters["KeystoneInternalURL"] = keystoneInternalURL
		templateParameters["KeystonePublicURL"] = keystonePublicURL
	} else {
		ironicAPI, err := ironicv1.GetIronicAPI(
			ctx, h, instance.Namespace, map[string]string{})
		if err != nil {
			return err
		}
		ironicInternalURL, err := ironicAPI.GetEndpoint(endpoint.EndpointInternal)
		if err != nil {
			return err
		}
		templateParameters["IronicInternalURL"] = ironicInternalURL
	}
	dhcpRanges, err := ironic.PrefixOrNetmaskFromCIDR(instance.Spec.DHCPRanges)
	if err != nil {
		Log.Error(err, "unable to get Prefix or Netmask from IP network Prefix (CIDR)")
	}
	templateParameters["DHCPRanges"] = dhcpRanges
	templateParameters["Standalone"] = instance.Spec.Standalone

	databaseAccount := db.GetAccount()
	dbSecret := db.GetSecret()

	templateParameters["DatabaseConnection"] = fmt.Sprintf("mysql+pymysql://%s:%s@%s/%s?read_default_file=/etc/my.cnf",
		databaseAccount.Spec.UserName,
		string(dbSecret.Data[mariadbv1.DatabasePasswordSelector]),
		instance.Status.DatabaseHostname,
		ironicinspector.DatabaseName,
	)

	// create httpd  vhost template parameters
	httpdVhostConfig := map[string]interface{}{}
	for _, endpt := range []service.Endpoint{service.EndpointInternal, service.EndpointPublic} {
		endptConfig := map[string]interface{}{}
		endptConfig["ServerName"] = fmt.Sprintf("%s-%s-%s.%s.svc", ironic.ServiceName, ironic.InspectorComponent, endpt.String(), instance.Namespace)
		endptConfig["TLS"] = false // default TLS to false, and set it bellow to true if enabled
		if instance.Spec.TLS.API.Enabled(endpt) {
			endptConfig["TLS"] = true
			endptConfig["SSLCertificateFile"] = fmt.Sprintf("/etc/pki/tls/certs/%s.crt", endpt.String())
			endptConfig["SSLCertificateKeyFile"] = fmt.Sprintf("/etc/pki/tls/private/%s.key", endpt.String())
		}
		httpdVhostConfig[endpt.String()] = endptConfig
	}
	templateParameters["VHosts"] = httpdVhostConfig

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
	return oko_secret.EnsureSecrets(ctx, h, instance, cms, envVars)
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

func (r *IronicInspectorReconciler) reconcileServices(
	ctx context.Context,
	instance *ironicv1.IronicInspector,
	helper *helper.Helper,
	serviceLabels map[string]string,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)

	Log.Info("Reconciling Inspector Services")

	podList, err := ironicinspector.InspectorPods(
		ctx, instance, helper, serviceLabels)
	if err != nil {
		return ctrl.Result{}, err
	}

	for _, inspectorPod := range podList.Items {
		// Create the inspector pod service if none exists
		inspectorServiceLabels := map[string]string{
			common.AppSelector:       ironic.ServiceName,
			common.ComponentSelector: ironic.InspectorComponent,
		}
		inspectorService := ironicinspector.Service(
			inspectorPod.Name,
			instance,
			inspectorServiceLabels)
		if inspectorService != nil {
			err = controllerutil.SetOwnerReference(&inspectorPod, inspectorService, helper.GetScheme())
			if err != nil {
				return ctrl.Result{}, err
			}
			err = r.Get(
				ctx,
				k8s_types.NamespacedName{
					Name:      inspectorService.Name,
					Namespace: inspectorService.Namespace,
				},
				inspectorService,
			)
			if err != nil && k8s_errors.IsNotFound(err) {
				Log.Info(fmt.Sprintf("Service port %s does not exist, creating it", inspectorService.Name))
				err = r.Create(ctx, inspectorService)
				if err != nil {
					return ctrl.Result{}, err
				}
			} else {
				Log.Info(fmt.Sprintf("Service port %s exists, updating it", inspectorService.Name))
				err = r.Update(ctx, inspectorService)
				if err != nil {
					return ctrl.Result{}, err
				}
			}
		}
		// create service - end

		if instance.Spec.InspectionNetwork == "" {
			// Create the inspector pod route to enable traffic to the
			// httpboot service, only when there is no inspection network
			inspectorRouteLabels := map[string]string{
				common.AppSelector:       ironic.ServiceName,
				common.ComponentSelector: ironic.InspectorComponent + "-" + ironic.HttpbootComponent,
			}
			inspectorRoute := ironicinspector.Route(inspectorPod.Name, instance, inspectorRouteLabels)
			err = controllerutil.SetOwnerReference(&inspectorPod, inspectorRoute, helper.GetScheme())
			if err != nil {
				return ctrl.Result{}, err
			}
			err = r.Get(
				ctx,
				k8s_types.NamespacedName{
					Name:      inspectorRoute.Name,
					Namespace: inspectorRoute.Namespace,
				},
				inspectorRoute,
			)
			if err != nil && k8s_errors.IsNotFound(err) {
				Log.Info(fmt.Sprintf("Route %s does not exist, creating it", inspectorRoute.Name))
				err = r.Create(ctx, inspectorRoute)
				if err != nil {
					return ctrl.Result{}, err
				}
			} else {
				Log.Info(fmt.Sprintf("Route %s exists, updating it", inspectorRoute.Name))
				err = r.Update(ctx, inspectorRoute)
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

	Log.Info("Reconciled Inspector Services successfully")
	return ctrl.Result{}, nil
}
