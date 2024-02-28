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
			infra.GetTransportURL(ironicNames.InspectorTransportURLName)
			infra.SimulateTransportURLReady(ironicNames.InspectorTransportURLName)
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
			infra.GetTransportURL(ironicNames.InspectorTransportURLName)
			infra.SimulateTransportURLReady(ironicNames.InspectorTransportURLName)
			mariadb.GetMariaDBDatabase(ironicNames.InspectorDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(ironicNames.InspectorDatabaseName)
			mariadb.SimulateMariaDBDatabaseCompleted(ironicNames.InspectorDatabaseName)
			cm := th.GetConfigMap(ironicNames.InspectorConfigDataName)
			myCnf := cm.Data["my.cnf"]
			Expect(myCnf).To(
				ContainSubstring("[client]\nssl=0"))

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
			infra.GetTransportURL(ironicNames.InspectorTransportURLName)
			infra.SimulateTransportURLReady(ironicNames.InspectorTransportURLName)
			mariadb.GetMariaDBDatabase(ironicNames.InspectorDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(ironicNames.InspectorDatabaseName)
			mariadb.SimulateMariaDBDatabaseCompleted(ironicNames.InspectorDatabaseName)
			th.ExpectCondition(
				ironicNames.InspectorName,
				ConditionGetterFunc(IronicInspectorConditionGetter),
				condition.DBReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("Runs service database DBsync", func() {
			infra.GetTransportURL(ironicNames.InspectorTransportURLName)
			infra.SimulateTransportURLReady(ironicNames.InspectorTransportURLName)
			mariadb.GetMariaDBDatabase(ironicNames.InspectorDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(ironicNames.InspectorDatabaseName)
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
			infra.GetTransportURL(ironicNames.InspectorTransportURLName)
			infra.SimulateTransportURLReady(ironicNames.InspectorTransportURLName)
			mariadb.GetMariaDBDatabase(ironicNames.InspectorDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(ironicNames.InspectorDatabaseName)
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
			infra.GetTransportURL(ironicNames.InspectorTransportURLName)
			infra.SimulateTransportURLReady(ironicNames.InspectorTransportURLName)
			mariadb.GetMariaDBDatabase(ironicNames.InspectorDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(ironicNames.InspectorDatabaseName)
			mariadb.SimulateMariaDBDatabaseCompleted(ironicNames.InspectorDatabaseName)
			th.SimulateJobSuccess(ironicNames.InspectorDBSyncJobName)
			th.SimulateStatefulSetReplicaReady(ironicNames.InspectorName)
			keystone.SimulateKeystoneServiceReady(ironicNames.InspectorName)
			keystone.SimulateKeystoneEndpointReady(ironicNames.InspectorName)

			ss := th.GetStatefulSet(ironicNames.InspectorName)
			// Check the resulting deployment fields
			Expect(int(*ss.Spec.Replicas)).To(Equal(1))
			Expect(ss.Spec.Template.Spec.Volumes).To(HaveLen(6))
			Expect(ss.Spec.Template.Spec.Containers).To(HaveLen(3))

			// Check the ironic-inspector-httpd container
			container := ss.Spec.Template.Spec.Containers[0]
			Expect(container.Name).To(Equal("ironic-inspector-httpd"))
			Expect(container.VolumeMounts).To(HaveLen(6))

			// Check the ironic-inspector container
			container = ss.Spec.Template.Spec.Containers[1]
			Expect(container.VolumeMounts).To(HaveLen(6))
			Expect(container.Name).To(Equal("ironic-inspector"))
			Expect(container.LivenessProbe.HTTPGet.Port.IntVal).To(Equal(int32(5050)))
			Expect(container.ReadinessProbe.HTTPGet.Port.IntVal).To(Equal(int32(5050)))

			// Check the ironic-httpboot container
			container = ss.Spec.Template.Spec.Containers[2]
			Expect(container.VolumeMounts).To(HaveLen(6))
			Expect(container.Name).To(Equal("inspector-httpboot"))

			th.ExpectCondition(
				ironicNames.InspectorName,
				ConditionGetterFunc(IronicInspectorConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("Creates keystone service, users and endpoints", func() {
			infra.GetTransportURL(ironicNames.InspectorTransportURLName)
			infra.SimulateTransportURLReady(ironicNames.InspectorTransportURLName)
			mariadb.GetMariaDBDatabase(ironicNames.InspectorDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(ironicNames.InspectorDatabaseName)
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
			infra.GetTransportURL(ironicNames.InspectorTransportURLName)
			infra.SimulateTransportURLReady(ironicNames.InspectorTransportURLName)
			mariadb.GetMariaDBDatabase(ironicNames.InspectorDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(ironicNames.InspectorDatabaseName)
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

	When("IronicInspector is created with TLS cert secrets", func() {
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
			DeferCleanup(
				th.DeleteInstance,
				CreateIronicInspector(ironicNames.InspectorName, spec))

			infra.GetTransportURL(ironicNames.InspectorTransportURLName)
			infra.SimulateTransportURLReady(ironicNames.InspectorTransportURLName)
			mariadb.SimulateMariaDBAccountCompleted(ironicNames.InspectorDatabaseName)
			mariadb.SimulateMariaDBTLSDatabaseCompleted(ironicNames.InspectorDatabaseName)
		})

		It("reports that the CA secret is missing", func() {
			th.ExpectConditionWithDetails(
				ironicNames.InspectorName,
				ConditionGetterFunc(IronicInspectorConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				fmt.Sprintf("TLSInput error occured in TLS sources Secret %s/combined-ca-bundle not found", ironicNames.Namespace),
			)
			th.ExpectCondition(
				ironicNames.InspectorName,
				ConditionGetterFunc(IronicInspectorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("reports that the internal cert secret is missing", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(ironicNames.CaBundleSecretName))

			th.ExpectConditionWithDetails(
				ironicNames.InspectorName,
				ConditionGetterFunc(IronicInspectorConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				fmt.Sprintf("TLSInput error occured in TLS sources Secret %s/internal-tls-certs not found", ironicNames.Namespace),
			)
			th.ExpectCondition(
				ironicNames.InspectorName,
				ConditionGetterFunc(IronicInspectorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("reports that the public cert secret is missing", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(ironicNames.CaBundleSecretName))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(ironicNames.InternalCertSecretName))

			th.ExpectConditionWithDetails(
				ironicNames.InspectorName,
				ConditionGetterFunc(IronicInspectorConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				fmt.Sprintf("TLSInput error occured in TLS sources Secret %s/public-tls-certs not found", ironicNames.Namespace),
			)
			th.ExpectCondition(
				ironicNames.InspectorName,
				ConditionGetterFunc(IronicInspectorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("creates a Deployment for ironic-inspector service with TLS certs attached", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(ironicNames.CaBundleSecretName))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(ironicNames.InternalCertSecretName))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(ironicNames.PublicCertSecretName))

			mariadb.GetMariaDBDatabase(ironicNames.InspectorDatabaseName)
			th.SimulateJobSuccess(ironicNames.InspectorDBSyncJobName)

			th.SimulateStatefulSetReplicaReady(ironicNames.InspectorName)
			keystone.SimulateKeystoneServiceReady(ironicNames.InspectorName)
			keystone.SimulateKeystoneEndpointReady(ironicNames.InspectorName)

			depl := th.GetStatefulSet(ironicNames.InspectorName)
			// Check the resulting deployment fields
			Expect(int(*depl.Spec.Replicas)).To(Equal(1))
			Expect(depl.Spec.Template.Spec.Volumes).To(HaveLen(9))
			Expect(depl.Spec.Template.Spec.Containers).To(HaveLen(3))

			// cert deployment volumes
			th.AssertVolumeExists(ironicNames.CaBundleSecretName.Name, depl.Spec.Template.Spec.Volumes)
			th.AssertVolumeExists(ironicNames.InternalCertSecretName.Name, depl.Spec.Template.Spec.Volumes)
			th.AssertVolumeExists(ironicNames.PublicCertSecretName.Name, depl.Spec.Template.Spec.Volumes)

			// httpd container certs
			container := depl.Spec.Template.Spec.Containers[0]
			th.AssertVolumeMountExists(ironicNames.InternalCertSecretName.Name, "tls.key", container.VolumeMounts)
			th.AssertVolumeMountExists(ironicNames.InternalCertSecretName.Name, "tls.crt", container.VolumeMounts)
			th.AssertVolumeMountExists(ironicNames.PublicCertSecretName.Name, "tls.key", container.VolumeMounts)
			th.AssertVolumeMountExists(ironicNames.PublicCertSecretName.Name, "tls.crt", container.VolumeMounts)
			th.AssertVolumeMountExists(ironicNames.CaBundleSecretName.Name, "tls-ca-bundle.pem", container.VolumeMounts)

			Expect(container.ReadinessProbe.HTTPGet.Scheme).To(Equal(corev1.URISchemeHTTPS))
			Expect(container.LivenessProbe.HTTPGet.Scheme).To(Equal(corev1.URISchemeHTTPS))

			configDataMap := th.GetConfigMap(ironicNames.InspectorConfigDataName)
			Expect(configDataMap).ShouldNot(BeNil())
			Expect(configDataMap.Data).Should(HaveKey("httpd.conf"))
			Expect(configDataMap.Data).Should(HaveKey("ssl.conf"))
			configData := string(configDataMap.Data["httpd.conf"])
			Expect(configData).Should(ContainSubstring("SSLEngine on"))
			Expect(configData).Should(ContainSubstring("SSLCertificateFile      \"/etc/pki/tls/certs/internal.crt\""))
			Expect(configData).Should(ContainSubstring("SSLCertificateKeyFile   \"/etc/pki/tls/private/internal.key\""))
			Expect(configData).Should(ContainSubstring("SSLCertificateFile      \"/etc/pki/tls/certs/public.crt\""))
			Expect(configData).Should(ContainSubstring("SSLCertificateKeyFile   \"/etc/pki/tls/private/public.key\""))
			configData = string(configDataMap.Data["my.cnf"])
			Expect(configData).To(
				ContainSubstring("[client]\nssl-ca=/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem\nssl=1"))
		})

		It("TLS Endpoints are created", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(ironicNames.CaBundleSecretName))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(ironicNames.InternalCertSecretName))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(ironicNames.PublicCertSecretName))

			mariadb.GetMariaDBDatabase(ironicNames.InspectorDatabaseName)
			th.SimulateJobSuccess(ironicNames.InspectorDBSyncJobName)

			th.SimulateStatefulSetReplicaReady(ironicNames.InspectorName)
			keystone.SimulateKeystoneServiceReady(ironicNames.InspectorName)
			keystone.SimulateKeystoneEndpointReady(ironicNames.InspectorName)

			keystoneEndpoint := keystone.GetKeystoneEndpoint(ironicNames.InspectorName)
			endpoints := keystoneEndpoint.Spec.Endpoints
			Expect(endpoints).To(HaveKeyWithValue("public", string("https://ironic-inspector-public."+ironicNames.InspectorName.Namespace+".svc:5050")))
			Expect(endpoints).To(HaveKeyWithValue("internal", "https://ironic-inspector-internal."+ironicNames.InspectorName.Namespace+".svc:5050"))

			th.ExpectCondition(
				ironicNames.InspectorName,
				ConditionGetterFunc(IronicInspectorConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("reconfigures the deployment when CA changes", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(ironicNames.CaBundleSecretName))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(ironicNames.InternalCertSecretName))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(ironicNames.PublicCertSecretName))

			mariadb.GetMariaDBDatabase(ironicNames.InspectorDatabaseName)
			th.SimulateJobSuccess(ironicNames.InspectorDBSyncJobName)

			th.SimulateStatefulSetReplicaReady(ironicNames.InspectorName)
			keystone.SimulateKeystoneServiceReady(ironicNames.InspectorName)
			keystone.SimulateKeystoneEndpointReady(ironicNames.InspectorName)

			depl := th.GetStatefulSet(ironicNames.InspectorName)
			// Check the resulting deployment fields
			Expect(int(*depl.Spec.Replicas)).To(Equal(1))
			Expect(depl.Spec.Template.Spec.Volumes).To(HaveLen(9))
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
					th.GetStatefulSet(ironicNames.InspectorName).Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")
				g.Expect(newHash).NotTo(BeEmpty())
				g.Expect(newHash).NotTo(Equal(originalHash))
			}, timeout, interval).Should(Succeed())
		})
	})
})
