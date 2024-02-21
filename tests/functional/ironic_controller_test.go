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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ironicv1 "github.com/openstack-k8s-operators/ironic-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"
	mariadb_test "github.com/openstack-k8s-operators/mariadb-operator/api/test/helpers"
	corev1 "k8s.io/api/core/v1"
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
			}, timeout, interval).Should(ContainElement("Ironic"))
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

	// Run MariaDBAccount suite tests.  these are pre-packaged ginkgo tests
	// that exercise standard account create / update patterns that should be
	// common to all controllers that ensure MariaDBAccount CRs.
	mariadbSuite := &mariadb_test.MariaDBTestHarness{
		PopulateHarness: func(harness *mariadb_test.MariaDBTestHarness) {
			harness.Setup(
				"Ironic",
				ironicNames.Namespace,
				ironicNames.IronicDatabaseName.Name,
				"Ironic",
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

	mariadbSuite.RunURLAssertSuite(func(accountName types.NamespacedName, username string, password string) {
		Eventually(func(g Gomega) {
			configDataMap := th.GetSecret(ironicNames.IronicConfigSecretName)

			conf := configDataMap.Data["ironic.conf"]

			g.Expect(string(conf)).Should(
				ContainSubstring(fmt.Sprintf("connection=mysql+pymysql://%s:%s@hostname-for-openstack.%s.svc/ironic?read_default_file=/etc/my.cnf",
					username, password, ironicNames.Namespace)))
		}).Should(Succeed())

	})

})
