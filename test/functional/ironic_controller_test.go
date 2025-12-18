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

package functional_test

import (
	"fmt"
	"os"

	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	//revive:disable-next-line:dot-imports

	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"

	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	ironicv1 "github.com/openstack-k8s-operators/ironic-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	mariadb_test "github.com/openstack-k8s-operators/mariadb-operator/api/test/helpers"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Ironic controller", func() {
	When("Ironic is created with rpcTransport == oslo", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateIronicSecret(ironicNames.Namespace, SecretName),
			)
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateMessageBusSecret(ironicNames.Namespace, MessageBusSecretName),
			)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					ironicNames.Namespace,
					"openstack",
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			DeferCleanup(
				keystone.DeleteKeystoneAPI,
				keystone.CreateKeystoneAPI(ironicNames.Namespace))
			spec := GetDefaultIronicSpec()
			spec["rpcTransport"] = "oslo"
			spec["transportURLSecret"] = MessageBusSecretName
			DeferCleanup(
				th.DeleteInstance,
				CreateIronic(ironicNames.IronicName, spec),
			)
		})
		It("should have the Spec fields initialized", func() {
			instance := GetIronic(ironicNames.IronicName)
			Expect(instance.Spec.ServiceUser).Should(Equal("ironic"))
			Expect(instance.Spec.Standalone).Should(BeFalse())
			Expect(instance.Spec.PasswordSelectors).Should(Equal(
				ironicv1.PasswordSelector{
					Service: "IronicPassword",
				}))
			Expect(instance.Spec.CustomServiceConfig).Should(Equal("# add your customization here"))
			Expect(instance.Spec.StorageClass).Should(Equal(""))
			Expect(instance.Spec.PreserveJobs).Should(BeTrue())
			Expect(instance.Spec.RabbitMqClusterName).Should(Equal("rabbitmq"))
		})
		It("initializes Status fields", func() {
			instance := GetIronic(ironicNames.IronicName)
			Expect(instance.Status.Hash).To(BeEmpty())
			Expect(instance.Status.APIEndpoints).To(BeEmpty())
			Expect(instance.Status.IronicAPIReadyCount).To(Equal(int32(0)))
			Expect(instance.Status.InspectorReadyCount).To(Equal(int32(0)))
			Expect(instance.Status.IronicNeutronAgentReadyCount).To(Equal(int32(0)))
			Expect(instance.Status.IronicConductorReadyCount).To(BeEmpty())
		})
		It("should have a finalizer", func() {
			Eventually(func() []string {
				return GetIronic(ironicNames.IronicName).Finalizers
			}, timeout, interval).Should(ContainElement("openstack.org/ironic"))
		})
		It("creates service account, role and rolebindig", func() {
			th.ExpectCondition(
				ironicNames.IronicName,
				ConditionGetterFunc(IronicConditionGetter),
				condition.ServiceAccountReadyCondition,
				corev1.ConditionTrue,
			)
			sa := th.GetServiceAccount(ironicNames.IronicServiceAccount)
			th.ExpectCondition(
				ironicNames.IronicName,
				ConditionGetterFunc(IronicConditionGetter),
				condition.RoleReadyCondition,
				corev1.ConditionTrue,
			)
			role := th.GetRole(ironicNames.IronicRole)
			Expect(role.Rules).To(HaveLen(2))
			Expect(role.Rules[0].Resources).To(Equal([]string{"securitycontextconstraints"}))
			Expect(role.Rules[1].Resources).To(Equal([]string{"pods"}))
			th.ExpectCondition(
				ironicNames.IronicName,
				ConditionGetterFunc(IronicConditionGetter),
				condition.RoleBindingReadyCondition,
				corev1.ConditionTrue,
			)
			binding := th.GetRoleBinding(ironicNames.IronicRoleBinding)
			Expect(binding.RoleRef.Name).To(Equal(role.Name))
			Expect(binding.Subjects).To(HaveLen(1))
			Expect(binding.Subjects[0].Name).To(Equal(sa.Name))
		})
		It("creates Transport URL and sets TransportURLSecret status field", func() {
			infra.GetTransportURL(ironicNames.IronicTransportURLName)
			infra.SimulateTransportURLReady(ironicNames.IronicTransportURLName)
			th.ExpectCondition(
				ironicNames.IronicName,
				ConditionGetterFunc(IronicConditionGetter),
				condition.RabbitMqTransportURLReadyCondition,
				corev1.ConditionTrue,
			)
			instance := GetIronic(ironicNames.IronicName)
			Expect(instance.Status.TransportURLSecret).To(Equal("rabbitmq-secret"))
		})
		It("Creates ConfigMaps and gets Secrets (input) and set Hash of inputs", func() {
			infra.GetTransportURL(ironicNames.IronicTransportURLName)
			infra.SimulateTransportURLReady(ironicNames.IronicTransportURLName)
			mariadb.GetMariaDBDatabase(ironicNames.IronicDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(ironicNames.IronicDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(ironicNames.IronicDatabaseName)
			cm := th.GetSecret(ironicNames.IronicConfigSecretName)
			myCnf := cm.Data["my.cnf"]
			Expect(myCnf).To(
				ContainSubstring("[client]\nssl=0"))
			th.ExpectCondition(
				ironicNames.IronicName,
				ConditionGetterFunc(IronicConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
			Eventually(func(g Gomega) {
				instance := GetIronic(ironicNames.IronicName)
				g.Expect(instance.Status.Hash).Should(HaveKeyWithValue("input", Not(BeEmpty())))
			}, timeout, interval).Should(Succeed())
			th.ExpectCondition(
				ironicNames.IronicName,
				ConditionGetterFunc(IronicConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("Creates service database instance", func() {
			infra.GetTransportURL(ironicNames.IronicTransportURLName)
			infra.SimulateTransportURLReady(ironicNames.IronicTransportURLName)
			mariadb.GetMariaDBDatabase(ironicNames.IronicDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(ironicNames.IronicDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(ironicNames.IronicDatabaseName)
			th.ExpectCondition(
				ironicNames.IronicName,
				ConditionGetterFunc(IronicConditionGetter),
				condition.DBReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("Runs service database DBsync", func() {
			infra.GetTransportURL(ironicNames.IronicTransportURLName)
			infra.SimulateTransportURLReady(ironicNames.IronicTransportURLName)
			mariadb.GetMariaDBDatabase(ironicNames.IronicDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(ironicNames.IronicDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(ironicNames.IronicDatabaseName)
			th.SimulateJobSuccess(ironicNames.IronicDBSyncJobName)
			th.ExpectCondition(
				ironicNames.IronicName,
				ConditionGetterFunc(IronicConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("Creates deployment for API, Conductor, Inspector and INA", func() {
			infra.GetTransportURL(ironicNames.IronicTransportURLName)
			infra.SimulateTransportURLReady(ironicNames.IronicTransportURLName)
			mariadb.GetMariaDBDatabase(ironicNames.IronicDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(ironicNames.IronicDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(ironicNames.IronicDatabaseName)
			th.SimulateJobSuccess(ironicNames.IronicDBSyncJobName)
			Eventually(func(g Gomega) {
				g.Expect(th.K8sClient.Get(th.Ctx, types.NamespacedName{
					Namespace: ironicNames.Namespace,
					Name:      "ironic-api",
				}, &ironicv1.IronicAPI{})).Should(Succeed())
			}, th.Timeout, th.Interval).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(th.K8sClient.Get(th.Ctx, types.NamespacedName{
					Namespace: ironicNames.Namespace,
					Name:      "ironic-conductor",
				}, &ironicv1.IronicConductor{})).Should(Succeed())
			}, th.Timeout, th.Interval).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(th.K8sClient.Get(th.Ctx, types.NamespacedName{
					Namespace: ironicNames.Namespace,
					Name:      "ironic-inspector",
				}, &ironicv1.IronicInspector{})).Should(Succeed())
			}, th.Timeout, th.Interval).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(th.K8sClient.Get(th.Ctx, types.NamespacedName{
					Namespace: ironicNames.Namespace,
					Name:      fmt.Sprintf("%s-ironic-neutron-agent", ironicNames.IronicName.Name),
				}, &ironicv1.IronicNeutronAgent{})).Should(Succeed())
			}, th.Timeout, th.Interval).Should(Succeed())
		})
	})

	When("Deployment rollout is progressing", func() {
		var inaName types.NamespacedName
		BeforeEach(func() {
			inaName = types.NamespacedName{
				Namespace: ironicNames.Namespace,
				Name:      ironicNames.IronicName.Name + "-" + ironicNames.INAName.Name,
			}

			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateIronicSecret(ironicNames.Namespace, SecretName),
			)
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateMessageBusSecret(ironicNames.Namespace, MessageBusSecretName),
			)

			apiMariaDBAccount, apiMariaDBSecret := mariadb.CreateMariaDBAccountAndSecret(ironicNames.IronicDatabaseAccount, mariadbv1.MariaDBAccountSpec{})
			DeferCleanup(k8sClient.Delete, ctx, apiMariaDBAccount)
			DeferCleanup(k8sClient.Delete, ctx, apiMariaDBSecret)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					ironicNames.Namespace,
					"openstack",
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			DeferCleanup(
				keystone.DeleteKeystoneAPI,
				keystone.CreateKeystoneAPI(ironicNames.Namespace))
			DeferCleanup(
				th.DeleteInstance,
				CreateIronic(ironicNames.IronicName, GetDefaultIronicSpec()),
			)
			mariadb.GetMariaDBDatabase(ironicNames.IronicDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(ironicNames.IronicDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(ironicNames.IronicDatabaseName)
			th.SimulateJobSuccess(ironicNames.IronicDBSyncJobName)

			keystone.SimulateKeystoneServiceReady(ironicNames.IronicName)
			keystone.SimulateKeystoneEndpointReady(ironicNames.IronicName)

			mariadb.GetMariaDBDatabase(ironicNames.InspectorDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(ironicNames.InspectorDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(ironicNames.InspectorDatabaseName)
			th.SimulateJobSuccess(ironicNames.InspectorDBSyncJobName)

			nestedINATransportURLName := ironicNames.INATransportURLName
			nestedINATransportURLName.Name = ironicNames.IronicName.Name + "-" + nestedINATransportURLName.Name
			infra.GetTransportURL(nestedINATransportURLName)
			infra.SimulateTransportURLReady(nestedINATransportURLName)

			// API, Conductor, Inspector and Worker and NeutronAgent deployment in progress
			th.SimulateDeploymentProgressing(ironicNames.IronicName)
			th.SimulateStatefulSetProgressing(ironicNames.ConductorName)
			th.SimulateStatefulSetProgressing(ironicNames.InspectorName)
			th.SimulateDeploymentProgressing(ironicNames.INAName)
		})

		It("shows the IronicAPI deployment progressing in DeploymentReadyCondition", func() {
			// IronicAPI - deployment progressing
			th.ExpectConditionWithDetails(
				ironicNames.APIName,
				ConditionGetterFunc(IronicAPIConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				condition.DeploymentReadyRunningMessage,
			)
			th.ExpectCondition(
				ironicNames.APIName,
				ConditionGetterFunc(IronicAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)

			// overall Ironic condition false
			th.ExpectCondition(
				ironicNames.IronicName,
				ConditionGetterFunc(IronicConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("shows the IronicConductor deployment progressing in DeploymentReadyCondition", func() {
			// IronicConductor - deployment progressing
			th.ExpectConditionWithDetails(
				ironicNames.ConductorName,
				ConditionGetterFunc(IronicConductorConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				condition.DeploymentReadyRunningMessage,
			)
			th.ExpectCondition(
				ironicNames.ConductorName,
				ConditionGetterFunc(IronicConductorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)

			// overall Ironic condition false
			th.ExpectCondition(
				ironicNames.IronicName,
				ConditionGetterFunc(IronicConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("shows the IronicInspector deployment progressing in DeploymentReadyCondition", func() {
			// IronicInspector - deployment progressing
			th.ExpectConditionWithDetails(
				ironicNames.InspectorName,
				ConditionGetterFunc(IronicInspectorConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				condition.DeploymentReadyRunningMessage,
			)
			th.ExpectCondition(
				ironicNames.InspectorName,
				ConditionGetterFunc(IronicInspectorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)

			// overall Ironic condition false
			th.ExpectCondition(
				ironicNames.IronicName,
				ConditionGetterFunc(IronicConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("shows the IronicNeutronAgent deployment progressing in DeploymentReadyCondition", func() {
			// IronicNeutronAgent - deployment progressing
			th.ExpectConditionWithDetails(
				inaName,
				ConditionGetterFunc(INAConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				condition.DeploymentReadyRunningMessage,
			)
			th.ExpectCondition(
				inaName,
				ConditionGetterFunc(INAConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)

			// overall Ironic condition false
			th.ExpectCondition(
				ironicNames.IronicName,
				ConditionGetterFunc(IronicConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("still shows the IronicAPI deployment progressing in DeploymentReadyCondition when rollout hits ProgressDeadlineExceeded", func() {
			th.SimulateDeploymentProgressDeadlineExceeded(ironicNames.IronicName)
			// IronicAPI - deployment progressing
			th.ExpectConditionWithDetails(
				ironicNames.APIName,
				ConditionGetterFunc(IronicAPIConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				condition.DeploymentReadyRunningMessage,
			)
			th.ExpectCondition(
				ironicNames.APIName,
				ConditionGetterFunc(IronicAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)

			// overall Ironic condition false
			th.ExpectCondition(
				ironicNames.IronicName,
				ConditionGetterFunc(IronicConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("still shows the IronicNeutronAgent deployment progressing in DeploymentReadyCondition when rollout hits ProgressDeadlineExceeded", func() {
			th.SimulateDeploymentProgressDeadlineExceeded(ironicNames.INAName)
			// IronicNeutronAgent - deployment progressing
			th.ExpectConditionWithDetails(
				inaName,
				ConditionGetterFunc(INAConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				condition.DeploymentReadyRunningMessage,
			)
			th.ExpectCondition(
				inaName,
				ConditionGetterFunc(INAConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)

			// overall Ironic condition false
			th.ExpectCondition(
				ironicNames.IronicName,
				ConditionGetterFunc(IronicConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("IronicAPI reaches Ready when deployment rollout finished", func() {
			th.ExpectConditionWithDetails(
				ironicNames.APIName,
				ConditionGetterFunc(IronicAPIConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				condition.DeploymentReadyRunningMessage,
			)
			th.ExpectCondition(
				ironicNames.APIName,
				ConditionGetterFunc(IronicAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)

			th.SimulateDeploymentReplicaReady(ironicNames.IronicName)
			th.ExpectCondition(
				ironicNames.APIName,
				ConditionGetterFunc(IronicAPIConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				ironicNames.APIName,
				ConditionGetterFunc(IronicAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)

			// overall Ironic condition false
			th.ExpectCondition(
				ironicNames.IronicName,
				ConditionGetterFunc(IronicConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("IronicConductor reaches Ready when deployment rollout finished", func() {
			th.ExpectConditionWithDetails(
				ironicNames.ConductorName,
				ConditionGetterFunc(IronicConductorConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				condition.DeploymentReadyRunningMessage,
			)
			th.ExpectCondition(
				ironicNames.ConductorName,
				ConditionGetterFunc(IronicConductorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)

			th.SimulateStatefulSetReplicaReady(ironicNames.ConductorName)
			th.ExpectCondition(
				ironicNames.ConductorName,
				ConditionGetterFunc(IronicConductorConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				ironicNames.ConductorName,
				ConditionGetterFunc(IronicConductorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)

			// overall Ironic condition false
			th.ExpectCondition(
				ironicNames.IronicName,
				ConditionGetterFunc(IronicConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("IronicInspector reaches Ready when deployment rollout finished", func() {
			th.ExpectConditionWithDetails(
				ironicNames.InspectorName,
				ConditionGetterFunc(IronicInspectorConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				condition.DeploymentReadyRunningMessage,
			)
			th.ExpectCondition(
				ironicNames.InspectorName,
				ConditionGetterFunc(IronicInspectorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)

			th.SimulateStatefulSetReplicaReady(ironicNames.InspectorName)
			keystone.SimulateKeystoneServiceReady(ironicNames.InspectorName)
			keystone.SimulateKeystoneEndpointReady(ironicNames.InspectorName)
			th.ExpectCondition(
				ironicNames.InspectorName,
				ConditionGetterFunc(IronicInspectorConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				ironicNames.InspectorName,
				ConditionGetterFunc(IronicInspectorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)

			// overall Ironic condition false
			th.ExpectCondition(
				ironicNames.IronicName,
				ConditionGetterFunc(IronicConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("IronicNeutronAgent reaches Ready when deployment rollout finished", func() {
			th.ExpectConditionWithDetails(
				inaName,
				ConditionGetterFunc(INAConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				condition.DeploymentReadyRunningMessage,
			)
			th.ExpectCondition(
				inaName,
				ConditionGetterFunc(INAConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)

			th.SimulateDeploymentReplicaReady(ironicNames.INAName)
			th.ExpectCondition(
				inaName,
				ConditionGetterFunc(INAConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				inaName,
				ConditionGetterFunc(INAConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)

			// overall Ironic condition false
			th.ExpectCondition(
				ironicNames.IronicName,
				ConditionGetterFunc(IronicConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("Ironic overall condition reaches ready when all deployments succeeded", func() {
			th.ExpectCondition(
				ironicNames.APIName,
				ConditionGetterFunc(IronicAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				ironicNames.ConductorName,
				ConditionGetterFunc(IronicConductorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				ironicNames.InspectorName,
				ConditionGetterFunc(IronicInspectorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				inaName,
				ConditionGetterFunc(INAConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)

			// overall Ironic condition false
			th.ExpectCondition(
				ironicNames.IronicName,
				ConditionGetterFunc(IronicConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)

			// set all deployments to finished
			th.SimulateDeploymentReplicaReady(ironicNames.IronicName)
			th.SimulateStatefulSetReplicaReady(ironicNames.ConductorName)
			th.SimulateStatefulSetReplicaReady(ironicNames.InspectorName)
			keystone.SimulateKeystoneServiceReady(ironicNames.InspectorName)
			keystone.SimulateKeystoneEndpointReady(ironicNames.InspectorName)
			th.SimulateDeploymentReplicaReady(ironicNames.INAName)

			th.ExpectCondition(
				ironicNames.APIName,
				ConditionGetterFunc(IronicAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				ironicNames.ConductorName,
				ConditionGetterFunc(IronicConductorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				ironicNames.InspectorName,
				ConditionGetterFunc(IronicInspectorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				inaName,
				ConditionGetterFunc(INAConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)

			// overall Barbican condition true
			th.ExpectCondition(
				ironicNames.IronicName,
				ConditionGetterFunc(IronicConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})

	})

	Context("Ironic is fully deployed", func() {
		keystoneAPIName := types.NamespacedName{}
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateIronicSecret(ironicNames.Namespace, SecretName),
			)
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateMessageBusSecret(ironicNames.Namespace, MessageBusSecretName),
			)

			apiMariaDBAccount, apiMariaDBSecret := mariadb.CreateMariaDBAccountAndSecret(ironicNames.IronicDatabaseAccount, mariadbv1.MariaDBAccountSpec{})
			DeferCleanup(k8sClient.Delete, ctx, apiMariaDBAccount)
			DeferCleanup(k8sClient.Delete, ctx, apiMariaDBSecret)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					ironicNames.Namespace,
					"openstack",
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			keystoneAPIName = keystone.CreateKeystoneAPI(ironicNames.Namespace)
			DeferCleanup(
				keystone.DeleteKeystoneAPI,
				keystoneAPIName)
			DeferCleanup(
				th.DeleteInstance,
				CreateIronic(ironicNames.IronicName, GetDefaultIronicSpec()),
			)
			mariadb.GetMariaDBDatabase(ironicNames.IronicDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(ironicNames.IronicDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(ironicNames.IronicDatabaseName)
			th.SimulateJobSuccess(ironicNames.IronicDBSyncJobName)

			keystone.SimulateKeystoneServiceReady(ironicNames.IronicName)
			keystone.SimulateKeystoneEndpointReady(ironicNames.IronicName)

			mariadb.GetMariaDBDatabase(ironicNames.InspectorDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(ironicNames.InspectorDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(ironicNames.InspectorDatabaseName)
			th.SimulateJobSuccess(ironicNames.InspectorDBSyncJobName)

			nestedINATransportURLName := ironicNames.INATransportURLName
			nestedINATransportURLName.Name = ironicNames.IronicName.Name + "-" + nestedINATransportURLName.Name
			infra.GetTransportURL(nestedINATransportURLName)
			infra.SimulateTransportURLReady(nestedINATransportURLName)

			th.SimulateDeploymentReplicaReady(ironicNames.IronicName)
			th.SimulateStatefulSetReplicaReady(ironicNames.ConductorName)
			th.SimulateStatefulSetReplicaReady(ironicNames.InspectorName)
			keystone.SimulateKeystoneServiceReady(ironicNames.InspectorName)
			keystone.SimulateKeystoneEndpointReady(ironicNames.InspectorName)
			th.SimulateDeploymentReplicaReady(ironicNames.INAName)

			th.ExpectCondition(
				ironicNames.IronicName,
				ConditionGetterFunc(IronicConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("updates the KeystoneAuthURL if keystone internal endpoint changes", func() {
			newInternalEndpoint := "https://keystone-internal"

			keystone.UpdateKeystoneAPIEndpoint(keystoneAPIName, "internal", newInternalEndpoint)
			logger.Info("Reconfigured")

			Eventually(func(g Gomega) {
				confSecret := th.GetSecret(ironicNames.IronicConfigSecretName)
				g.Expect(confSecret).ShouldNot(BeNil())

				conf := confSecret.Data["ironic.conf"]
				g.Expect(string(conf)).Should(
					ContainSubstring("auth_url=%s", newInternalEndpoint))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				confSecret := th.GetSecret(ironicNames.InspectorConfigSecretName)
				g.Expect(confSecret).ShouldNot(BeNil())

				conf := confSecret.Data["01-inspector.conf"]
				g.Expect(string(conf)).Should(
					ContainSubstring("auth_url=%s", newInternalEndpoint))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				confSecret := th.GetSecret(types.NamespacedName{
					Namespace: ironicNames.Namespace,
					Name:      fmt.Sprintf("%s-ironic-neutron-agent-config-data", ironicNames.IronicName.Name),
				})
				g.Expect(confSecret).ShouldNot(BeNil())

				conf := confSecret.Data["01-ironic_neutron_agent.conf"]
				g.Expect(string(conf)).Should(
					ContainSubstring("auth_url=%s", newInternalEndpoint))
			}, timeout, interval).Should(Succeed())
		})
	})

	When("Ironic is created with topologyref", func() {
		var topologyRef, topologyRefAlt *topologyv1.TopoRef
		BeforeEach(func() {
			// Define the two topology references used in this test
			topologyRef = &topologyv1.TopoRef{
				Name:      ironicNames.IronicTopologies[0].Name,
				Namespace: ironicNames.IronicTopologies[0].Namespace,
			}
			topologyRefAlt = &topologyv1.TopoRef{
				Name:      ironicNames.IronicTopologies[1].Name,
				Namespace: ironicNames.IronicTopologies[1].Namespace,
			}
			// Create Test Topologies
			for _, t := range ironicNames.IronicTopologies {
				// Build the topology Spec
				topologySpec, _ := GetSampleTopologySpec(t.Name)
				infra.CreateTopology(t, topologySpec)
			}
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateIronicSecret(ironicNames.Namespace, SecretName),
			)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					ironicNames.Namespace,
					"openstack",
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			DeferCleanup(
				keystone.DeleteKeystoneAPI,
				keystone.CreateKeystoneAPI(ironicNames.Namespace))
			spec := GetDefaultIronicSpec()
			spec["topologyRef"] = map[string]any{
				"name": topologyRef.Name,
			}
			DeferCleanup(
				th.DeleteInstance,
				CreateIronic(ironicNames.IronicName, spec),
			)
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateMessageBusSecret(ironicNames.Namespace, MessageBusSecretName),
			)
			mariadb.GetMariaDBDatabase(ironicNames.IronicDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(ironicNames.IronicDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(ironicNames.IronicDatabaseName)
			th.SimulateJobSuccess(ironicNames.IronicDBSyncJobName)

			keystone.SimulateKeystoneServiceReady(ironicNames.IronicName)
			keystone.SimulateKeystoneEndpointReady(ironicNames.IronicName)

			mariadb.GetMariaDBDatabase(ironicNames.InspectorDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(ironicNames.InspectorDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(ironicNames.InspectorDatabaseName)
			th.SimulateJobSuccess(ironicNames.InspectorDBSyncJobName)

			nestedINATransportURLName := ironicNames.INATransportURLName
			nestedINATransportURLName.Name = ironicNames.IronicName.Name + "-" + nestedINATransportURLName.Name
			infra.GetTransportURL(nestedINATransportURLName)
			infra.SimulateTransportURLReady(nestedINATransportURLName)

			th.SimulateDeploymentReplicaReady(ironicNames.IronicName)
			th.SimulateStatefulSetReplicaReady(ironicNames.ConductorName)
			th.SimulateStatefulSetReplicaReady(ironicNames.InspectorName)
			th.SimulateDeploymentReplicaReady(ironicNames.INAName)
		})

		It("sets topology in CR status", func() {
			Eventually(func(g Gomega) {
				tp := infra.GetTopology(types.NamespacedName{
					Name:      topologyRef.Name,
					Namespace: topologyRef.Namespace,
				})
				finalizers := tp.GetFinalizers()
				g.Expect(finalizers).To(HaveLen(4))
				ironicAPI := GetIronicAPI(ironicNames.APIName)
				g.Expect(ironicAPI.Status.LastAppliedTopology).ToNot(BeNil())
				g.Expect(ironicAPI.Status.LastAppliedTopology).To(Equal(topologyRef))
				g.Expect(finalizers).To(ContainElement(
					fmt.Sprintf("openstack.org/ironicapi-%s", ironicAPI.Name)))

				ironicConductor := GetIronicConductor(ironicNames.ConductorName)
				g.Expect(ironicConductor.Status.LastAppliedTopology).ToNot(BeNil())
				g.Expect(ironicConductor.Status.LastAppliedTopology).To(Equal(topologyRef))
				g.Expect(finalizers).To(ContainElement(
					fmt.Sprintf("openstack.org/ironicconductor-%s", ironicConductor.Name)))

				ironicInspector := GetIronicInspector(ironicNames.InspectorName)
				g.Expect(ironicInspector.Status.LastAppliedTopology).ToNot(BeNil())
				g.Expect(ironicInspector.Status.LastAppliedTopology).To(Equal(topologyRef))
				g.Expect(finalizers).To(ContainElement(
					fmt.Sprintf("openstack.org/ironicinspector-%s", ironicInspector.Name)))
			}, timeout, interval).Should(Succeed())
		})
		It("sets topology in resource specs", func() {
			Eventually(func(g Gomega) {
				_, expectedTopologySpecObj := GetSampleTopologySpec(topologyRef.Name)
				g.Expect(th.GetDeployment(ironicNames.IronicName).Spec.Template.Spec.TopologySpreadConstraints).ToNot(BeNil())
				g.Expect(th.GetDeployment(ironicNames.IronicName).Spec.Template.Spec.TopologySpreadConstraints).To(Equal(expectedTopologySpecObj))
				g.Expect(th.GetDeployment(ironicNames.IronicName).Spec.Template.Spec.Affinity).To(BeNil())

				g.Expect(th.GetStatefulSet(ironicNames.InspectorName).Spec.Template.Spec.TopologySpreadConstraints).ToNot(BeNil())
				g.Expect(th.GetStatefulSet(ironicNames.InspectorName).Spec.Template.Spec.TopologySpreadConstraints).To(Equal(expectedTopologySpecObj))
				g.Expect(th.GetStatefulSet(ironicNames.InspectorName).Spec.Template.Spec.Affinity).To(BeNil())

				g.Expect(th.GetStatefulSet(ironicNames.ConductorName).Spec.Template.Spec.TopologySpreadConstraints).To(Equal(expectedTopologySpecObj))
				g.Expect(th.GetStatefulSet(ironicNames.ConductorName).Spec.Template.Spec.TopologySpreadConstraints).ToNot(BeNil())
				g.Expect(th.GetStatefulSet(ironicNames.ConductorName).Spec.Template.Spec.Affinity).To(BeNil())

				g.Expect(th.GetDeployment(ironicNames.INAName).Spec.Template.Spec.TopologySpreadConstraints).ToNot(BeNil())
				g.Expect(th.GetDeployment(ironicNames.INAName).Spec.Template.Spec.TopologySpreadConstraints).To(Equal(expectedTopologySpecObj))
				g.Expect(th.GetDeployment(ironicNames.INAName).Spec.Template.Spec.Affinity).To(BeNil())
			}, timeout, interval).Should(Succeed())
		})
		It("updates topology when the reference changes", func() {
			expectedTopology := &topologyv1.TopoRef{
				Name:      ironicNames.IronicTopologies[1].Name,
				Namespace: ironicNames.IronicTopologies[1].Namespace,
			}
			var finalizers []string
			Eventually(func(g Gomega) {
				ironic := GetIronic(ironicNames.IronicName)
				ironic.Spec.TopologyRef.Name = topologyRefAlt.Name
				g.Expect(k8sClient.Update(ctx, ironic)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				tp := infra.GetTopology(types.NamespacedName{
					Name:      expectedTopology.Name,
					Namespace: expectedTopology.Namespace,
				})
				finalizers = tp.GetFinalizers()
				g.Expect(finalizers).To(HaveLen(4))

				ironicAPI := GetIronicAPI(ironicNames.APIName)
				g.Expect(ironicAPI.Status.LastAppliedTopology).ToNot(BeNil())
				g.Expect(ironicAPI.Status.LastAppliedTopology).To(Equal(topologyRefAlt))
				g.Expect(finalizers).To(ContainElement(
					fmt.Sprintf("openstack.org/ironicapi-%s", ironicAPI.Name)))

				ironicConductor := GetIronicConductor(ironicNames.ConductorName)
				g.Expect(ironicConductor.Status.LastAppliedTopology).ToNot(BeNil())
				g.Expect(ironicConductor.Status.LastAppliedTopology).To(Equal(topologyRefAlt))
				g.Expect(finalizers).To(ContainElement(
					fmt.Sprintf("openstack.org/ironicconductor-%s", ironicConductor.Name)))

				ironicInspector := GetIronicInspector(ironicNames.InspectorName)
				g.Expect(ironicInspector.Status.LastAppliedTopology).ToNot(BeNil())
				g.Expect(ironicInspector.Status.LastAppliedTopology).To(Equal(topologyRefAlt))
				g.Expect(finalizers).To(ContainElement(
					fmt.Sprintf("openstack.org/ironicinspector-%s", ironicInspector.Name)))

				// Get the previous topology and verify there are no finalizers
				// anymore
				tp = infra.GetTopology(types.NamespacedName{
					Name:      topologyRef.Name,
					Namespace: topologyRef.Namespace,
				})
				g.Expect(tp.GetFinalizers()).To(BeEmpty())
			}, timeout, interval).Should(Succeed())
		})
		It("overrides topology when the reference changes", func() {
			Eventually(func(g Gomega) {
				ironic := GetIronic(ironicNames.IronicName)
				//Patch IronicAPI Spec
				newAPI := GetIronicAPISpec(ironicNames.APIName)
				newAPI.TopologyRef.Name = ironicNames.IronicTopologies[1].Name
				ironic.Spec.IronicAPI = newAPI
				//Patch ironicConductor Spec
				newCnd := GetIronicConductorSpec(ironicNames.ConductorName)
				newCnd.TopologyRef.Name = ironicNames.IronicTopologies[2].Name
				ironic.Spec.IronicConductors[0] = newCnd
				//Patch ironicInspector Spec
				newInsp := GetIronicInspectorSpec(ironicNames.InspectorName)
				newInsp.TopologyRef.Name = ironicNames.IronicTopologies[3].Name
				ironic.Spec.IronicInspector = newInsp
				g.Expect(k8sClient.Update(ctx, ironic)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				expectedTopology := &topologyv1.TopoRef{
					Name:      ironicNames.IronicTopologies[1].Name,
					Namespace: ironicNames.IronicTopologies[1].Namespace,
				}
				tp := infra.GetTopology(types.NamespacedName{
					Name:      expectedTopology.Name,
					Namespace: expectedTopology.Namespace,
				})
				g.Expect(tp.GetFinalizers()).To(HaveLen(1))
				finalizers := tp.GetFinalizers()
				ironicAPI := GetIronicAPI(ironicNames.APIName)
				g.Expect(ironicAPI.Status.LastAppliedTopology).ToNot(BeNil())
				g.Expect(ironicAPI.Status.LastAppliedTopology).To(Equal(expectedTopology))
				g.Expect(finalizers).To(ContainElement(
					fmt.Sprintf("openstack.org/ironicapi-%s", ironicAPI.Name)))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				expectedTopology := &topologyv1.TopoRef{
					Name:      ironicNames.IronicTopologies[2].Name,
					Namespace: ironicNames.IronicTopologies[2].Namespace,
				}
				tp := infra.GetTopology(types.NamespacedName{
					Name:      expectedTopology.Name,
					Namespace: expectedTopology.Namespace,
				})
				g.Expect(tp.GetFinalizers()).To(HaveLen(1))
				finalizers := tp.GetFinalizers()
				ironicConductor := GetIronicConductor(ironicNames.ConductorName)
				g.Expect(ironicConductor.Status.LastAppliedTopology).ToNot(BeNil())
				g.Expect(ironicConductor.Status.LastAppliedTopology).To(Equal(expectedTopology))
				g.Expect(finalizers).To(ContainElement(
					fmt.Sprintf("openstack.org/ironicconductor-%s", ironicConductor.Name)))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				expectedTopology := &topologyv1.TopoRef{
					Name:      ironicNames.IronicTopologies[3].Name,
					Namespace: ironicNames.IronicTopologies[3].Namespace,
				}
				tp := infra.GetTopology(types.NamespacedName{
					Name:      expectedTopology.Name,
					Namespace: expectedTopology.Namespace,
				})
				g.Expect(tp.GetFinalizers()).To(HaveLen(1))
				finalizers := tp.GetFinalizers()
				ironicInspector := GetIronicInspector(ironicNames.InspectorName)
				g.Expect(ironicInspector.Status.LastAppliedTopology).ToNot(BeNil())
				g.Expect(ironicInspector.Status.LastAppliedTopology).To(Equal(expectedTopology))
				g.Expect(finalizers).To(ContainElement(
					fmt.Sprintf("openstack.org/ironicinspector-%s", ironicInspector.Name)))
			}, timeout, interval).Should(Succeed())
		})
		It("removes topologyRef from the spec", func() {
			Eventually(func(g Gomega) {
				ironic := GetIronic(ironicNames.IronicName)
				// Remove the TopologyRef from the existing Ironic .Spec
				ironic.Spec.TopologyRef = nil
				g.Expect(k8sClient.Update(ctx, ironic)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				ironicAPI := GetIronicAPI(ironicNames.APIName)
				g.Expect(ironicAPI.Status.LastAppliedTopology).Should(BeNil())
				ironicConductor := GetIronicConductor(ironicNames.ConductorName)
				g.Expect(ironicConductor.Status.LastAppliedTopology).Should(BeNil())
				ironicInspector := GetIronicInspector(ironicNames.InspectorName)
				g.Expect(ironicInspector.Status.LastAppliedTopology).Should(BeNil())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				g.Expect(th.GetDeployment(ironicNames.IronicName).Spec.Template.Spec.TopologySpreadConstraints).To(BeNil())
				g.Expect(th.GetDeployment(ironicNames.IronicName).Spec.Template.Spec.Affinity).ToNot(BeNil())
				g.Expect(th.GetStatefulSet(ironicNames.InspectorName).Spec.Template.Spec.TopologySpreadConstraints).To(BeNil())
				g.Expect(th.GetStatefulSet(ironicNames.InspectorName).Spec.Template.Spec.Affinity).ToNot(BeNil())
				g.Expect(th.GetStatefulSet(ironicNames.ConductorName).Spec.Template.Spec.TopologySpreadConstraints).To(BeNil())
				g.Expect(th.GetStatefulSet(ironicNames.ConductorName).Spec.Template.Spec.Affinity).ToNot(BeNil())
				g.Expect(th.GetDeployment(ironicNames.INAName).Spec.Template.Spec.TopologySpreadConstraints).To(BeNil())
				g.Expect(th.GetDeployment(ironicNames.INAName).Spec.Template.Spec.Affinity).ToNot(BeNil())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				for _, topology := range ironicNames.IronicTopologies {
					// Get the current topology and verify there are no finalizers
					tp := infra.GetTopology(types.NamespacedName{
						Name:      topology.Name,
						Namespace: topology.Namespace,
					})
					g.Expect(tp.GetFinalizers()).To(BeEmpty())
				}
			}, timeout, interval).Should(Succeed())
		})
	})
	When("Ironic is created with nodeSelector", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateIronicSecret(ironicNames.Namespace, SecretName),
			)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					ironicNames.Namespace,
					"openstack",
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			DeferCleanup(
				keystone.DeleteKeystoneAPI,
				keystone.CreateKeystoneAPI(ironicNames.Namespace))
			spec := GetDefaultIronicSpec()
			spec["nodeSelector"] = map[string]any{
				"foo": "bar",
			}
			DeferCleanup(
				th.DeleteInstance,
				CreateIronic(ironicNames.IronicName, spec),
			)
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateMessageBusSecret(ironicNames.Namespace, MessageBusSecretName),
			)

			mariadb.GetMariaDBDatabase(ironicNames.IronicDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(ironicNames.IronicDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(ironicNames.IronicDatabaseName)
			th.SimulateJobSuccess(ironicNames.IronicDBSyncJobName)

			keystone.SimulateKeystoneServiceReady(ironicNames.IronicName)
			keystone.SimulateKeystoneEndpointReady(ironicNames.IronicName)

			mariadb.GetMariaDBDatabase(ironicNames.InspectorDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(ironicNames.InspectorDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(ironicNames.InspectorDatabaseName)
			th.SimulateJobSuccess(ironicNames.InspectorDBSyncJobName)

			nestedINATransportURLName := ironicNames.INATransportURLName
			nestedINATransportURLName.Name = ironicNames.IronicName.Name + "-" + nestedINATransportURLName.Name
			infra.GetTransportURL(nestedINATransportURLName)
			infra.SimulateTransportURLReady(nestedINATransportURLName)

			th.SimulateDeploymentReplicaReady(ironicNames.IronicName)
			th.SimulateStatefulSetReplicaReady(ironicNames.ConductorName)
			th.SimulateStatefulSetReplicaReady(ironicNames.InspectorName)
			th.SimulateDeploymentReplicaReady(ironicNames.INAName)
		})

		It("sets nodeSelector in resource specs", func() {
			Eventually(func(g Gomega) {
				g.Expect(th.GetJob(ironicNames.IronicDBSyncJobName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(th.GetJob(ironicNames.InspectorDBSyncJobName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(th.GetDeployment(ironicNames.IronicName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(th.GetStatefulSet(ironicNames.InspectorName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(th.GetStatefulSet(ironicNames.ConductorName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(th.GetDeployment(ironicNames.INAName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
			}, timeout, interval).Should(Succeed())
		})

		It("updates nodeSelector in resource specs when changed", func() {
			Eventually(func(g Gomega) {
				g.Expect(th.GetJob(ironicNames.IronicDBSyncJobName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(th.GetJob(ironicNames.InspectorDBSyncJobName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(th.GetDeployment(ironicNames.IronicName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(th.GetStatefulSet(ironicNames.InspectorName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(th.GetStatefulSet(ironicNames.ConductorName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(th.GetDeployment(ironicNames.INAName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				ironic := GetIronic(ironicNames.IronicName)
				newNodeSelector := map[string]string{
					"foo2": "bar2",
				}
				ironic.Spec.NodeSelector = &newNodeSelector
				g.Expect(k8sClient.Update(ctx, ironic)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				th.SimulateJobSuccess(ironicNames.IronicDBSyncJobName)
				th.SimulateJobSuccess(ironicNames.InspectorDBSyncJobName)
				g.Expect(th.GetJob(ironicNames.IronicDBSyncJobName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo2": "bar2"}))
				g.Expect(th.GetJob(ironicNames.InspectorDBSyncJobName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo2": "bar2"}))
				g.Expect(th.GetDeployment(ironicNames.IronicName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo2": "bar2"}))
				g.Expect(th.GetStatefulSet(ironicNames.InspectorName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo2": "bar2"}))
				g.Expect(th.GetStatefulSet(ironicNames.ConductorName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo2": "bar2"}))
				g.Expect(th.GetDeployment(ironicNames.INAName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo2": "bar2"}))
			}, timeout, interval).Should(Succeed())
		})

		It("removes nodeSelector from resource specs when cleared", func() {
			Eventually(func(g Gomega) {
				g.Expect(th.GetJob(ironicNames.IronicDBSyncJobName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(th.GetJob(ironicNames.InspectorDBSyncJobName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(th.GetDeployment(ironicNames.IronicName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(th.GetStatefulSet(ironicNames.InspectorName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(th.GetStatefulSet(ironicNames.ConductorName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(th.GetDeployment(ironicNames.INAName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				ironic := GetIronic(ironicNames.IronicName)
				emptyNodeSelector := map[string]string{}
				ironic.Spec.NodeSelector = &emptyNodeSelector
				g.Expect(k8sClient.Update(ctx, ironic)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				th.SimulateJobSuccess(ironicNames.IronicDBSyncJobName)
				th.SimulateJobSuccess(ironicNames.InspectorDBSyncJobName)
				g.Expect(th.GetJob(ironicNames.IronicDBSyncJobName).Spec.Template.Spec.NodeSelector).To(BeNil())
				g.Expect(th.GetJob(ironicNames.InspectorDBSyncJobName).Spec.Template.Spec.NodeSelector).To(BeNil())
				g.Expect(th.GetDeployment(ironicNames.IronicName).Spec.Template.Spec.NodeSelector).To(BeNil())
				g.Expect(th.GetStatefulSet(ironicNames.InspectorName).Spec.Template.Spec.NodeSelector).To(BeNil())
				g.Expect(th.GetStatefulSet(ironicNames.ConductorName).Spec.Template.Spec.NodeSelector).To(BeNil())
				g.Expect(th.GetDeployment(ironicNames.INAName).Spec.Template.Spec.NodeSelector).To(BeNil())
			}, timeout, interval).Should(Succeed())
		})

		It("removes nodeSelector from resource specs when nilled", func() {
			Eventually(func(g Gomega) {
				g.Expect(th.GetJob(ironicNames.IronicDBSyncJobName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(th.GetJob(ironicNames.InspectorDBSyncJobName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(th.GetDeployment(ironicNames.IronicName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(th.GetStatefulSet(ironicNames.InspectorName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(th.GetStatefulSet(ironicNames.ConductorName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(th.GetDeployment(ironicNames.INAName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				ironic := GetIronic(ironicNames.IronicName)
				ironic.Spec.NodeSelector = nil
				g.Expect(k8sClient.Update(ctx, ironic)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				th.SimulateJobSuccess(ironicNames.IronicDBSyncJobName)
				th.SimulateJobSuccess(ironicNames.InspectorDBSyncJobName)
				g.Expect(th.GetJob(ironicNames.IronicDBSyncJobName).Spec.Template.Spec.NodeSelector).To(BeNil())
				g.Expect(th.GetJob(ironicNames.InspectorDBSyncJobName).Spec.Template.Spec.NodeSelector).To(BeNil())
				g.Expect(th.GetDeployment(ironicNames.IronicName).Spec.Template.Spec.NodeSelector).To(BeNil())
				g.Expect(th.GetStatefulSet(ironicNames.InspectorName).Spec.Template.Spec.NodeSelector).To(BeNil())
				g.Expect(th.GetStatefulSet(ironicNames.ConductorName).Spec.Template.Spec.NodeSelector).To(BeNil())
				g.Expect(th.GetDeployment(ironicNames.INAName).Spec.Template.Spec.NodeSelector).To(BeNil())
			}, timeout, interval).Should(Succeed())
		})

		It("allows nodeSelector service override", func() {
			Eventually(func(g Gomega) {
				g.Expect(th.GetJob(ironicNames.IronicDBSyncJobName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(th.GetJob(ironicNames.InspectorDBSyncJobName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(th.GetDeployment(ironicNames.IronicName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(th.GetStatefulSet(ironicNames.InspectorName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(th.GetStatefulSet(ironicNames.ConductorName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(th.GetDeployment(ironicNames.INAName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				ironic := GetIronic(ironicNames.IronicName)
				apiNodeSelector := map[string]string{
					"foo": "api",
				}
				ironic.Spec.IronicAPI.NodeSelector = &apiNodeSelector
				conductorNodeSelector := map[string]string{
					"foo": "conductor",
				}
				ironic.Spec.IronicConductors[0].NodeSelector = &conductorNodeSelector
				inspectorNodeSelector := map[string]string{
					"foo": "inspector",
				}
				ironic.Spec.IronicInspector.NodeSelector = &inspectorNodeSelector
				INANodeSelector := map[string]string{
					"foo": "ina",
				}
				ironic.Spec.IronicNeutronAgent.NodeSelector = &INANodeSelector

				g.Expect(k8sClient.Update(ctx, ironic)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				th.SimulateJobSuccess(ironicNames.IronicDBSyncJobName)
				th.SimulateJobSuccess(ironicNames.InspectorDBSyncJobName)
				g.Expect(th.GetJob(ironicNames.IronicDBSyncJobName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(th.GetJob(ironicNames.InspectorDBSyncJobName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "inspector"}))
				g.Expect(th.GetDeployment(ironicNames.IronicName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "api"}))
				g.Expect(th.GetStatefulSet(ironicNames.InspectorName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "inspector"}))
				g.Expect(th.GetStatefulSet(ironicNames.ConductorName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "conductor"}))
				g.Expect(th.GetDeployment(ironicNames.INAName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "ina"}))
			}, timeout, interval).Should(Succeed())
		})

		It("allows nodeSelector service override to empty", func() {
			Eventually(func(g Gomega) {
				g.Expect(th.GetJob(ironicNames.IronicDBSyncJobName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(th.GetJob(ironicNames.InspectorDBSyncJobName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(th.GetDeployment(ironicNames.IronicName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(th.GetStatefulSet(ironicNames.InspectorName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(th.GetStatefulSet(ironicNames.ConductorName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(th.GetDeployment(ironicNames.INAName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				ironic := GetIronic(ironicNames.IronicName)
				apiNodeSelector := map[string]string{}
				ironic.Spec.IronicAPI.NodeSelector = &apiNodeSelector
				conductorNodeSelector := map[string]string{}
				ironic.Spec.IronicConductors[0].NodeSelector = &conductorNodeSelector
				inspectorNodeSelector := map[string]string{}
				ironic.Spec.IronicInspector.NodeSelector = &inspectorNodeSelector
				INANodeSelector := map[string]string{}
				ironic.Spec.IronicNeutronAgent.NodeSelector = &INANodeSelector

				g.Expect(k8sClient.Update(ctx, ironic)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				th.SimulateJobSuccess(ironicNames.IronicDBSyncJobName)
				th.SimulateJobSuccess(ironicNames.InspectorDBSyncJobName)
				g.Expect(th.GetJob(ironicNames.IronicDBSyncJobName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(th.GetJob(ironicNames.InspectorDBSyncJobName).Spec.Template.Spec.NodeSelector).To(BeNil())
				g.Expect(th.GetDeployment(ironicNames.IronicName).Spec.Template.Spec.NodeSelector).To(BeNil())
				g.Expect(th.GetStatefulSet(ironicNames.InspectorName).Spec.Template.Spec.NodeSelector).To(BeNil())
				g.Expect(th.GetStatefulSet(ironicNames.ConductorName).Spec.Template.Spec.NodeSelector).To(BeNil())
				g.Expect(th.GetDeployment(ironicNames.INAName).Spec.Template.Spec.NodeSelector).To(BeNil())
			}, timeout, interval).Should(Succeed())
		})

	})

	When("Ironic is created with rpc=oslo and quorum queue enabled transport URL", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateIronicSecret(ironicNames.Namespace, SecretName),
			)
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				infra.CreateTransportURLSecret(ironicNames.Namespace, MessageBusSecretName, true),
			)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					ironicNames.Namespace,
					"openstack",
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			DeferCleanup(
				keystone.DeleteKeystoneAPI,
				keystone.CreateKeystoneAPI(ironicNames.Namespace))
			spec := GetDefaultIronicSpec()
			spec["rpcTransport"] = "oslo"
			spec["transportURLSecret"] = MessageBusSecretName
			DeferCleanup(
				th.DeleteInstance,
				CreateIronic(ironicNames.IronicName, spec),
			)
		})

		It("generates ironic config with oslo_messaging_rabbit section when quorum queues enabled", func() {

			th.ExpectCondition(
				ironicNames.IronicName,
				ConditionGetterFunc(IronicConditionGetter),
				condition.ServiceAccountReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				ironicNames.IronicName,
				ConditionGetterFunc(IronicConditionGetter),
				condition.RoleReadyCondition,
				corev1.ConditionTrue,
			)

			th.ExpectCondition(
				ironicNames.IronicName,
				ConditionGetterFunc(IronicConditionGetter),
				condition.RoleBindingReadyCondition,
				corev1.ConditionTrue,
			)

			infra.SimulateTransportURLReady(ironicNames.IronicTransportURLName)
			th.ExpectCondition(
				ironicNames.IronicName,
				ConditionGetterFunc(IronicConditionGetter),
				condition.RabbitMqTransportURLReadyCondition,
				corev1.ConditionTrue,
			)

			mariadb.GetMariaDBDatabase(ironicNames.IronicDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(ironicNames.IronicDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(ironicNames.IronicDatabaseName)

			th.ExpectCondition(
				ironicNames.IronicName,
				ConditionGetterFunc(IronicConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)

			th.ExpectCondition(
				ironicNames.IronicName,
				ConditionGetterFunc(IronicConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)

			configDataMap := th.GetSecret(ironicNames.IronicConfigSecretName)
			Expect(configDataMap).ShouldNot(BeNil())
			Expect(configDataMap.Data).Should(HaveKey("ironic.conf"))
			configData := string(configDataMap.Data["ironic.conf"])

			Expect(configData).Should(ContainSubstring("[oslo_messaging_rabbit]"))
			Expect(configData).Should(ContainSubstring("rabbit_quorum_queue=true"))
			Expect(configData).Should(ContainSubstring("rabbit_transient_quorum_queue=true"))
			Expect(configData).Should(ContainSubstring("amqp_durable_queues=true"))
		})
	})

	// Run MariaDBAccount suite tests.  these are pre-packaged ginkgo tests
	// that exercise standard account create / update patterns that should be
	// common to all controllers that ensure MariaDBAccount CRs.
	mariadbSuite := &mariadb_test.MariaDBTestHarness{
		PopulateHarness: func(harness *mariadb_test.MariaDBTestHarness) {
			harness.Setup(
				"Ironic",
				ironicNames.Namespace,
				ironicNames.IronicDatabaseName.Name,
				"openstack.org/ironic",
				mariadb,
				timeout,
				interval,
			)
		},
		// Generate a fully running Ironic service given an accountName
		// needs to make it all the way to the end where the mariadb finalizers
		// are removed from unused accounts since that's part of what we are testing
		SetupCR: func(accountName types.NamespacedName) {
			spec := GetDefaultIronicSpec()

			spec["databaseAccount"] = accountName.Name

			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateIronicSecret(ironicNames.Namespace, SecretName),
			)
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateMessageBusSecret(ironicNames.Namespace, MessageBusSecretName),
			)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					ironicNames.Namespace,
					"openstack",
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			DeferCleanup(
				keystone.DeleteKeystoneAPI,
				keystone.CreateKeystoneAPI(ironicNames.Namespace))

			spec["rpcTransport"] = "oslo"
			spec["transportURLSecret"] = MessageBusSecretName
			DeferCleanup(
				th.DeleteInstance,
				CreateIronic(ironicNames.IronicName, spec),
			)
			infra.GetTransportURL(ironicNames.IronicTransportURLName)
			infra.SimulateTransportURLReady(ironicNames.IronicTransportURLName)
			mariadb.GetMariaDBDatabase(ironicNames.IronicDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(accountName)
			mariadb.SimulateMariaDBDatabaseCompleted(ironicNames.IronicDatabaseName)
			th.SimulateJobSuccess(ironicNames.IronicDBSyncJobName)
			th.ExpectCondition(
				ironicNames.IronicName,
				ConditionGetterFunc(IronicConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionTrue,
			)

		},
		// Change the account name in the service to a new name
		UpdateAccount: func(newAccountName types.NamespacedName) {

			Eventually(func(g Gomega) {
				ironic := GetIronic(ironicNames.IronicName)
				ironic.Spec.DatabaseAccount = newAccountName.Name
				g.Expect(th.K8sClient.Update(ctx, ironic)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

		},
		// delete the keystone instance to exercise finalizer removal
		DeleteCR: func() {
			th.DeleteInstance(GetIronic(ironicNames.IronicName))
		},
	}

	mariadbSuite.RunBasicSuite()

	mariadbSuite.RunURLAssertSuite(func(_ types.NamespacedName, username string, password string) {
		Eventually(func(g Gomega) {
			configDataMap := th.GetSecret(ironicNames.IronicConfigSecretName)

			conf := configDataMap.Data["ironic.conf"]

			g.Expect(string(conf)).Should(
				ContainSubstring(fmt.Sprintf("connection=mysql+pymysql://%s:%s@hostname-for-openstack.%s.svc/ironic?read_default_file=/etc/my.cnf",
					username, password, ironicNames.Namespace)))
		}).Should(Succeed())

	})

	When("An ApplicationCredential is created for Ironic", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateIronicSecret(ironicNames.Namespace, SecretName),
			)
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				infra.CreateTransportURLSecret(ironicNames.Namespace, MessageBusSecretName, false),
			)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					ironicNames.Namespace,
					"openstack",
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			DeferCleanup(
				keystone.DeleteKeystoneAPI,
				keystone.CreateKeystoneAPI(ironicNames.Namespace))

			// Create AC secret - the controller reads this directly
			acSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ironicNames.Namespace,
					Name:      "ac-ironic-secret",
				},
				Data: map[string][]byte{
					"AC_ID":     []byte("test-ac-id"),
					"AC_SECRET": []byte("test-ac-secret"),
				},
			}
			DeferCleanup(k8sClient.Delete, ctx, acSecret)
			Expect(k8sClient.Create(ctx, acSecret)).To(Succeed())

			spec := GetDefaultIronicSpec()
			spec["rpcTransport"] = "oslo"
			spec["transportURLSecret"] = MessageBusSecretName
			// Explicitly set the AC secret name to enable AC auth
			spec["auth"] = map[string]any{
				"applicationCredentialSecret": "ac-ironic-secret",
			}
			DeferCleanup(
				th.DeleteInstance,
				CreateIronic(ironicNames.IronicName, spec),
			)
		})

		It("should render ApplicationCredential auth in ironic.conf (parent/dbsync)", func() {
			infra.SimulateTransportURLReady(ironicNames.IronicTransportURLName)
			mariadb.GetMariaDBDatabase(ironicNames.IronicDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(ironicNames.IronicDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(ironicNames.IronicDatabaseName)

			th.ExpectCondition(
				ironicNames.IronicName,
				ConditionGetterFunc(IronicConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				cfgSecret := th.GetSecret(ironicNames.IronicConfigSecretName)
				g.Expect(cfgSecret).NotTo(BeNil())

				conf := string(cfgSecret.Data["ironic.conf"])

				// AC auth is configured
				g.Expect(conf).To(ContainSubstring("auth_type=v3applicationcredential"))
				g.Expect(conf).To(ContainSubstring("application_credential_id = test-ac-id"))
				g.Expect(conf).To(ContainSubstring("application_credential_secret = test-ac-secret"))

				// Password auth fields should not be present
				g.Expect(conf).NotTo(ContainSubstring("auth_type=password"))
				g.Expect(conf).NotTo(ContainSubstring("username=ironic"))
				g.Expect(conf).NotTo(ContainSubstring("project_name=service"))
			}, timeout, interval).Should(Succeed())
		})
	})

})

var _ = Describe("Ironic Webhook", func() {

	BeforeEach(func() {
		err := os.Setenv("OPERATOR_TEMPLATES", "../../templates")
		Expect(err).NotTo(HaveOccurred())
	})

	It("rejects with wrong IronicAPI service override endpoint type", func() {
		spec := GetDefaultIronicSpec()
		apiSpec := GetDefaultIronicAPISpec()
		apiSpec["override"] = map[string]any{
			"service": map[string]any{
				"internal": map[string]any{},
				"wrooong":  map[string]any{},
			},
		}
		spec["ironicAPI"] = apiSpec

		raw := map[string]any{
			"apiVersion": "ironic.openstack.org/v1beta1",
			"kind":       "Ironic",
			"metadata": map[string]any{
				"name":      ironicNames.IronicName.Name,
				"namespace": ironicNames.IronicName.Namespace,
			},
			"spec": spec,
		}

		unstructuredObj := &unstructured.Unstructured{Object: raw}
		_, err := controllerutil.CreateOrPatch(
			th.Ctx, th.K8sClient, unstructuredObj, func() error { return nil })
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(
			ContainSubstring(
				"invalid: spec.ironicAPI.override.service[wrooong]: " +
					"Invalid value: \"wrooong\": invalid endpoint type: wrooong"),
		)
	})
	It("rejects with wrong IronicInspector service override endpoint type", func() {
		spec := GetDefaultIronicSpec()
		apiSpec := GetDefaultIronicInspectorSpec()
		apiSpec["override"] = map[string]any{
			"service": map[string]any{
				"internal": map[string]any{},
				"wrooong":  map[string]any{},
			},
		}
		spec["ironicInspector"] = apiSpec

		raw := map[string]any{
			"apiVersion": "ironic.openstack.org/v1beta1",
			"kind":       "Ironic",
			"metadata": map[string]any{
				"name":      ironicNames.IronicName.Name,
				"namespace": ironicNames.IronicName.Namespace,
			},
			"spec": spec,
		}

		unstructuredObj := &unstructured.Unstructured{Object: raw}
		_, err := controllerutil.CreateOrPatch(
			th.Ctx, th.K8sClient, unstructuredObj, func() error { return nil })
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(
			ContainSubstring(
				"invalid: spec.ironicInspector.override.service[wrooong]: " +
					"Invalid value: \"wrooong\": invalid endpoint type: wrooong"),
		)
	})

	DescribeTable("rejects wrong topology for",
		func(serviceNameFunc func() (string, string)) {

			component, errorPath := serviceNameFunc()
			expectedErrorMessage := fmt.Sprintf("spec.%s.namespace: Invalid value: \"namespace\": Customizing namespace field is not supported", errorPath)

			spec := GetDefaultIronicSpec()

			// API, Inspector, NeutronAgent
			if component != "top-level" && component != "ironicConductors" {
				spec[component] = map[string]any{
					"topologyRef": map[string]any{
						"name":      "bar",
						"namespace": "foo",
					},
				}
			}
			// Conductors
			if component == "ironicConductors" {
				condList := []map[string]any{
					{
						"topologyRef": map[string]any{
							"name":      "foo",
							"namespace": "bar",
						},
					},
				}
				spec["ironicConductors"] = condList
				// top-level topologyRef
			} else {
				spec["topologyRef"] = map[string]any{
					"name":      "bar",
					"namespace": "foo",
				}
			}
			// Build the ironic CR
			raw := map[string]any{
				"apiVersion": "ironic.openstack.org/v1beta1",
				"kind":       "Ironic",
				"metadata": map[string]any{
					"name":      ironicNames.IronicName.Name,
					"namespace": ironicNames.IronicName.Namespace,
				},
				"spec": spec,
			}
			unstructuredObj := &unstructured.Unstructured{Object: raw}
			_, err := controllerutil.CreateOrPatch(
				th.Ctx, th.K8sClient, unstructuredObj, func() error { return nil })
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(
				ContainSubstring(expectedErrorMessage))
		},
		Entry("top-level topologyRef", func() (string, string) {
			return "top-level", "topologyRef"
		}),
		Entry("ironicAPI topologyRef", func() (string, string) {
			component := "ironicAPI"
			return component, fmt.Sprintf("%s.topologyRef", component)
		}),
		Entry("ironicInspector topologyRef", func() (string, string) {
			component := "ironicInspector"
			return component, fmt.Sprintf("%s.topologyRef", component)
		}),
		Entry("ironicNeutronAgent topologyRef", func() (string, string) {
			component := "ironicNeutronAgent"
			return component, fmt.Sprintf("%s.topologyRef", component)
		}),
		Entry("ironicConductor topologyRef", func() (string, string) {
			component := "ironicConductors"
			return component, fmt.Sprintf("%s.topologyRef", component)
		}),
	)

	When("Ironic starts with notifications enabled and then disables them", func() {
		var notificationsTransportURLName types.NamespacedName

		BeforeEach(func() {
			spec := GetDefaultIronicSpec()
			spec["rpcTransport"] = "oslo"
			spec["transportURLSecret"] = MessageBusSecretName
			// Null out deprecated field before setting new notificationsBus field
			spec["rabbitMqClusterName"] = ""
			spec["notificationsBus"] = map[string]any{
				"cluster": "notifications-rabbitmq",
				"user":    "ironic-notifications",
				"vhost":   "/notifications",
			}

			DeferCleanup(k8sClient.Delete, ctx, CreateIronicSecret(ironicNames.Namespace, SecretName))
			DeferCleanup(k8sClient.Delete, ctx, CreateMessageBusSecret(ironicNames.Namespace, MessageBusSecretName))
			DeferCleanup(mariadb.DeleteDBService, mariadb.CreateDBService(
				ironicNames.Namespace,
				"openstack",
				corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 3306}},
				},
			))
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(ironicNames.Namespace))
			DeferCleanup(th.DeleteInstance, CreateIronic(ironicNames.IronicName, spec))

			notificationsTransportURLName = types.NamespacedName{
				Namespace: ironicNames.Namespace,
				Name:      ironicNames.IronicName.Name + "-transport-notifications",
			}
		})

		It("should initially have notifications enabled", func() {
			// First set up the main TransportURL
			infra.GetTransportURL(ironicNames.IronicTransportURLName)
			infra.SimulateTransportURLReady(ironicNames.IronicTransportURLName)

			// Now wait for controller to create the notifications TransportURL, then simulate it as ready
			infra.GetTransportURL(notificationsTransportURLName)
			DeferCleanup(k8sClient.Delete, ctx, CreateTransportURLSecret(types.NamespacedName{
				Namespace: ironicNames.Namespace,
				Name:      "notifications-rabbitmq-secret",
			}))
			infra.SimulateTransportURLReady(notificationsTransportURLName)

			Eventually(func(g Gomega) {
				ironic := GetIronic(ironicNames.IronicName)
				g.Expect(ironic.Status.NotificationsURLSecret).ToNot(BeNil())
				g.Expect(*ironic.Status.NotificationsURLSecret).ToNot(BeEmpty())
			}, timeout, interval).Should(Succeed())
		})

		It("should disable notifications when notificationsBus is removed", func() {
			// First set up the main TransportURL
			infra.GetTransportURL(ironicNames.IronicTransportURLName)
			infra.SimulateTransportURLReady(ironicNames.IronicTransportURLName)

			// Now wait for controller to create the notifications TransportURL, then simulate it as ready
			infra.GetTransportURL(notificationsTransportURLName)
			DeferCleanup(k8sClient.Delete, ctx, CreateTransportURLSecret(types.NamespacedName{
				Namespace: ironicNames.Namespace,
				Name:      "notifications-rabbitmq-secret",
			}))
			infra.SimulateTransportURLReady(notificationsTransportURLName)

			// Verify notifications are initially enabled
			Eventually(func(g Gomega) {
				ironic := GetIronic(ironicNames.IronicName)
				g.Expect(ironic.Status.NotificationsURLSecret).ToNot(BeNil())
				g.Expect(*ironic.Status.NotificationsURLSecret).ToNot(BeEmpty())
			}, timeout, interval).Should(Succeed())

			// Update the Ironic spec to remove notificationsBus
			Eventually(func(g Gomega) {
				ironic := GetIronic(ironicNames.IronicName)
				ironic.Spec.NotificationsBus = nil
				g.Expect(k8sClient.Update(ctx, ironic)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			// Wait for notifications to be disabled
			Eventually(func(g Gomega) {
				ironic := GetIronic(ironicNames.IronicName)
				g.Expect(ironic.Status.NotificationsURLSecret).To(BeNil())
			}, timeout, interval).Should(Succeed())
		})
	})
})
