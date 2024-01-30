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
	"github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("IronicAPI controller", func() {
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
		})
		It("should have the Spec fields initialized", func() {
			instance := GetIronicAPI(ironicNames.APIName)
			Expect(instance.Spec.ServiceUser).Should(Equal("ironic"))
			Expect(instance.Spec.Standalone).Should(BeFalse())
			Expect(instance.Spec.PasswordSelectors).Should(Equal(
				ironicv1.PasswordSelector{
					Database: "IronicDatabasePassword",
					Service:  "IronicPassword",
				}))
			Expect(instance.Spec.CustomServiceConfig).Should(Equal("# add your customization here"))
			Expect(instance.Spec.Debug.Service).Should(BeFalse())
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
			}, timeout, interval).Should(ContainElement("IronicAPI"))
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
		It("Creates ConfigMaps and gets Secrets (input) and set Hash of inputs", func() {
			th.ExpectCondition(
				ironicNames.APIName,
				ConditionGetterFunc(IronicAPIConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
			instance := GetIronicAPI(ironicNames.APIName)
			Expect(instance.Status.Hash).To(Equal(
				map[string]string{
					common.InputHashName: APIInputHash,
				}))
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
			instance := GetIronicAPI(ironicNames.APIName)
			apiConfigMapName := types.NamespacedName{
				Namespace: instance.Namespace,
				Name:      fmt.Sprintf("%s-config-data", instance.Name),
			}
			configDataMap := th.GetConfigMap(apiConfigMapName)
			Expect(configDataMap).ShouldNot(BeNil())
			Expect(configDataMap.Data).Should(HaveKey("ironic.conf"))
			configData := string(configDataMap.Data["ironic.conf"])
			// as part of additional hardening we now require service_token_roles_required
			// to be set to true to ensure that the service token is not just a user token
			// ironic does not currently rely on the service token for enforcement of elevated
			// privileges but this is a good practice to follow and might be required in the
			// future
			Expect(configData).Should(ContainSubstring("service_token_roles_required = true"))
		})
		It("Sets NetworkAttachmentsReady", func() {
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
})
