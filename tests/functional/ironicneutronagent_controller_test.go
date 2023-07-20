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
	// "encoding/json"
	// "fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ironicv1 "github.com/openstack-k8s-operators/ironic-operator/api/v1beta1"
)

var _ = Describe("IronicNeutronAgent controller", func() {
	When("IronicNeutronAgent is created", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateIronicSecret(ironicNames.Namespace, SecretName),
			)
			DeferCleanup(th.DeleteKeystoneAPI, th.CreateKeystoneAPI(ironicNames.Namespace))
			DeferCleanup(th.DeleteInstance, CreateIronicNeutronAgent(ironicNames.INAName, GetDefaultIronicNeutronAgentSpec()))
		})
		It("initializes Status fields", func() {
			instance := GetIronicNeutronAgent(ironicNames.INAName)
			Expect(instance.Status.Hash).To(BeEmpty())
			Expect(instance.Status.ReadyCount).To(Equal(int32(0)))
			Expect(instance.Status.TransportURLSecret).To(BeEmpty())
		})
		It("creates Transport URL and sets TransportURLSecret status field", func() {
			th.GetTransportURL(ironicNames.INATransportURLName)
			th.SimulateTransportURLReady(ironicNames.INATransportURLName)
			th.ExpectCondition(
				ironicNames.INAName,
				ConditionGetterFunc(INAConditionGetter),
				ironicv1.IronicRabbitMqTransportURLReadyCondition,
				corev1.ConditionTrue,
			)
			instance := GetIronicNeutronAgent(ironicNames.INAName)
			Expect(instance.Status.TransportURLSecret).To(Equal("rabbitmq-secret"))
		})
		It("Creates ConfigMaps and gets Secrets (input)", func() {
			th.GetTransportURL(ironicNames.INATransportURLName)
			th.SimulateTransportURLReady(ironicNames.INATransportURLName)
			th.ExpectCondition(
				ironicNames.INAName,
				ConditionGetterFunc(INAConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				ironicNames.INAName,
				ConditionGetterFunc(INAConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("Creates Deployment and set status fields - is Ready", func() {
			th.GetTransportURL(ironicNames.INATransportURLName)
			th.SimulateTransportURLReady(ironicNames.INATransportURLName)
			th.SimulateDeploymentReplicaReady(ironicNames.INAName)
			th.ExpectCondition(
				ironicNames.INAName,
				ConditionGetterFunc(INAConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				ironicNames.INAName,
				ConditionGetterFunc(INAConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
			instance := GetIronicNeutronAgent(ironicNames.INAName)
			Expect(instance.Status.ReadyCount).To(Equal(int32(1)))
		})
	})

	When("IronicNeutronAgent is created pointing to non existent Secret", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateIronicNeutronAgent(ironicNames.INAName, GetDefaultIronicNeutronAgentSpec()))
			th.GetTransportURL(ironicNames.INATransportURLName)
			th.SimulateTransportURLReady(ironicNames.INATransportURLName)
			DeferCleanup(th.DeleteKeystoneAPI, th.CreateKeystoneAPI(ironicNames.Namespace))
		})
		It("is missing secret", func() {
			th.ExpectConditionWithDetails(
				ironicNames.INAName,
				ConditionGetterFunc(INAConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"Input data resources missing",
			)
		})
		It("is false Ready", func() {
			th.ExpectCondition(
				ironicNames.INAName,
				ConditionGetterFunc(INAConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})
		It("has empty Status fields", func() {
			instance := GetIronicNeutronAgent(ironicNames.INAName)
			Expect(instance.Status.Hash).To(BeEmpty())
			Expect(instance.Status.ReadyCount).To(Equal(int32(0)))
		})
		When("an unrelated Secret is created, CR state does not change", func() {
			BeforeEach(func() {
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "not-relevant-secret",
						Namespace: ironicNames.INAName.Namespace,
					},
				}
				Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
				DeferCleanup(k8sClient.Delete, ctx, secret)
				th.GetTransportURL(ironicNames.INATransportURLName)
				th.SimulateTransportURLReady(ironicNames.INATransportURLName)
			})
			It("is missing secret", func() {
				th.ExpectConditionWithDetails(
					ironicNames.INAName,
					ConditionGetterFunc(INAConditionGetter),
					condition.InputReadyCondition,
					corev1.ConditionFalse,
					condition.RequestedReason,
					"Input data resources missing",
				)
			})
			It("is false Ready", func() {
				th.ExpectCondition(
					ironicNames.INAName,
					ConditionGetterFunc(INAConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionFalse,
				)
			})
			It("has empty Status fields", func() {
				instance := GetIronicNeutronAgent(ironicNames.INAName)
				Expect(instance.Status.Hash).To(BeEmpty())
				Expect(instance.Status.ReadyCount).To(Equal(int32(0)))
			})
		})
		When("the Secret is created", func() {
			BeforeEach(func() {
				DeferCleanup(
					k8sClient.Delete,
					ctx,
					CreateIronicSecret(ironicNames.Namespace, SecretName),
				)
				th.SimulateDeploymentReplicaReady(ironicNames.INAName)
			})
			It("is reporting inputs are ready", func() {

				th.ExpectCondition(
					ironicNames.INAName,
					ConditionGetterFunc(INAConditionGetter),
					condition.InputReadyCondition,
					corev1.ConditionTrue,
				)
			})
			It("is Ready", func() {
				th.ExpectCondition(
					ironicNames.INAName,
					ConditionGetterFunc(INAConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionTrue,
				)
				instance := GetIronicNeutronAgent(ironicNames.INAName)
				Expect(instance.Status.ReadyCount).To(Equal(int32(1)))
			})
		})
	})

})
