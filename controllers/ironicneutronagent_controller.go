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

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	rabbitmqv1 "github.com/openstack-k8s-operators/infra-operator/apis/rabbitmq/v1beta1"
	ironicv1 "github.com/openstack-k8s-operators/ironic-operator/api/v1beta1"
	ironic "github.com/openstack-k8s-operators/ironic-operator/pkg/ironic"
	"github.com/openstack-k8s-operators/ironic-operator/pkg/ironicneutronagent"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	endpoint "github.com/openstack-k8s-operators/lib-common/modules/common/endpoint"

	"github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/configmap"
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
	Log     logr.Logger
	Scheme  *runtime.Scheme
}

// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironicneutronagents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironicneutronagents/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironicneutronagents/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;create;update;patch;delete;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;create;update;patch;delete;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;create;update;patch;delete;watch
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneservices,verbs=get;list;watch
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneendpoints,verbs=get;list;watch

// service account, role, rolebinding
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=roles,verbs=get;list;watch;create;update
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=rolebindings,verbs=get;list;watch;create;update
// service account permissions that are needed to grant permission to the above
// +kubebuilder:rbac:groups="security.openshift.io",resourceNames=anyuid;privileged,resources=securitycontextconstraints,verbs=use
// +kubebuilder:rbac:groups="",resources=pods,verbs=create;delete;get;list;patch;update;watch

// Reconcile -
func (r *IronicNeutronAgentReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (result ctrl.Result, _err error) {
	_ = log.FromContext(ctx)

	// Fetch the IronicNeutronAgent instance
	instance := &ironicv1.IronicNeutronAgent{}
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
			condition.UnknownCondition(
				condition.InputReadyCondition,
				condition.InitReason,
				condition.InputReadyInitMessage),
			condition.UnknownCondition(
				ironicv1.IronicRabbitMqTransportURLReadyCondition,
				condition.InitReason,
				ironicv1.IronicRabbitMqTransportURLReadyInitMessage),
			condition.UnknownCondition(
				condition.ServiceConfigReadyCondition,
				condition.InitReason,
				condition.ServiceConfigReadyInitMessage),
			condition.UnknownCondition(
				condition.DeploymentReadyCondition,
				condition.InitReason,
				condition.DeploymentReadyInitMessage),
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

		// Register overall status immediately to have an early feedback e.g. in the cli
		return ctrl.Result{}, nil
	}
	if instance.Status.Hash == nil {
		instance.Status.Hash = map[string]string{}
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
	mgr ctrl.Manager,
) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ironicv1.IronicNeutronAgent{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&rabbitmqv1.TransportURL{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Complete(r)
}

func (r *IronicNeutronAgentReconciler) reconcileTransportURL(
	ctx context.Context,
	instance *ironicv1.IronicNeutronAgent,
	helper *helper.Helper,
) (ctrl.Result, error) {
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

	return ctrl.Result{}, nil
}

func (r *IronicNeutronAgentReconciler) reconcileConfigMapsAndSecrets(
	ctx context.Context,
	instance *ironicv1.IronicNeutronAgent,
	helper *helper.Helper,
) (ctrl.Result, string, error) {
	// ConfigMap
	configMapVars := make(map[string]env.Setter)

	// check for required OpenStack secret holding passwords for service/admin user and add hash to the vars map
	ospSecret, hash, err := secret.GetSecret(ctx, helper, instance.Spec.Secret, instance.Namespace)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.InputReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				condition.InputReadyWaitingMessage))
			return ctrl.Result{RequeueAfter: time.Second * 10}, "", fmt.Errorf("OpenStack secret %s not found", instance.Spec.Secret)
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
	instance.Status.Conditions.MarkTrue(
		condition.InputReadyCondition,
		condition.InputReadyMessage)
	// run check OpenStack secret - end

	//
	// Create ConfigMaps required as input for the Service and calculate an overall hash of hashes
	//

	// create custom Configmap for IronicNeutronAgent input
	// - %-scripts configmap holding scripts to e.g. bootstrap the service
	// - %-config configmap holding minimal neutron config required to get the
	//   service up, user can add additional files to be added to the service
	// - parameters which has passwords gets added from the OpenStack secret via the init container
	//
	err = r.generateServiceConfigMaps(ctx, helper, instance, &configMapVars)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ServiceConfigReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ServiceConfigReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, "", err
	}
	// Create ConfigMaps - end

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
		return ctrl.Result{}, "", nil
	}
	instance.Status.Conditions.MarkTrue(
		condition.ServiceConfigReadyCondition,
		condition.ServiceConfigReadyMessage)
	// Create ConfigMaps and Secrets - end

	return ctrl.Result{}, inputHash, nil
}

func (r *IronicNeutronAgentReconciler) reconcileDeployment(
	ctx context.Context,
	instance *ironicv1.IronicNeutronAgent,
	helper *helper.Helper,
	inputHash string,
	serviceLabels map[string]string,
) (ctrl.Result, error) {
	// Define a new Deployment object
	deplomentDef := ironicneutronagent.Deployment(
		instance,
		inputHash,
		serviceLabels,
	)
	deployment := deployment.NewDeployment(deplomentDef, 5)
	ctrlResult, err := deployment.CreateOrPatch(ctx, helper)
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
	instance.Status.ReadyCount = deployment.GetDeployment().Status.ReadyReplicas

	if instance.Status.ReadyCount > 0 {
		instance.Status.Conditions.MarkTrue(condition.DeploymentReadyCondition, condition.DeploymentReadyMessage)
	}

	return ctrl.Result{}, nil
}

func (r *IronicNeutronAgentReconciler) reconcileNormal(
	ctx context.Context,
	instance *ironicv1.IronicNeutronAgent,
	helper *helper.Helper,
) (ctrl.Result, error) {
	r.Log.Info("Reconciling IronicNeutronAgent")

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

	ctrlResult, inputHash, err := r.reconcileConfigMapsAndSecrets(ctx, instance, helper)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	//
	// TODO check when/if Init, Update, or Upgrade should/could be skipped
	//

	serviceLabels := map[string]string{
		common.AppSelector: ironicneutronagent.ServiceName,
	}

	// Handle service init
	ctrlResult, err = r.reconcileInit(ctx, instance, helper, serviceLabels)
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
	ctrlResult, err = r.reconcileDeployment(ctx, instance, helper, inputHash, serviceLabels)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	r.Log.Info("Reconciled IronicNeutronAgent Service successfully")
	return ctrl.Result{}, nil
}

func (r *IronicNeutronAgentReconciler) reconcileInit(
	ctx context.Context,
	instance *ironicv1.IronicNeutronAgent,
	helper *helper.Helper,
	serviceLabels map[string]string,
) (ctrl.Result, error) {
	r.Log.Info("Reconciling IronicNeutronAgent init")
	r.Log.Info("Reconciled IronicNeutronAgent init successfully")

	return ctrl.Result{}, nil
}

func (r *IronicNeutronAgentReconciler) reconcileDelete(
	ctx context.Context,
	instance *ironicv1.IronicNeutronAgent,
	helper *helper.Helper,
) (ctrl.Result, error) {
	r.Log.Info("Reconciling IronicNeutronAgent delete")
	// Service is deleted so remove the finalizer.
	controllerutil.RemoveFinalizer(instance, helper.GetFinalizer())
	r.Log.Info("Reconciled IronicNeutronAgent delete successfully")

	return ctrl.Result{}, nil
}

func (r *IronicNeutronAgentReconciler) reconcileUpdate(
	ctx context.Context,
	instance *ironicv1.IronicNeutronAgent,
	helper *helper.Helper,
) (ctrl.Result, error) {
	r.Log.Info("Reconciling IronicNeutronAgent update")
	r.Log.Info("Reconciled IronicNeutronAgent update successfully")

	return ctrl.Result{}, nil
}

func (r *IronicNeutronAgentReconciler) reconcileUpgrade(
	ctx context.Context,
	instance *ironicv1.IronicNeutronAgent,
	helper *helper.Helper,
) (ctrl.Result, error) {
	r.Log.Info("Reconciling IronicNeutronAgent upgrade")
	r.Log.Info("Reconciled IronicNeutronAgent upgrade successfully")

	return ctrl.Result{}, nil
}

// generateServiceConfigMaps - create custom configmap to hold service-specific config
func (r *IronicNeutronAgentReconciler) generateServiceConfigMaps(
	ctx context.Context,
	h *helper.Helper,
	instance *ironicv1.IronicNeutronAgent,
	envVars *map[string]env.Setter,
) error {
	//
	// create custom Configmap for ironic-neutron-agnet-specific config input
	// - %-config-data configmap holding custom config for the service config
	//

	cmLabels := labels.GetLabels(instance, labels.GetGroupLabel(ironic.ServiceName), map[string]string{})

	// customData hold any customization for the service.
	// custom.conf is going to be merged into /etc/ironic/ironic.conf
	// TODO: make sure custom.conf can not be overwritten
	customData := map[string]string{common.CustomServiceConfigFileName: instance.Spec.CustomServiceConfig}
	customData[common.CustomServiceConfigFileName] = instance.Spec.CustomServiceConfig

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

	templateParameters := make(map[string]interface{})
	templateParameters["ServiceUser"] = instance.Spec.ServiceUser
	templateParameters["KeystoneInternalURL"] = keystoneInternalURL
	templateParameters["KeystonePublicURL"] = keystonePublicURL

	cms := []util.Template{
		// ScriptsConfigMap
		{
			Name:         fmt.Sprintf("%s-scripts", instance.Name),
			Namespace:    instance.Namespace,
			Type:         util.TemplateTypeScripts,
			InstanceType: instance.Kind,
			AdditionalTemplate: map[string]string{
				"common.sh": "/common/bin/common.sh",
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
			Labels:        cmLabels,
		},
	}

	return configmap.EnsureConfigMaps(ctx, h, instance, cms, envVars)
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
