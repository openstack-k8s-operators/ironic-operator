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

	ironicv1 "github.com/openstack-k8s-operators/ironic-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	mariadb_test "github.com/openstack-k8s-operators/mariadb-operator/api/test/helpers"
	corev1 "k8s.io/api/core/v1"
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
					Name:      "ironic-ironic-neutron-agent",
				}, &ironicv1.IronicNeutronAgent{})).Should(Succeed())
			}, th.Timeout, th.Interval).Should(Succeed())
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
			spec["nodeSelector"] = map[string]interface{}{
				"foo": "bar",
			}
			DeferCleanup(
				th.DeleteInstance,
				CreateIronic(ironicNames.IronicName, spec),
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

})

var _ = Describe("Ironic Webhook", func() {

	BeforeEach(func() {
		err := os.Setenv("OPERATOR_TEMPLATES", "../../templates")
		Expect(err).NotTo(HaveOccurred())
	})

	It("rejects with wrong IronicAPI service override endpoint type", func() {
		spec := GetDefaultIronicSpec()
		apiSpec := GetDefaultIronicAPISpec()
		apiSpec["override"] = map[string]interface{}{
			"service": map[string]interface{}{
				"internal": map[string]interface{}{},
				"wrooong":  map[string]interface{}{},
			},
		}
		spec["ironicAPI"] = apiSpec

		raw := map[string]interface{}{
			"apiVersion": "ironic.openstack.org/v1beta1",
			"kind":       "Ironic",
			"metadata": map[string]interface{}{
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
		apiSpec["override"] = map[string]interface{}{
			"service": map[string]interface{}{
				"internal": map[string]interface{}{},
				"wrooong":  map[string]interface{}{},
			},
		}
		spec["ironicInspector"] = apiSpec

		raw := map[string]interface{}{
			"apiVersion": "ironic.openstack.org/v1beta1",
			"kind":       "Ironic",
			"metadata": map[string]interface{}{
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
})
