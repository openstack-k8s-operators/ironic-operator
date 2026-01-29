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

	"github.com/google/uuid"
	ironicv1 "github.com/openstack-k8s-operators/ironic-operator/api/v1beta1"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	mariadb_test "github.com/openstack-k8s-operators/mariadb-operator/api/test/helpers"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
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
				keystone.CreateKeystoneAPI(ironicNames.Namespace),
			)
			spec := GetDefaultIronicInspectorSpec()
			spec["rpcTransport"] = "oslo"
			DeferCleanup(
				th.DeleteInstance,
				CreateIronicInspector(ironicNames.InspectorName, spec))
		})
		It("should have the Spec fields initialized", func() {
			instance := GetIronicInspector(ironicNames.InspectorName)
			Expect(instance.Spec.DatabaseInstance).Should(Equal("openstack"))
			Expect(instance.Spec.MessagingBus.Cluster).Should(Equal("rabbitmq"))
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
			}, timeout, interval).Should(ContainElement("openstack.org/ironicinspector"))
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
		It("Creates Config Secrets and gets Secrets (input)", func() {
			infra.GetTransportURL(ironicNames.InspectorTransportURLName)
			infra.SimulateTransportURLReady(ironicNames.InspectorTransportURLName)
			mariadb.GetMariaDBDatabase(ironicNames.InspectorDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(ironicNames.InspectorDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(ironicNames.InspectorDatabaseName)
			cm := th.GetSecret(ironicNames.InspectorConfigSecretName)
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
			mariadb.SimulateMariaDBAccountCompleted(ironicNames.InspectorDatabaseAccount)
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
			mariadb.SimulateMariaDBAccountCompleted(ironicNames.InspectorDatabaseAccount)
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
			mariadb.SimulateMariaDBAccountCompleted(ironicNames.InspectorDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(ironicNames.InspectorDatabaseName)
			th.SimulateJobSuccess(ironicNames.InspectorDBSyncJobName)
			th.ExpectCondition(
				ironicNames.InspectorName,
				ConditionGetterFunc(IronicInspectorConditionGetter),
				condition.CreateServiceReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("Creates StatefulSet and set status fields - Deployment is Ready", func() {
			infra.GetTransportURL(ironicNames.InspectorTransportURLName)
			infra.SimulateTransportURLReady(ironicNames.InspectorTransportURLName)
			mariadb.GetMariaDBDatabase(ironicNames.InspectorDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(ironicNames.InspectorDatabaseAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(ironicNames.InspectorDatabaseName)
			th.SimulateJobSuccess(ironicNames.InspectorDBSyncJobName)
			th.SimulateStatefulSetReplicaReady(ironicNames.InspectorName)
			keystone.SimulateKeystoneServiceReady(ironicNames.InspectorName)
			keystone.SimulateKeystoneEndpointReady(ironicNames.InspectorName)

			ss := th.GetStatefulSet(ironicNames.InspectorName)
			// Check the resulting deployment fields
			Expect(int(*ss.Spec.Replicas)).To(Equal(1))
			Expect(ss.Spec.Template.Spec.Volumes).To(HaveLen(5))
			Expect(ss.Spec.Template.Spec.Containers).To(HaveLen(4))

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
			mariadb.SimulateMariaDBAccountCompleted(ironicNames.InspectorDatabaseAccount)
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
			mariadb.SimulateMariaDBAccountCompleted(ironicNames.InspectorDatabaseAccount)
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
			spec := GetDefaultIronicInspectorSpec()
			spec["rpcTransport"] = "oslo"
			spec["tls"] = map[string]any{
				"api": map[string]any{
					"internal": map[string]any{
						"secretName": ironicNames.InternalCertSecretName.Name,
					},
					"public": map[string]any{
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
			mariadb.SimulateMariaDBAccountCompleted(ironicNames.InspectorDatabaseAccount)
			mariadb.SimulateMariaDBTLSDatabaseCompleted(ironicNames.InspectorDatabaseName)
		})

		It("reports that the CA secret is missing", func() {
			th.ExpectConditionWithDetails(
				ironicNames.InspectorName,
				ConditionGetterFunc(IronicInspectorConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				fmt.Sprintf("TLSInput is missing: %s", ironicNames.CaBundleSecretName.Name),
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
				condition.RequestedReason,
				fmt.Sprintf("TLSInput is missing: secrets \"%s in namespace %s\" not found",
					ironicNames.InternalCertSecretName.Name, ironicNames.InternalCertSecretName.Namespace),
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
				condition.RequestedReason,
				fmt.Sprintf("TLSInput is missing: secrets \"%s in namespace %s\" not found",
					ironicNames.PublicCertSecretName.Name, ironicNames.PublicCertSecretName.Namespace),
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

			th.ExpectCondition(
				ironicNames.InspectorName,
				ConditionGetterFunc(IronicInspectorConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionTrue,
			)

			th.SimulateJobSuccess(ironicNames.InspectorDBSyncJobName)

			th.SimulateStatefulSetReplicaReady(ironicNames.InspectorName)
			keystone.SimulateKeystoneServiceReady(ironicNames.InspectorName)
			keystone.SimulateKeystoneEndpointReady(ironicNames.InspectorName)

			depl := th.GetStatefulSet(ironicNames.InspectorName)
			// Check the resulting deployment fields
			Expect(int(*depl.Spec.Replicas)).To(Equal(1))
			Expect(depl.Spec.Template.Spec.Volumes).To(HaveLen(8))
			Expect(depl.Spec.Template.Spec.Containers).To(HaveLen(4))

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

			configDataMap := th.GetSecret(ironicNames.InspectorConfigSecretName)
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

			th.ExpectCondition(
				ironicNames.InspectorName,
				ConditionGetterFunc(IronicInspectorConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionTrue,
			)

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

			th.ExpectCondition(
				ironicNames.InspectorName,
				ConditionGetterFunc(IronicInspectorConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionTrue,
			)

			th.SimulateJobSuccess(ironicNames.InspectorDBSyncJobName)

			th.SimulateStatefulSetReplicaReady(ironicNames.InspectorName)
			keystone.SimulateKeystoneServiceReady(ironicNames.InspectorName)
			keystone.SimulateKeystoneEndpointReady(ironicNames.InspectorName)

			depl := th.GetStatefulSet(ironicNames.InspectorName)
			// Check the resulting deployment fields
			Expect(int(*depl.Spec.Replicas)).To(Equal(1))
			Expect(depl.Spec.Template.Spec.Volumes).To(HaveLen(8))
			Expect(depl.Spec.Template.Spec.Containers).To(HaveLen(4))

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

	// Run MariaDBAccount suite tests.  these are pre-packaged ginkgo tests
	// that exercise standard account create / update patterns that should be
	// common to all controllers that ensure MariaDBAccount CRs.
	mariadbSuite := &mariadb_test.MariaDBTestHarness{
		PopulateHarness: func(harness *mariadb_test.MariaDBTestHarness) {
			harness.Setup(
				"IronicInspector",
				ironicNames.Namespace,
				ironicNames.InspectorDatabaseName.Name,
				"openstack.org/ironicinspector",
				mariadb,
				timeout,
				interval,
			)
		},
		// Generate a fully running Ironic Inspector service given an accountName
		// needs to make it all the way to the end where the mariadb finalizers
		// are removed from unused accounts since that's part of what we are testing
		SetupCR: func(accountName types.NamespacedName) {
			spec := GetDefaultIronicInspectorSpec()

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
			DeferCleanup(
				th.DeleteInstance,
				CreateIronicInspector(ironicNames.InspectorName, spec))

			infra.GetTransportURL(ironicNames.InspectorTransportURLName)
			infra.SimulateTransportURLReady(ironicNames.InspectorTransportURLName)
			mariadb.GetMariaDBDatabase(ironicNames.InspectorDatabaseName)
			mariadb.SimulateMariaDBAccountCompleted(accountName)
			mariadb.SimulateMariaDBDatabaseCompleted(ironicNames.InspectorDatabaseName)
			th.SimulateJobSuccess(ironicNames.InspectorDBSyncJobName)
			th.SimulateStatefulSetReplicaReady(ironicNames.InspectorName)
			keystone.SimulateKeystoneServiceReady(ironicNames.InspectorName)
			keystone.SimulateKeystoneEndpointReady(ironicNames.InspectorName)

		},
		// Change the account name in the service to a new name
		UpdateAccount: func(newAccountName types.NamespacedName) {

			Eventually(func(g Gomega) {
				inspector := GetIronicInspector(ironicNames.InspectorName)
				inspector.Spec.DatabaseAccount = newAccountName.Name
				g.Expect(th.K8sClient.Update(ctx, inspector)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

		},
		// delete the keystone instance to exercise finalizer removal
		DeleteCR: func() {
			th.DeleteInstance(GetIronicInspector(ironicNames.InspectorName))
		},
	}

	mariadbSuite.RunBasicSuite()

	mariadbSuite.RunURLAssertSuite(func(_ types.NamespacedName, username string, password string) {
		Eventually(func(g Gomega) {
			configDataMap := th.GetSecret(ironicNames.InspectorConfigSecretName)

			conf := configDataMap.Data["01-inspector.conf"]

			g.Expect(string(conf)).Should(
				ContainSubstring(fmt.Sprintf("connection=mysql+pymysql://%s:%s@hostname-for-openstack.%s.svc/ironic_inspector?read_default_file=/etc/my.cnf",
					username, password, ironicNames.Namespace)))
		}).Should(Succeed())
	})

	When("IronicInspector mirrors parent RBAC conditions", func() {

		It("should mirror parent RBAC conditions when child service", func() {
			parentIronic := &ironicv1.Ironic{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ironicNames.IronicName.Name,
					Namespace: ironicNames.Namespace,
				},
				Spec: ironicv1.IronicSpec{
					IronicSpecCore: ironicv1.IronicSpecCore{
						DatabaseInstance: DatabaseInstance,
						Secret:           SecretName,
						APITimeout:       60,
						ServiceUser:      "ironic",
						IronicConductors: []ironicv1.IronicConductorTemplate{{
							StorageRequest: "10G",
						}},
					},
				},
			}
			Expect(k8sClient.Create(ctx, parentIronic)).Should(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, parentIronic)

			// set parent RBAC conditions to true
			Eventually(func(g Gomega) {
				parent := &ironicv1.Ironic{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      ironicNames.IronicName.Name,
					Namespace: ironicNames.Namespace,
				}, parent)).Should(Succeed())

				parent.Status.Conditions.Set(condition.TrueCondition(
					condition.ServiceAccountReadyCondition,
					"ServiceAccount created"))
				parent.Status.Conditions.Set(condition.TrueCondition(
					condition.RoleReadyCondition,
					"Role created"))
				parent.Status.Conditions.Set(condition.TrueCondition(
					condition.RoleBindingReadyCondition,
					"RoleBinding created"))

				g.Expect(k8sClient.Status().Update(ctx, parent)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

			spec := GetDefaultIronicInspectorSpec()
			spec["rpcTransport"] = "oslo"
			spec["transportURLSecret"] = MessageBusSecretName
			inspector := CreateIronicInspector(ironicNames.InspectorName, spec)
			DeferCleanup(th.DeleteInstance, inspector)

			th.ExpectCondition(
				ironicNames.InspectorName,
				ConditionGetterFunc(IronicInspectorConditionGetter),
				condition.ServiceAccountReadyCondition,
				corev1.ConditionTrue,
			)

			// set ironicInspector object is owned by parent ironic object
			Eventually(func(g Gomega) {
				parent := GetIronic(ironicNames.IronicName)
				inspectorObj := GetIronicInspector(ironicNames.InspectorName)
				inspectorObj.SetOwnerReferences([]metav1.OwnerReference{{
					APIVersion:         "ironic.openstack.org/v1beta1",
					Kind:               "Ironic",
					Name:               parent.Name,
					UID:                parent.UID,
					Controller:         ptr.To(true),
					BlockOwnerDeletion: ptr.To(true),
				}})
				g.Expect(k8sClient.Update(ctx, inspectorObj)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

			// checks that inspector mirrors parent conditions
			Eventually(func(g Gomega) {
				inspectorObj := GetIronicInspector(ironicNames.InspectorName)
				serviceAccountCondition := inspectorObj.Status.Conditions.Get(condition.ServiceAccountReadyCondition)
				g.Expect(serviceAccountCondition).ToNot(BeNil())
				g.Expect(serviceAccountCondition.Status).To(Equal(corev1.ConditionTrue))
				g.Expect(serviceAccountCondition.Message).To(Equal("ServiceAccount created"))
			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				ironicNames.InspectorName,
				ConditionGetterFunc(IronicInspectorConditionGetter),
				condition.RoleReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				ironicNames.InspectorName,
				ConditionGetterFunc(IronicInspectorConditionGetter),
				condition.RoleBindingReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})

	When("An ApplicationCredential is created for IronicInspector", func() {
		var (
			namespace             string
			inspectorName         types.NamespacedName
			acSecretName          string
			servicePasswordSecret string
		)
		BeforeEach(func() {
			namespace = uuid.New().String()
			th.CreateNamespace(namespace)
			DeferCleanup(th.DeleteNamespace, namespace)

			inspectorName = types.NamespacedName{
				Namespace: namespace,
				Name:      "ironic-inspector-appcred",
			}
			servicePasswordSecret = "ac-test-osp-secret" //nolint:gosec // G101

			// Create OSP secret with passwords (required even when using AppCreds)
			DeferCleanup(k8sClient.Delete, ctx, CreateIronicSecret(namespace, servicePasswordSecret))

			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(namespace))

			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					namespace,
					"openstack",
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)

			acSecretName = "ac-ironic-inspector-secret" //nolint:gosec // G101
			acSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      acSecretName,
				},
				Data: map[string][]byte{
					keystonev1.ACIDSecretKey:     []byte("test-inspector-ac-id"),
					keystonev1.ACSecretSecretKey: []byte("test-inspector-ac-secret"),
				},
			}
			DeferCleanup(k8sClient.Delete, ctx, acSecret)
			Expect(k8sClient.Create(ctx, acSecret)).To(Succeed())

			// Create IronicInspector (serviceUser defaults to "ironic-inspector" in the API)
			spec := GetDefaultIronicInspectorSpec()
			spec["secret"] = servicePasswordSecret
			spec["databaseInstance"] = "openstack"
			spec["databaseAccount"] = "ironic-inspector"
			spec["rpcTransport"] = "json-rpc"
			spec["auth"] = map[string]any{
				"applicationCredentialSecret": acSecretName,
			}

			inspectorAccount := types.NamespacedName{Namespace: namespace, Name: "ironic-inspector"}
			_, _ = mariadb.CreateMariaDBAccountAndSecret(inspectorAccount, mariadbv1.MariaDBAccountSpec{})
			mariadb.CreateMariaDBDatabase(namespace, "ironic-inspector", mariadbv1.MariaDBDatabaseSpec{})

			DeferCleanup(th.DeleteInstance, CreateIronicInspector(inspectorName, spec))

			mariadb.SimulateMariaDBAccountCompleted(inspectorAccount)
			mariadb.SimulateMariaDBDatabaseCompleted(types.NamespacedName{
				Namespace: namespace,
				Name:      "ironic-inspector",
			})
		})

		It("should render ApplicationCredential auth in IronicInspector config", func() {
			configSecretName := types.NamespacedName{
				Namespace: namespace,
				Name:      fmt.Sprintf("%s-config-data", inspectorName.Name),
			}

			Eventually(func(g Gomega) {
				cfgSecret := th.GetSecret(configSecretName)
				g.Expect(cfgSecret).NotTo(BeNil())

				conf := string(cfgSecret.Data["01-inspector.conf"])

				// AC auth is configured with inspector credentials
				g.Expect(conf).To(ContainSubstring("auth_type=v3applicationcredential"))
				g.Expect(conf).To(ContainSubstring("application_credential_id = test-inspector-ac-id"))
				g.Expect(conf).To(ContainSubstring("application_credential_secret = test-inspector-ac-secret"))

				// Password auth fields should not be present
				g.Expect(conf).NotTo(ContainSubstring("auth_type=password"))
				g.Expect(conf).NotTo(ContainSubstring("username=ironic-inspector"))
				g.Expect(conf).NotTo(ContainSubstring("project_name=service"))
			}, timeout, interval).Should(Succeed())
		})
	})

})
