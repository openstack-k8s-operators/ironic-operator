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

package functional_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"
	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("IronicInspector controller", func() {
	When("IronicInspector is created with rpcTransport == oslo", func() {
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
			spec := GetDefaultIronicInspectorSpec()
			spec["rpcTransport"] = "oslo"
			DeferCleanup(
				th.DeleteInstance,
				CreateIronicInspector(ironicNames.InspectorName, spec))
		})
		It("should have the Spec fields initialized", func() {
			instance := GetIronicInspector(ironicNames.InspectorName)
			Expect(instance.Spec.DatabaseInstance).Should(Equal("openstack"))
			Expect(instance.Spec.RabbitMqClusterName).Should(Equal("rabbitmq"))
			Expect(instance.Spec.ServiceUser).Should(Equal("ironic-inspector"))
		})
		It("initializes Status fields", func() {
			instance := GetIronicInspector(ironicNames.InspectorName)
			Expect(instance.Status.Hash).To(BeEmpty())
			Expect(instance.Status.DatabaseHostname).To(BeEmpty())
			Expect(instance.Status.APIEndpoints).To(BeEmpty())
			Expect(instance.Status.NetworkAttachments).To(BeEmpty())
			Expect(instance.Status.ReadyCount).To(Equal(int32(0)))
			Expect(instance.Status.TransportURLSecret).To(BeEmpty())
		})
		It("should have a finalizer", func() {
			Eventually(func() []string {
				return GetIronicInspector(ironicNames.InspectorName).Finalizers
			}, timeout, interval).Should(ContainElement("IronicInspector"))
		})
		It("creates service account, role and rolebindig", func() {
			th.ExpectCondition(
				ironicNames.InspectorName,
				ConditionGetterFunc(IronicInspectorConditionGetter),
				condition.ServiceAccountReadyCondition,
				corev1.ConditionTrue,
			)
			sa := th.GetServiceAccount(ironicNames.InspectorServiceAccount)
			th.ExpectCondition(
				ironicNames.InspectorName,
				ConditionGetterFunc(IronicInspectorConditionGetter),
				condition.RoleReadyCondition,
				corev1.ConditionTrue,
			)
			role := th.GetRole(ironicNames.InspectorRole)
			Expect(role.Rules).To(HaveLen(2))
			Expect(role.Rules[0].Resources).To(Equal([]string{"securitycontextconstraints"}))
			Expect(role.Rules[1].Resources).To(Equal([]string{"pods"}))
			th.ExpectCondition(
				ironicNames.InspectorName,
				ConditionGetterFunc(IronicInspectorConditionGetter),
				condition.RoleBindingReadyCondition,
				corev1.ConditionTrue,
			)
			binding := th.GetRoleBinding(ironicNames.InspectorRoleBinding)
			Expect(binding.RoleRef.Name).To(Equal(role.Name))
			Expect(binding.Subjects).To(HaveLen(1))
			Expect(binding.Subjects[0].Name).To(Equal(sa.Name))
		})
		It("creates Transport URL and sets TransportURLSecret status field", func() {
			th.GetTransportURL(ironicNames.InspectorTransportURLName)
			th.SimulateTransportURLReady(ironicNames.InspectorTransportURLName)
			th.ExpectCondition(
				ironicNames.InspectorName,
				ConditionGetterFunc(IronicInspectorConditionGetter),
				condition.RabbitMqTransportURLReadyCondition,
				corev1.ConditionTrue,
			)
			instance := GetIronicInspector(ironicNames.InspectorName)
			Expect(instance.Status.TransportURLSecret).To(Equal("rabbitmq-secret"))
		})
		It("Creates ConfigMaps and gets Secrets (input)", func() {
			th.GetTransportURL(ironicNames.InspectorTransportURLName)
			th.SimulateTransportURLReady(ironicNames.InspectorTransportURLName)
			th.ExpectCondition(
				ironicNames.InspectorName,
				ConditionGetterFunc(IronicInspectorConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				ironicNames.InspectorName,
				ConditionGetterFunc(IronicInspectorConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("Creates service database instance", func() {
			th.GetTransportURL(ironicNames.InspectorTransportURLName)
			th.SimulateTransportURLReady(ironicNames.InspectorTransportURLName)
			mariadb.GetMariaDBDatabase(ironicNames.InspectorDatabaseName)
			mariadb.SimulateMariaDBDatabaseCompleted(ironicNames.InspectorDatabaseName)
			th.ExpectCondition(
				ironicNames.InspectorName,
				ConditionGetterFunc(IronicInspectorConditionGetter),
				condition.DBReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("Runs service database DBsync", func() {
			th.GetTransportURL(ironicNames.InspectorTransportURLName)
			th.SimulateTransportURLReady(ironicNames.InspectorTransportURLName)
			mariadb.GetMariaDBDatabase(ironicNames.InspectorDatabaseName)
			mariadb.SimulateMariaDBDatabaseCompleted(ironicNames.InspectorDatabaseName)
			th.SimulateJobSuccess(ironicNames.InspectorDBSyncJobName)
			th.ExpectCondition(
				ironicNames.InspectorName,
				ConditionGetterFunc(IronicInspectorConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("Exposes services", func() {
			th.GetTransportURL(ironicNames.InspectorTransportURLName)
			th.SimulateTransportURLReady(ironicNames.InspectorTransportURLName)
			mariadb.GetMariaDBDatabase(ironicNames.InspectorDatabaseName)
			mariadb.SimulateMariaDBDatabaseCompleted(ironicNames.InspectorDatabaseName)
			th.SimulateJobSuccess(ironicNames.InspectorDBSyncJobName)
			th.ExpectCondition(
				ironicNames.InspectorName,
				ConditionGetterFunc(IronicInspectorConditionGetter),
				condition.ExposeServiceReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("Creates StatefulSet and set status fields - Deployment is Ready", func() {
			th.GetTransportURL(ironicNames.InspectorTransportURLName)
			th.SimulateTransportURLReady(ironicNames.InspectorTransportURLName)
			mariadb.GetMariaDBDatabase(ironicNames.InspectorDatabaseName)
			mariadb.SimulateMariaDBDatabaseCompleted(ironicNames.InspectorDatabaseName)
			th.SimulateJobSuccess(ironicNames.InspectorDBSyncJobName)
			th.SimulateStatefulSetReplicaReady(ironicNames.InspectorName)
			keystone.SimulateKeystoneServiceReady(ironicNames.InspectorName)
			keystone.SimulateKeystoneEndpointReady(ironicNames.InspectorName)
			th.ExpectCondition(
				ironicNames.InspectorName,
				ConditionGetterFunc(IronicInspectorConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("Creates keystone service, users and endpoints", func() {
			th.GetTransportURL(ironicNames.InspectorTransportURLName)
			th.SimulateTransportURLReady(ironicNames.InspectorTransportURLName)
			mariadb.GetMariaDBDatabase(ironicNames.InspectorDatabaseName)
			mariadb.SimulateMariaDBDatabaseCompleted(ironicNames.InspectorDatabaseName)
			th.SimulateJobSuccess(ironicNames.InspectorDBSyncJobName)
			th.SimulateStatefulSetReplicaReady(ironicNames.InspectorName)
			keystone.SimulateKeystoneServiceReady(ironicNames.InspectorName)
			keystone.SimulateKeystoneEndpointReady(ironicNames.InspectorName)
			th.ExpectCondition(
				ironicNames.InspectorName,
				ConditionGetterFunc(IronicInspectorConditionGetter),
				condition.KeystoneServiceReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				ironicNames.InspectorName,
				ConditionGetterFunc(IronicInspectorConditionGetter),
				condition.KeystoneEndpointReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("Sets ReadyCondition and replica count", func() {
			th.GetTransportURL(ironicNames.InspectorTransportURLName)
			th.SimulateTransportURLReady(ironicNames.InspectorTransportURLName)
			mariadb.GetMariaDBDatabase(ironicNames.InspectorDatabaseName)
			mariadb.SimulateMariaDBDatabaseCompleted(ironicNames.InspectorDatabaseName)
			th.SimulateJobSuccess(ironicNames.InspectorDBSyncJobName)
			th.SimulateStatefulSetReplicaReady(ironicNames.InspectorName)
			keystone.SimulateKeystoneServiceReady(ironicNames.InspectorName)
			keystone.SimulateKeystoneEndpointReady(ironicNames.InspectorName)
			th.ExpectCondition(
				ironicNames.InspectorName,
				ConditionGetterFunc(IronicInspectorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
			instance := GetIronicInspector(ironicNames.InspectorName)
			Expect(instance.Status.ReadyCount).To(Equal(int32(1)))
		})
	})
})
