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
	routev1 "github.com/openshift/api/route/v1"
	ironicv1 "github.com/openstack-k8s-operators/ironic-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("IronicConductor controller", func() {
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
				th.DeleteDBService,
				th.CreateDBService(
					ironicNames.Namespace,
					"openstack",
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			DeferCleanup(
				th.DeleteKeystoneAPI,
				th.CreateKeystoneAPI(ironicNames.Namespace))
			spec := GetDefaultIronicConductorSpec()
			spec["rpcTransport"] = "oslo"
			spec["transportURLSecret"] = MessageBusSecretName
			DeferCleanup(
				th.DeleteInstance,
				CreateIronicConductor(ironicNames.ConductorName, spec))
		})
		It("should have the Spec fields initialized", func() {
			instance := GetIronicConductor(ironicNames.ConductorName)
			Expect(instance.Spec.ServiceUser).Should(Equal("ironic"))
			Expect(instance.Spec.Standalone).Should(BeFalse())
			Expect(instance.Spec.PasswordSelectors).Should(Equal(
				ironicv1.PasswordSelector{
					Database: "IronicDatabasePassword",
					Service:  "IronicPassword",
				}))
			Expect(instance.Spec.CustomServiceConfig).Should(Equal("# add your customization here"))
			Expect(instance.Spec.StorageClass).Should(Equal(""))
			Expect(instance.Spec.Debug.Service).Should(BeFalse())
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
			}, timeout, interval).Should(ContainElement("IronicConductor"))
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
		It("Creates ConfigMaps and gets Secrets (input) and set Hash of inputs", func() {
			th.ExpectCondition(
				ironicNames.ConductorName,
				ConditionGetterFunc(IronicConductorConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
			instance := GetIronicConductor(ironicNames.ConductorName)
			Expect(instance.Status.Hash).To(Equal(
				map[string]string{
					common.InputHashName: ConductorInputHash,
				}))
			th.ExpectCondition(
				ironicNames.ConductorName,
				ConditionGetterFunc(IronicConductorConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("Sets NetworkAttachmentsReady", func() {
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
})
