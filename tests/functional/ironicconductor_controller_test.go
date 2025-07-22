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
	"fmt"

	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports

	//revive:disable-next-line:dot-imports
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"

	routev1 "github.com/openshift/api/route/v1"
	ironicv1 "github.com/openstack-k8s-operators/ironic-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/ironic-operator/pkg/ironic"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("IronicConductor controller", func() {
	BeforeEach(func() {
		apiMariaDBAccount, apiMariaDBSecret := mariadb.CreateMariaDBAccountAndSecret(ironicNames.IronicDatabaseAccount, mariadbv1.MariaDBAccountSpec{})
		DeferCleanup(k8sClient.Delete, ctx, apiMariaDBAccount)
		DeferCleanup(k8sClient.Delete, ctx, apiMariaDBSecret)
	})

	When("IronicConductor is created with rpcTransport == oslo", func() {
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
			spec := GetDefaultIronicConductorSpec()
			spec["rpcTransport"] = "oslo"
			spec["transportURLSecret"] = MessageBusSecretName
			DeferCleanup(
				th.DeleteInstance,
				CreateIronicConductor(ironicNames.ConductorName, spec))
			mariadb.CreateMariaDBDatabase(ironicNames.Namespace, ironic.DatabaseName, mariadbv1.MariaDBDatabaseSpec{})
			mariadb.SimulateMariaDBAccountCompleted(ironicNames.IronicDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(ironicNames.IronicDatabaseName)
		})
		It("should have the Spec fields initialized", func() {
			instance := GetIronicConductor(ironicNames.ConductorName)
			Expect(instance.Spec.ServiceUser).Should(Equal("ironic"))
			Expect(instance.Spec.Standalone).Should(BeFalse())
			Expect(instance.Spec.PasswordSelectors).Should(Equal(
				ironicv1.PasswordSelector{
					Service: "IronicPassword",
				}))
			Expect(instance.Spec.CustomServiceConfig).Should(Equal("# add your customization here"))
			Expect(instance.Spec.StorageClass).Should(Equal(""))
		})
		It("initializes Status fields", func() {
			instance := GetIronicConductor(ironicNames.ConductorName)
			Expect(instance.Status.Hash).To(BeEmpty())
			Expect(instance.Status.NetworkAttachments).To(BeEmpty())
			Expect(instance.Status.ReadyCount).To(Equal(int32(0)))
		})
		It("should have a finalizer", func() {
			Eventually(func() []string {
				return GetIronicConductor(ironicNames.ConductorName).Finalizers
			}, timeout, interval).Should(ContainElement("openstack.org/ironicconductor"))
		})
		It("creates service account, role and rolebindig", func() {
			th.ExpectCondition(
				ironicNames.ConductorName,
				ConditionGetterFunc(IronicConductorConditionGetter),
				condition.ServiceAccountReadyCondition,
				corev1.ConditionTrue,
			)
			sa := th.GetServiceAccount(ironicNames.ConductorServiceAccount)
			th.ExpectCondition(
				ironicNames.ConductorName,
				ConditionGetterFunc(IronicConductorConditionGetter),
				condition.RoleReadyCondition,
				corev1.ConditionTrue,
			)
			role := th.GetRole(ironicNames.ConductorRole)
			Expect(role.Rules).To(HaveLen(2))
			Expect(role.Rules[0].Resources).To(Equal([]string{"securitycontextconstraints"}))
			Expect(role.Rules[1].Resources).To(Equal([]string{"pods"}))
			th.ExpectCondition(
				ironicNames.ConductorName,
				ConditionGetterFunc(IronicConductorConditionGetter),
				condition.RoleBindingReadyCondition,
				corev1.ConditionTrue,
			)
			binding := th.GetRoleBinding(ironicNames.ConductorRoleBinding)
			Expect(binding.RoleRef.Name).To(Equal(role.Name))
			Expect(binding.Subjects).To(HaveLen(1))
			Expect(binding.Subjects[0].Name).To(Equal(sa.Name))
		})
		It("Creates config Secrets and gets Secrets (input) and set Hash of inputs", func() {
			th.ExpectCondition(
				ironicNames.ConductorName,
				ConditionGetterFunc(IronicConductorConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
			Eventually(func(g Gomega) {
				instance := GetIronicConductor(ironicNames.ConductorName)
				g.Expect(instance.Status.Hash).Should(HaveKeyWithValue("input", Not(BeEmpty())))
			}, timeout, interval).Should(Succeed())
			th.ExpectCondition(
				ironicNames.ConductorName,
				ConditionGetterFunc(IronicConductorConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)
			configDataMap := th.GetSecret(ironicNames.ConductorConfigSecretName)
			Expect(configDataMap).ShouldNot(BeNil())
			Expect(configDataMap.Data).Should(HaveKey("ironic.conf"))
			Expect(configDataMap.Data).Should(HaveKey("my.cnf"))
			configData := string(configDataMap.Data["my.cnf"])
			Expect(configData).To(
				ContainSubstring("[client]\nssl=0"))
		})
		It("Sets NetworkAttachmentsReady", func() {
			th.SimulateStatefulSetReplicaReady(ironicNames.ConductorName)
			th.ExpectCondition(
				ironicNames.ConductorName,
				ConditionGetterFunc(IronicConductorConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("Creates StatefulSet and set status fields - Deployment is Ready", func() {
			th.SimulateStatefulSetReplicaReady(ironicNames.ConductorName)
			th.ExpectCondition(
				ironicNames.ConductorName,
				ConditionGetterFunc(IronicConductorConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("Creates a Service and a Route", func() {
			podIps := map[string][]string{
				"openshift-sdn": {"10.217.1.26"},
			}
			th.SimulateStatefulSetReplicaReadyWithPods(ironicNames.ConductorName, podIps)
			// Route and Service for each replica share the same name
			name := types.NamespacedName{
				Namespace: ironicNames.ConductorName.Namespace,
				Name:      ironicNames.ConductorName.Name + "-0",
			}
			// Verify Service created
			Eventually(func(g Gomega) {
				g.Expect(
					k8sClient.Get(ctx, name, &corev1.Service{}),
				).Should(Succeed())
			}, timeout, interval).Should(Succeed())
			// Verify Route created
			Eventually(func(g Gomega) {
				g.Expect(
					k8sClient.Get(ctx, name, &routev1.Route{}),
				).Should(Succeed())
			}, timeout, interval).Should(Succeed())
		})
		It("Sets ReadyCondition and replica count", func() {
			th.SimulateStatefulSetReplicaReady(ironicNames.ConductorName)
			th.ExpectCondition(
				ironicNames.ConductorName,
				ConditionGetterFunc(IronicConductorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
			instance := GetIronicConductor(ironicNames.ConductorName)
			Expect(instance.Status.ReadyCount).To(Equal(int32(1)))
		})
	})

	When("IronicConductor is created with TLS cert secrets", func() {
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
			spec := GetDefaultIronicConductorSpec()
			spec["rpcTransport"] = "oslo"
			spec["transportURLSecret"] = MessageBusSecretName
			spec["tls"] = map[string]interface{}{
				"caBundleSecretName": ironicNames.CaBundleSecretName.Name,
			}
			DeferCleanup(
				th.DeleteInstance,
				CreateIronicConductor(ironicNames.ConductorName, spec))
			mariadb.CreateMariaDBDatabase(ironicNames.Namespace, ironic.DatabaseName, mariadbv1.MariaDBDatabaseSpec{})
			mariadb.SimulateMariaDBAccountCompleted(ironicNames.IronicDatabaseAccount)
			mariadb.SimulateMariaDBTLSDatabaseCompleted(ironicNames.IronicDatabaseName)
		})

		It("reports that the CA secret is missing", func() {
			th.ExpectConditionWithDetails(
				ironicNames.ConductorName,
				ConditionGetterFunc(IronicConductorConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				fmt.Sprintf("TLSInput is missing: %s", ironicNames.CaBundleSecretName.Name),
			)
			th.ExpectCondition(
				ironicNames.ConductorName,
				ConditionGetterFunc(IronicConductorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("creates a StatefulSet for ironic-conductor service with TLS CA cert attached", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(ironicNames.CaBundleSecretName))
			th.SimulateStatefulSetReplicaReady(ironicNames.ConductorName)

			th.ExpectCondition(
				ironicNames.ConductorName,
				ConditionGetterFunc(IronicConductorConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionTrue,
			)

			depl := th.GetStatefulSet(ironicNames.ConductorName)
			// Check the resulting deployment fields
			Expect(int(*depl.Spec.Replicas)).To(Equal(1))
			Expect(depl.Spec.Template.Spec.Volumes).To(HaveLen(5))
			Expect(depl.Spec.Template.Spec.Containers).To(HaveLen(3))

			// cert deployment volumes
			th.AssertVolumeExists(ironicNames.CaBundleSecretName.Name, depl.Spec.Template.Spec.Volumes)

			// cert volumeMounts
			container := depl.Spec.Template.Spec.Containers[1]
			th.AssertVolumeMountExists(ironicNames.CaBundleSecretName.Name, "tls-ca-bundle.pem", container.VolumeMounts)

			configDataMap := th.GetSecret(ironicNames.ConductorConfigSecretName)
			Expect(configDataMap).ShouldNot(BeNil())
			Expect(configDataMap.Data).Should(HaveKey("ironic.conf"))
			Expect(configDataMap.Data).Should(HaveKey("my.cnf"))
			configData := string(configDataMap.Data["my.cnf"])
			Expect(configData).To(
				ContainSubstring("[client]\nssl-ca=/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem\nssl=1"))
		})

		It("reconfigures the deployment when CA changes", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(ironicNames.CaBundleSecretName))
			th.SimulateStatefulSetReplicaReady(ironicNames.ConductorName)

			th.ExpectCondition(
				ironicNames.ConductorName,
				ConditionGetterFunc(IronicConductorConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionTrue,
			)

			depl := th.GetStatefulSet(ironicNames.ConductorName)
			// Check the resulting deployment fields
			Expect(int(*depl.Spec.Replicas)).To(Equal(1))
			Expect(depl.Spec.Template.Spec.Volumes).To(HaveLen(5))
			Expect(depl.Spec.Template.Spec.Containers).To(HaveLen(3))

			// Grab the current config hash
			originalHash := GetEnvVarValue(
				depl.Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")
			Expect(originalHash).NotTo(BeEmpty())

			// Change the content of the CA secret
			th.UpdateSecret(ironicNames.CaBundleSecretName, "tls-ca-bundle.pem", []byte("DifferentCAData"))

			// Assert that the deployment is updated
			Eventually(func(g Gomega) {
				newHash := GetEnvVarValue(
					th.GetStatefulSet(ironicNames.ConductorName).Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")
				g.Expect(newHash).NotTo(BeEmpty())
				g.Expect(newHash).NotTo(Equal(originalHash))
			}, timeout, interval).Should(Succeed())
		})
	})

	// FIXME(zzzeek) - build and/or update mariadb harness.go to have a URL
	// set/update test that handles all MariaDBAccount creation and does not
	// assume finalizers present
	When("IronicConductor is created for a particular MariaDBAccount", func() {

		BeforeEach(func() {
			oldAccountName := types.NamespacedName{
				Name:      "some-old-account",
				Namespace: ironicNames.Namespace,
			}
			newAccountName := types.NamespacedName{
				Name:      "some-new-account",
				Namespace: ironicNames.Namespace,
			}

			oldMariaDBAccount, oldMariaDBSecret := mariadb.CreateMariaDBAccountAndSecret(oldAccountName, mariadbv1.MariaDBAccountSpec{})
			newMariaDBAccount, newMariaDBSecret := mariadb.CreateMariaDBAccountAndSecret(newAccountName, mariadbv1.MariaDBAccountSpec{})
			DeferCleanup(k8sClient.Delete, ctx, oldMariaDBAccount)
			DeferCleanup(k8sClient.Delete, ctx, oldMariaDBSecret)
			DeferCleanup(k8sClient.Delete, ctx, newMariaDBAccount)
			DeferCleanup(k8sClient.Delete, ctx, newMariaDBSecret)

			spec := GetDefaultIronicConductorSpec()

			spec["databaseAccount"] = oldAccountName.Name
			spec["rpcTransport"] = "oslo"
			spec["transportURLSecret"] = MessageBusSecretName

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

			mariadb.CreateMariaDBDatabase(ironicNames.Namespace, ironic.DatabaseName, mariadbv1.MariaDBDatabaseSpec{})

			DeferCleanup(
				th.DeleteInstance,
				CreateIronicConductor(ironicNames.ConductorName, spec))

			mariadb.SimulateMariaDBAccountCompleted(oldAccountName)
			mariadb.SimulateMariaDBAccountCompleted(newAccountName)
			mariadb.SimulateMariaDBDatabaseCompleted(ironicNames.IronicDatabaseName)

		})

		It("Sets the correct mysql URL", func() {
			accountName := types.NamespacedName{
				Name:      "some-old-account",
				Namespace: ironicNames.Namespace,
			}

			databaseAccount := mariadb.GetMariaDBAccount(accountName)
			databaseSecret := th.GetSecret(types.NamespacedName{Name: databaseAccount.Spec.Secret, Namespace: ironicNames.Namespace})

			instance := GetIronicConductor(ironicNames.ConductorName)
			configDataMap := th.GetSecret(ironicNames.ConductorConfigSecretName)

			conf := configDataMap.Data["ironic.conf"]

			Expect(string(conf)).Should(
				ContainSubstring(fmt.Sprintf("connection=mysql+pymysql://%s:%s@%s/ironic?read_default_file=/etc/my.cnf",
					databaseAccount.Spec.UserName, databaseSecret.Data[mariadbv1.DatabasePasswordSelector], instance.Spec.DatabaseHostname)))
		})

		It("Updates the mysql URL when the account changes", func() {

			newAccountName := types.NamespacedName{
				Name:      "some-new-account",
				Namespace: ironicNames.Namespace,
			}

			Eventually(func(g Gomega) {
				api := GetIronicConductor(ironicNames.ConductorName)
				api.Spec.DatabaseAccount = newAccountName.Name
				g.Expect(th.K8sClient.Update(ctx, api)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

			databaseAccount := mariadb.GetMariaDBAccount(newAccountName)
			databaseSecret := th.GetSecret(types.NamespacedName{Name: databaseAccount.Spec.Secret, Namespace: ironicNames.Namespace})

			instance := GetIronicConductor(ironicNames.ConductorName)

			Eventually(func(g Gomega) {
				configDataMap := th.GetSecret(ironicNames.ConductorConfigSecretName)

				conf := configDataMap.Data["ironic.conf"]

				g.Expect(string(conf)).Should(
					ContainSubstring(fmt.Sprintf("connection=mysql+pymysql://%s:%s@%s/ironic?read_default_file=/etc/my.cnf",
						databaseAccount.Spec.UserName, databaseSecret.Data[mariadbv1.DatabasePasswordSelector], instance.Spec.DatabaseHostname)))
			}).Should(Succeed())
		})

	})

})
