/*
Copyright 2022.

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

	common "github.com/openstack-k8s-operators/lib-common/modules/common"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	configmap "github.com/openstack-k8s-operators/lib-common/modules/common/configmap"
	env "github.com/openstack-k8s-operators/lib-common/modules/common/env"
	helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	job "github.com/openstack-k8s-operators/lib-common/modules/common/job"
	labels "github.com/openstack-k8s-operators/lib-common/modules/common/labels"
	oko_secret "github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	util "github.com/openstack-k8s-operators/lib-common/modules/common/util"
	database "github.com/openstack-k8s-operators/lib-common/modules/database"

	ironicv1 "github.com/openstack-k8s-operators/ironic-operator/api/v1beta1"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"

	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// IronicReconciler reconciles a Ironic object
type IronicReconciler struct {
	client.Client
	Kclient kubernetes.Interface
	Log     logr.Logger
	Scheme  *runtime.Scheme
}

// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironics,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironics/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironics/finalizers,verbs=update
// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironicapis,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironicapis/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironicapis/finalizers,verbs=update
// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironicconductors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironicconductors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ironic.openstack.org,resources=ironicconductors/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;create;update;patch;delete;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;create;update;patch;delete;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;create;update;patch;delete;watch
// +kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbdatabases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneapis,verbs=get;list;watch
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneservices,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Ironic object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.1/pkg/reconcile
func (r *IronicReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = r.Log.WithValues("ironic", req.NamespacedName)

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
	//
	// initialize status
	//
	if instance.Status.Conditions == nil {
		instance.Status.Conditions = condition.Conditions{}

		cl := condition.CreateList(
			condition.UnknownCondition(condition.DBReadyCondition, condition.InitReason, condition.DBReadyInitMessage),
			condition.UnknownCondition(condition.DBSyncReadyCondition, condition.InitReason, condition.DBSyncReadyInitMessage),
			condition.UnknownCondition(condition.ExposeServiceReadyCondition, condition.InitReason, condition.ExposeServiceReadyInitMessage),
			condition.UnknownCondition(condition.BootstrapReadyCondition, condition.InitReason, condition.BootstrapReadyInitMessage),
			condition.UnknownCondition(condition.InputReadyCondition, condition.InitReason, condition.InputReadyInitMessage),
			condition.UnknownCondition(condition.ServiceConfigReadyCondition, condition.InitReason, condition.ServiceConfigReadyInitMessage),
			condition.UnknownCondition(condition.DeploymentReadyCondition, condition.InitReason, condition.DeploymentReadyInitMessage))

		instance.Status.Conditions.Init(&cl)

		// Register overall status immediately to have an early feedback e.g. in the cli
		if err := r.Status().Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}
	}
	if instance.Status.Hash == nil {
		instance.Status.Hash = map[string]string{}
	}
	if instance.Status.APIEndpoints == nil {
		instance.Status.APIEndpoints = map[string]map[string]string{}
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

		if err := helper.SetAfter(instance); err != nil {
			util.LogErrorForObject(helper, err, "Set after and calc patch/diff", instance)
		}

		if changed := helper.GetChanges()["status"]; changed {
			patch := client.MergeFrom(helper.GetBeforeObject())

			if err := r.Status().Patch(ctx, instance, patch); err != nil && !k8s_errors.IsNotFound(err) {
				util.LogErrorForObject(helper, err, "Update status", instance)
			}
		}
	}()

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
		Owns(&mariadbv1.MariaDBDatabase{}).
		Owns(&batchv1.Job{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}

func (r *IronicReconciler) reconcileDelete(ctx context.Context, instance *ironicv1.Ironic, helper *helper.Helper) (ctrl.Result, error) {
	r.Log.Info("Reconciling Service delete")

	// Service is deleted so remove the finalizer.
	controllerutil.RemoveFinalizer(instance, helper.GetFinalizer())
	r.Log.Info("Reconciled Service delete successfully")
	if err := r.Update(ctx, instance); err != nil && !k8s_errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *IronicReconciler) reconcileNormal(ctx context.Context, instance *ironicv1.Ironic, helper *helper.Helper) (ctrl.Result, error) {
	r.Log.Info("Reconciling Service")

	// If the service object doesn't have our finalizer, add it.
	controllerutil.AddFinalizer(instance, helper.GetFinalizer())
	// Register the finalizer immediately to avoid orphaning resources on delete
	if err := r.Update(ctx, instance); err != nil {
		return ctrl.Result{}, err
	}

	// ConfigMap
	configMapVars := make(map[string]env.Setter)

	//
	// check for required OpenStack secret holding passwords for service/admin user and add hash to the vars map
	//
	ospSecret, hash, err := oko_secret.GetSecret(ctx, helper, instance.Spec.Secret, instance.Namespace)
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

	instance.Status.Conditions.MarkTrue(condition.InputReadyCondition, condition.InputReadyMessage)
	// run check OpenStack secret - end

	//
	// Create ConfigMaps and Secrets required as input for the Service and calculate an overall hash of hashes
	//

	//
	// create Configmap required for ironic input
	// - %-scripts configmap holding scripts to e.g. bootstrap the service
	// - %-config configmap holding minimal ironic config required to get the service up, user can add additional files to be added to the service
	// - parameters which has passwords gets added from the OpenStack secret via the init container
	//
	err = r.generateServiceConfigMaps(ctx, instance, helper, &configMapVars)
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
	_, err = r.createHashOfInputHashes(ctx, instance, configMapVars)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ServiceConfigReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ServiceConfigReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	// Create ConfigMaps and Secrets - end

	instance.Status.Conditions.MarkTrue(condition.ServiceConfigReadyCondition, condition.ServiceConfigReadyMessage)

	//
	// TODO check when/if Init, Update, or Upgrade should/could be skipped
	//

	serviceLabels := map[string]string{
		common.AppSelector: ironic.ServiceName,
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

	// deploy ironic-api
	ironicAPI, op, err := r.apiDeploymentCreateOrUpdate(instance)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			ironicv1.IronicAPIReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			ironicv1.IronicAPIReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	if op != controllerutil.OperationResultNone {
		r.Log.Info(fmt.Sprintf("Deployment %s successfully reconciled - operation: %s", instance.Name, string(op)))
	}

	// Mirror IronicAPI status' APIEndpoints and ReadyCount to this parent CR
	instance.Status.APIEndpoints = ironicAPI.Status.APIEndpoints
	instance.Status.ServiceIDs = ironicAPI.Status.ServiceIDs
	instance.Status.IronicAPIReadyCount = ironicAPI.Status.ReadyCount

	// Mirror IronicAPI's condition status
	c := ironicAPI.Status.Conditions.Mirror(ironicv1.IronicAPIReadyCondition)
	if c != nil {
		instance.Status.Conditions.Set(c)
	}

	// deploy ironic-conductor
	ironicConductor, op, err := r.conductorDeploymentCreateOrUpdate(instance)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			ironicv1.IronicConductorReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			ironicv1.IronicConductorReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	if op != controllerutil.OperationResultNone {
		r.Log.Info(fmt.Sprintf("Deployment %s successfully reconciled - operation: %s", instance.Name, string(op)))
	}
	// Mirror IronicConductor status' ReadyCount to this parent CR
	// instance.Status.ServiceIDs = ironicConductor.Status.ServiceIDs
	instance.Status.IronicConductorReadyCount = ironicConductor.Status.ReadyCount

	// Mirror IronicConductor's condition status
	c = ironicConductor.Status.Conditions.Mirror(ironicv1.IronicConductorReadyCondition)
	if c != nil {
		instance.Status.Conditions.Set(c)
	}

	r.Log.Info("Reconciled Service successfully")
	return ctrl.Result{}, nil
}

func (r *IronicReconciler) reconcileInit(
	ctx context.Context,
	instance *ironicv1.Ironic,
	helper *helper.Helper,
	serviceLabels map[string]string,
) (ctrl.Result, error) {
	r.Log.Info("Reconciling Service init")

	//
	// create service DB instance
	//
	db := database.NewDatabase(
		instance.Name,
		instance.Spec.DatabaseUser,
		instance.Spec.Secret,
		map[string]string{
			"dbName": instance.Spec.DatabaseInstance,
		},
	)
	// create or patch the DB
	ctrlResult, err := db.CreateOrPatchDB(
		ctx,
		helper,
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
	ctrlResult, err = db.WaitForDBCreated(ctx, helper)
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
	instance.Status.Conditions.MarkTrue(condition.DBReadyCondition, condition.DBReadyMessage)

	// create service DB - end

	//
	// run ironic db sync
	//
	dbSyncHash := instance.Status.Hash[ironicv1.DbSyncHash]
	jobDef := ironic.DbSyncJob(instance, serviceLabels)
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
		if err := r.Client.Status().Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}
		r.Log.Info(fmt.Sprintf("Job %s hash added - %s", jobDef.Name, instance.Status.Hash[ironicv1.DbSyncHash]))
	}
	instance.Status.Conditions.MarkTrue(condition.DBSyncReadyCondition, condition.DBSyncReadyMessage)

	// run ironic db sync - end

	r.Log.Info("Reconciled Service init successfully")
	return ctrl.Result{}, nil
}

func (r *IronicReconciler) reconcileUpdate(ctx context.Context, instance *ironicv1.Ironic, helper *helper.Helper) (ctrl.Result, error) {
	r.Log.Info("Reconciling Service update")

	// TODO: should have minor update tasks if required
	// - delete dbsync hash from status to rerun it?

	r.Log.Info("Reconciled Service update successfully")
	return ctrl.Result{}, nil
}

func (r *IronicReconciler) reconcileUpgrade(ctx context.Context, instance *ironicv1.Ironic, helper *helper.Helper) (ctrl.Result, error) {
	r.Log.Info("Reconciling Service upgrade")

	// TODO: should have major version upgrade tasks
	// -delete dbsync hash from status to rerun it?

	r.Log.Info("Reconciled Service upgrade successfully")
	return ctrl.Result{}, nil
}

func (r *IronicReconciler) conductorDeploymentCreateOrUpdate(instance *ironicv1.Ironic) (*ironicv1.IronicConductor, controllerutil.OperationResult, error) {
	deployment := &ironicv1.IronicConductor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-conductor", instance.Name),
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(context.TODO(), r.Client, deployment, func() error {
		deployment.Spec = instance.Spec.IronicConductor
		// Add in transfers from umbrella Ironic (this instance) spec
		// TODO: Add logic to determine when to set/overwrite, etc
		deployment.Spec.Standalone = instance.Spec.Standalone
		deployment.Spec.ServiceUser = instance.Spec.ServiceUser
		deployment.Spec.DatabaseHostname = instance.Status.DatabaseHostname
		deployment.Spec.DatabaseUser = instance.Spec.DatabaseUser
		deployment.Spec.Secret = instance.Spec.Secret

		err := controllerutil.SetControllerReference(instance, deployment, r.Scheme)
		if err != nil {
			return err
		}

		return nil
	})

	return deployment, op, err
}

func (r *IronicReconciler) apiDeploymentCreateOrUpdate(instance *ironicv1.Ironic) (*ironicv1.IronicAPI, controllerutil.OperationResult, error) {
	deployment := &ironicv1.IronicAPI{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-api", instance.Name),
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(context.TODO(), r.Client, deployment, func() error {
		deployment.Spec = instance.Spec.IronicAPI
		// Add in transfers from umbrella Ironic (this instance) spec
		// TODO: Add logic to determine when to set/overwrite, etc
		deployment.Spec.Standalone = instance.Spec.Standalone
		deployment.Spec.ServiceUser = instance.Spec.ServiceUser
		deployment.Spec.DatabaseHostname = instance.Status.DatabaseHostname
		deployment.Spec.DatabaseUser = instance.Spec.DatabaseUser
		deployment.Spec.Secret = instance.Spec.Secret

		err := controllerutil.SetControllerReference(instance, deployment, r.Scheme)
		if err != nil {
			return err
		}

		return nil
	})

	return deployment, op, err
}

//
// generateServiceConfigMaps - create create configmaps which hold scripts and service configuration
// TODO add DefaultConfigOverwrite
//
func (r *IronicReconciler) generateServiceConfigMaps(
	ctx context.Context,
	instance *ironicv1.Ironic,
	h *helper.Helper,
	envVars *map[string]env.Setter,
) error {
	//
	// create Configmap/Secret required for ironic input
	// - %-scripts configmap holding scripts to e.g. bootstrap the service
	// - %-config configmap holding minimal ironic config required to get the service up, user can add additional files to be added to the service
	// - parameters which has passwords gets added from the ospSecret via the init container
	//

	cmLabels := labels.GetLabels(instance, labels.GetGroupLabel(ironic.ServiceName), map[string]string{})

	// customData hold any customization for the service.
	// custom.conf is going to /etc/<service>/<service>.conf.d
	// all other files get placed into /etc/<service> to allow overwrite of e.g. policy.json
	// TODO: make sure custom.conf can not be overwritten
	customData := map[string]string{common.CustomServiceConfigFileName: instance.Spec.CustomServiceConfig}
	for key, data := range instance.Spec.DefaultConfigOverwrite {
		customData[key] = data
	}

	templateParameters := make(map[string]interface{})

	cms := []util.Template{
		// ScriptsConfigMap
		{
			Name:               fmt.Sprintf("%s-scripts", instance.Name),
			Namespace:          instance.Namespace,
			Type:               util.TemplateTypeScripts,
			InstanceType:       instance.Kind,
			AdditionalTemplate: map[string]string{"common.sh": "/common/common.sh"},
			Labels:             cmLabels,
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
	err := configmap.EnsureConfigMaps(ctx, h, instance, cms, envVars)
	if err != nil {
		return nil
	}

	return nil
}

//
// createHashOfInputHashes - creates a hash of hashes which gets added to the resources which requires a restart
// if any of the input resources change, like configs, passwords, ...
//
func (r *IronicReconciler) createHashOfInputHashes(
	ctx context.Context,
	instance *ironicv1.Ironic,
	envVars map[string]env.Setter,
) (string, error) {
	mergedMapVars := env.MergeEnvs([]corev1.EnvVar{}, envVars)
	hash, err := util.ObjectHash(mergedMapVars)
	if err != nil {
		return hash, err
	}
	if hashMap, changed := util.SetHash(instance.Status.Hash, common.InputHashName, hash); changed {
		instance.Status.Hash = hashMap
		if err := r.Client.Status().Update(ctx, instance); err != nil {
			return hash, err
		}
		r.Log.Info(fmt.Sprintf("Input maps hash %s - %s", common.InputHashName, hash))
	}
	return hash, nil
}
