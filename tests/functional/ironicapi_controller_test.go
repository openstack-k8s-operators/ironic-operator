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

	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports

	//revive:disable-next-line:dot-imports
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"

	ironicv1 "github.com/openstack-k8s-operators/ironic-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/ironic-operator/pkg/ironic"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("IronicAPI controller", func() {
	BeforeEach(func() {
		apiMariaDBAccount, apiMariaDBSecret := mariadb.CreateMariaDBAccountAndSecret(ironicNames.IronicDatabaseAccount, mariadbv1.MariaDBAccountSpec{})
		DeferCleanup(k8sClient.Delete, ctx, apiMariaDBAccount)
		DeferCleanup(k8sClient.Delete, ctx, apiMariaDBSecret)
	})

	When("IronicAPI is created with rpcTransport == oslo", func() {
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
			spec := GetDefaultIronicAPISpec()
			spec["rpcTransport"] = "oslo"
			spec["transportURLSecret"] = MessageBusSecretName
			DeferCleanup(
				th.DeleteInstance,
				CreateIronicAPI(ironicNames.APIName, spec))
			mariadb.CreateMariaDBDatabase(ironicNames.Namespace, ironic.DatabaseName, mariadbv1.MariaDBDatabaseSpec{})
			mariadb.SimulateMariaDBAccountCompleted(ironicNames.IronicDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(ironicNames.IronicDatabaseName)
		})
		It("should have the Spec fields initialized", func() {
			instance := GetIronicAPI(ironicNames.APIName)
			Expect(instance.Spec.ServiceUser).Should(Equal("ironic"))
			Expect(instance.Spec.Standalone).Should(BeFalse())
			Expect(instance.Spec.PasswordSelectors).Should(Equal(
				ironicv1.PasswordSelector{
					Service: "IronicPassword",
				}))
			Expect(instance.Spec.CustomServiceConfig).Should(Equal("# add your customization here"))
		})
		It("initializes Status fields", func() {
			instance := GetIronicAPI(ironicNames.APIName)
			Expect(instance.Status.Hash).To(BeEmpty())
			Expect(instance.Status.NetworkAttachments).To(BeEmpty())
			Expect(instance.Status.ReadyCount).To(Equal(int32(0)))
		})
		It("should have a finalizer", func() {
			Eventually(func() []string {
				return GetIronicAPI(ironicNames.APIName).Finalizers
			}, timeout, interval).Should(ContainElement("openstack.org/ironicapi"))
		})
		It("creates service account, role and rolebindig", func() {
			th.ExpectCondition(
				ironicNames.APIName,
				ConditionGetterFunc(IronicAPIConditionGetter),
				condition.ServiceAccountReadyCondition,
				corev1.ConditionTrue,
			)
			sa := th.GetServiceAccount(ironicNames.APIServiceAccount)
			th.ExpectCondition(
				ironicNames.APIName,
				ConditionGetterFunc(IronicAPIConditionGetter),
				condition.RoleReadyCondition,
				corev1.ConditionTrue,
			)
			role := th.GetRole(ironicNames.APIRole)
			Expect(role.Rules).To(HaveLen(2))
			Expect(role.Rules[0].Resources).To(Equal([]string{"securitycontextconstraints"}))
			Expect(role.Rules[1].Resources).To(Equal([]string{"pods"}))
			th.ExpectCondition(
				ironicNames.APIName,
				ConditionGetterFunc(IronicAPIConditionGetter),
				condition.RoleBindingReadyCondition,
				corev1.ConditionTrue,
			)
			binding := th.GetRoleBinding(ironicNames.APIRoleBinding)
			Expect(binding.RoleRef.Name).To(Equal(role.Name))
			Expect(binding.Subjects).To(HaveLen(1))
			Expect(binding.Subjects[0].Name).To(Equal(sa.Name))
		})
		It("Creates config Secrets and gets Secrets (input) and set Hash of inputs", func() {
			th.ExpectCondition(
				ironicNames.APIName,
				ConditionGetterFunc(IronicAPIConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
			Eventually(func(g Gomega) {
				instance := GetIronicAPI(ironicNames.APIName)
				g.Expect(instance.Status.Hash).Should(HaveKeyWithValue("input", Not(BeEmpty())))
			}, timeout, interval).Should(Succeed())
			th.ExpectCondition(
				ironicNames.APIName,
				ConditionGetterFunc(IronicAPIConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("generated configs successfully", func() {
			th.ExpectCondition(
				ironicNames.APIName,
				ConditionGetterFunc(IronicAPIConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)

			configDataMap := th.GetSecret(ironicNames.APIConfigSecretName)
			Expect(configDataMap).ShouldNot(BeNil())
			Expect(configDataMap.Data).Should(HaveKey("ironic.conf"))
			configData := string(configDataMap.Data["ironic.conf"])
			// as part of additional hardening we now require service_token_roles_required
			// to be set to true to ensure that the service token is not just a user token
			// ironic does not currently rely on the service token for enforcement of elevated
			// privileges but this is a good practice to follow and might be required in the
			// future
			Expect(configData).Should(ContainSubstring("service_token_roles_required = true"))

			Expect(configDataMap.Data).Should(HaveKey("my.cnf"))
			configData = string(configDataMap.Data["my.cnf"])
			Expect(configData).To(
				ContainSubstring("[client]\nssl=0"))
		})
		It("Sets NetworkAttachmentsReady", func() {
			th.SimulateDeploymentReplicaReady(ironicNames.IronicName)
			th.ExpectCondition(
				ironicNames.APIName,
				ConditionGetterFunc(IronicAPIConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("Creates Deployment and set status fields - Deployment is Ready", func() {
			keystone.SimulateKeystoneEndpointReady(ironicNames.IronicName)
			keystone.SimulateKeystoneServiceReady(ironicNames.IronicName)
			th.SimulateDeploymentReplicaReady(ironicNames.IronicName)
			th.ExpectCondition(
				ironicNames.APIName,
				ConditionGetterFunc(IronicAPIConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("Creates a Services for internal and public", func() {
			var name types.NamespacedName
			// Verify Service ironic-internal created
			name = types.NamespacedName{
				Namespace: ironicNames.Namespace,
				Name:      "ironic-internal",
			}
			Eventually(func(g Gomega) {
				g.Expect(
					k8sClient.Get(ctx, name, &corev1.Service{}),
				).Should(Succeed())
			}, timeout, interval).Should(Succeed())
			// Verify Service ironic-public created
			name = types.NamespacedName{
				Namespace: ironicNames.Namespace,
				Name:      "ironic-public",
			}
			Eventually(func(g Gomega) {
				g.Expect(
					k8sClient.Get(ctx, name, &corev1.Service{}),
				).Should(Succeed())
			}, timeout, interval).Should(Succeed())
		})
		It("Sets ReadyCondition and replica count", func() {
			keystone.SimulateKeystoneServiceReady(ironicNames.IronicName)
			keystone.SimulateKeystoneEndpointReady(ironicNames.IronicName)
			th.SimulateDeploymentReplicaReady(ironicNames.IronicName)
			th.ExpectCondition(
				ironicNames.APIName,
				ConditionGetterFunc(IronicAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
			instance := GetIronicAPI(ironicNames.APIName)
			Expect(instance.Status.ReadyCount).To(Equal(int32(1)))
		})
	})

	When("IronicAPI is created with TLS cert secrets", func() {
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
			spec := GetDefaultIronicAPISpec()
			spec["rpcTransport"] = "oslo"
			spec["transportURLSecret"] = MessageBusSecretName
			spec["tls"] = map[string]interface{}{
				"api": map[string]interface{}{
					"internal": map[string]interface{}{
						"secretName": ironicNames.InternalCertSecretName.Name,
					},
					"public": map[string]interface{}{
						"secretName": ironicNames.PublicCertSecretName.Name,
					},
				},
				"caBundleSecretName": ironicNames.CaBundleSecretName.Name,
			}
			mariadb.CreateMariaDBDatabase(ironicNames.Namespace, ironic.DatabaseName, mariadbv1.MariaDBDatabaseSpec{})
			mariadb.SimulateMariaDBAccountCompleted(ironicNames.IronicDatabaseAccount)
			mariadb.SimulateMariaDBTLSDatabaseCompleted(ironicNames.IronicDatabaseName)

			DeferCleanup(
				th.DeleteInstance,
				CreateIronicAPI(ironicNames.APIName, spec))
		})

		It("reports that the CA secret is missing", func() {
			th.ExpectConditionWithDetails(
				ironicNames.APIName,
				ConditionGetterFunc(IronicAPIConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				fmt.Sprintf("TLSInput is missing: %s", ironicNames.CaBundleSecretName.Name),
			)
			th.ExpectCondition(
				ironicNames.APIName,
				ConditionGetterFunc(IronicAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("reports that the internal cert secret is missing", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(ironicNames.CaBundleSecretName))

			th.ExpectConditionWithDetails(
				ironicNames.APIName,
				ConditionGetterFunc(IronicAPIConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				fmt.Sprintf("TLSInput is missing: secrets \"%s in namespace %s\" not found",
					ironicNames.InternalCertSecretName.Name, ironicNames.InternalCertSecretName.Namespace),
			)
			th.ExpectCondition(
				ironicNames.APIName,
				ConditionGetterFunc(IronicAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("reports that the public cert secret is missing", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(ironicNames.CaBundleSecretName))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(ironicNames.InternalCertSecretName))

			th.ExpectConditionWithDetails(
				ironicNames.APIName,
				ConditionGetterFunc(IronicAPIConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				fmt.Sprintf("TLSInput is missing: secrets \"%s in namespace %s\" not found",
					ironicNames.PublicCertSecretName.Name, ironicNames.PublicCertSecretName.Namespace),
			)
			th.ExpectCondition(
				ironicNames.APIName,
				ConditionGetterFunc(IronicAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("creates a Deployment for ironic-api service with TLS certs attached", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(ironicNames.CaBundleSecretName))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(ironicNames.InternalCertSecretName))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(ironicNames.PublicCertSecretName))

			th.SimulateDeploymentReplicaReady(ironicNames.IronicName)
			keystone.SimulateKeystoneServiceReady(ironicNames.IronicName)
			keystone.SimulateKeystoneEndpointReady(ironicNames.IronicName)

			th.ExpectCondition(
				ironicNames.APIName,
				ConditionGetterFunc(IronicAPIConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionTrue,
			)

			depl := th.GetDeployment(ironicNames.IronicName)
			// Check the resulting deployment fields
			Expect(int(*depl.Spec.Replicas)).To(Equal(1))
			Expect(depl.Spec.Template.Spec.Volumes).To(HaveLen(8))
			Expect(depl.Spec.Template.Spec.Containers).To(HaveLen(2))

			// cert deployment volumes
			th.AssertVolumeExists(ironicNames.CaBundleSecretName.Name, depl.Spec.Template.Spec.Volumes)
			th.AssertVolumeExists(ironicNames.InternalCertSecretName.Name, depl.Spec.Template.Spec.Volumes)
			th.AssertVolumeExists(ironicNames.PublicCertSecretName.Name, depl.Spec.Template.Spec.Volumes)

			// httpd container certs
			apiContainer := depl.Spec.Template.Spec.Containers[1]
			th.AssertVolumeMountExists(ironicNames.InternalCertSecretName.Name, "tls.key", apiContainer.VolumeMounts)
			th.AssertVolumeMountExists(ironicNames.InternalCertSecretName.Name, "tls.crt", apiContainer.VolumeMounts)
			th.AssertVolumeMountExists(ironicNames.PublicCertSecretName.Name, "tls.key", apiContainer.VolumeMounts)
			th.AssertVolumeMountExists(ironicNames.PublicCertSecretName.Name, "tls.crt", apiContainer.VolumeMounts)
			th.AssertVolumeMountExists(ironicNames.CaBundleSecretName.Name, "tls-ca-bundle.pem", apiContainer.VolumeMounts)

			Expect(apiContainer.ReadinessProbe.HTTPGet.Scheme).To(Equal(corev1.URISchemeHTTPS))
			Expect(apiContainer.LivenessProbe.HTTPGet.Scheme).To(Equal(corev1.URISchemeHTTPS))

			configDataMap := th.GetSecret(ironicNames.APIConfigSecretName)
			Expect(configDataMap).ShouldNot(BeNil())
			Expect(configDataMap.Data).Should(HaveKey("ironic-api-httpd.conf"))
			Expect(configDataMap.Data).Should(HaveKey("ssl.conf"))
			configData := string(configDataMap.Data["ironic-api-httpd.conf"])
			Expect(configData).Should(ContainSubstring("SSLEngine on"))
			Expect(configData).Should(ContainSubstring("SSLCertificateFile      \"/etc/pki/tls/certs/internal.crt\""))
			Expect(configData).Should(ContainSubstring("SSLCertificateKeyFile   \"/etc/pki/tls/private/internal.key\""))
			Expect(configData).Should(ContainSubstring("SSLCertificateFile      \"/etc/pki/tls/certs/public.crt\""))
			Expect(configData).Should(ContainSubstring("SSLCertificateKeyFile   \"/etc/pki/tls/private/public.key\""))

			Expect(configDataMap.Data).Should(HaveKey("my.cnf"))
			configData = string(configDataMap.Data["my.cnf"])
			Expect(configData).To(
				ContainSubstring("[client]\nssl-ca=/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem\nssl=1"))
		})

		It("TLS Endpoints are created", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(ironicNames.CaBundleSecretName))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(ironicNames.InternalCertSecretName))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(ironicNames.PublicCertSecretName))

			th.ExpectCondition(
				ironicNames.APIName,
				ConditionGetterFunc(IronicAPIConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionTrue,
			)

			th.SimulateDeploymentReplicaReady(ironicNames.IronicName)
			keystone.SimulateKeystoneServiceReady(ironicNames.IronicName)
			keystone.SimulateKeystoneEndpointReady(ironicNames.IronicName)

			keystoneEndpoint := keystone.GetKeystoneEndpoint(types.NamespacedName{Namespace: ironicNames.APIName.Namespace, Name: "ironic"})
			endpoints := keystoneEndpoint.Spec.Endpoints
			Expect(endpoints).To(HaveKeyWithValue("public", string("https://ironic-public."+ironicNames.APIName.Namespace+".svc:6385")))
			Expect(endpoints).To(HaveKeyWithValue("internal", "https://ironic-internal."+ironicNames.APIName.Namespace+".svc:6385"))

			th.ExpectCondition(
				ironicNames.APIName,
				ConditionGetterFunc(IronicAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("reconfigures the deployment when CA changes", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(ironicNames.CaBundleSecretName))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(ironicNames.InternalCertSecretName))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(ironicNames.PublicCertSecretName))

			th.ExpectCondition(
				ironicNames.APIName,
				ConditionGetterFunc(IronicAPIConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionTrue,
			)

			th.SimulateDeploymentReplicaReady(ironicNames.IronicName)
			keystone.SimulateKeystoneServiceReady(ironicNames.IronicName)
			keystone.SimulateKeystoneEndpointReady(ironicNames.IronicName)

			depl := th.GetDeployment(ironicNames.IronicName)
			// Check the resulting deployment fields
			Expect(int(*depl.Spec.Replicas)).To(Equal(1))
			Expect(depl.Spec.Template.Spec.Volumes).To(HaveLen(8))
			Expect(depl.Spec.Template.Spec.Containers).To(HaveLen(2))

			// Grab the current config hash
			originalHash := GetEnvVarValue(
				depl.Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")
			Expect(originalHash).NotTo(BeEmpty())

			// Change the content of the CA secret
			th.UpdateSecret(ironicNames.CaBundleSecretName, "tls-ca-bundle.pem", []byte("DifferentCAData"))

			// Assert that the deployment is updated
			Eventually(func(g Gomega) {
				newHash := GetEnvVarValue(
					th.GetDeployment(ironicNames.IronicName).Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")
				g.Expect(newHash).NotTo(BeEmpty())
				g.Expect(newHash).NotTo(Equal(originalHash))
			}, timeout, interval).Should(Succeed())
		})
	})

	// FIXME(zzzeek) - build and/or update mariadb harness.go to have a URL
	// set/update test that handles all MariaDBAccount creation and does not
	// assume finalizers present
	When("IronicAPI is created for a particular MariaDBAccount", func() {

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

			spec := GetDefaultIronicAPISpec()

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
				CreateIronicAPI(ironicNames.APIName, spec))

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

			instance := GetIronicAPI(ironicNames.APIName)
			configDataMap := th.GetSecret(ironicNames.APIConfigSecretName)

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
				api := GetIronicAPI(ironicNames.APIName)
				api.Spec.DatabaseAccount = newAccountName.Name
				g.Expect(th.K8sClient.Update(ctx, api)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

			databaseAccount := mariadb.GetMariaDBAccount(newAccountName)
			databaseSecret := th.GetSecret(types.NamespacedName{Name: databaseAccount.Spec.Secret, Namespace: ironicNames.Namespace})

			instance := GetIronicAPI(ironicNames.APIName)

			Eventually(func(g Gomega) {
				configDataMap := th.GetSecret(ironicNames.APIConfigSecretName)

				conf := configDataMap.Data["ironic.conf"]

				g.Expect(string(conf)).Should(
					ContainSubstring(fmt.Sprintf("connection=mysql+pymysql://%s:%s@%s/ironic?read_default_file=/etc/my.cnf",
						databaseAccount.Spec.UserName, databaseSecret.Data[mariadbv1.DatabasePasswordSelector], instance.Spec.DatabaseHostname)))
			}).Should(Succeed())
		})

	})

})
