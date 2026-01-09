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

	"fmt"

	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports

	//revive:disable-next-line:dot-imports
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"

	ironicv1 "github.com/openstack-k8s-operators/ironic-operator/api/v1beta1"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
)

var _ = Describe("IronicNeutronAgent controller", func() {
	When("IronicNeutronAgent is created", func() {
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
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(ironicNames.Namespace))
			DeferCleanup(th.DeleteInstance, CreateIronicNeutronAgent(ironicNames.INAName, GetDefaultIronicNeutronAgentSpec()))
		})
		It("initializes Status fields", func() {
			instance := GetIronicNeutronAgent(ironicNames.INAName)
			Expect(instance.Status.Hash).To(BeEmpty())
			Expect(instance.Status.ReadyCount).To(Equal(int32(0)))
			Expect(instance.Status.TransportURLSecret).To(BeEmpty())
		})
		It("creates Transport URL and sets TransportURLSecret status field", func() {
			infra.GetTransportURL(ironicNames.INATransportURLName)
			infra.SimulateTransportURLReady(ironicNames.INATransportURLName)
			th.ExpectCondition(
				ironicNames.INAName,
				ConditionGetterFunc(INAConditionGetter),
				condition.RabbitMqTransportURLReadyCondition,
				corev1.ConditionTrue,
			)
			instance := GetIronicNeutronAgent(ironicNames.INAName)
			Expect(instance.Status.TransportURLSecret).To(Equal("rabbitmq-secret"))
		})
		It("Creates ConfigMaps and gets Secrets (input)", func() {
			infra.GetTransportURL(ironicNames.INATransportURLName)
			infra.SimulateTransportURLReady(ironicNames.INATransportURLName)
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
			infra.GetTransportURL(ironicNames.INATransportURLName)
			infra.SimulateTransportURLReady(ironicNames.INATransportURLName)
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
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateMessageBusSecret(ironicNames.Namespace, MessageBusSecretName),
			)
			infra.GetTransportURL(ironicNames.INATransportURLName)
			infra.SimulateTransportURLReady(ironicNames.INATransportURLName)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(ironicNames.Namespace))
		})
		It("is missing secret", func() {
			th.ExpectConditionWithDetails(
				ironicNames.INAName,
				ConditionGetterFunc(INAConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
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
				infra.GetTransportURL(ironicNames.INATransportURLName)
				infra.SimulateTransportURLReady(ironicNames.INATransportURLName)
			})
			It("is missing secret", func() {
				th.ExpectConditionWithDetails(
					ironicNames.INAName,
					ConditionGetterFunc(INAConditionGetter),
					condition.InputReadyCondition,
					corev1.ConditionFalse,
					condition.ErrorReason,
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

	When("IronicNeutronAgent is created with TLS cert secrets", func() {
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
			spec := GetDefaultIronicNeutronAgentSpec()
			spec["tls"] = map[string]any{
				"caBundleSecretName": ironicNames.CaBundleSecretName.Name,
			}
			DeferCleanup(th.DeleteInstance, CreateIronicNeutronAgent(ironicNames.INAName, spec))
			infra.GetTransportURL(ironicNames.INATransportURLName)
			infra.SimulateTransportURLReady(ironicNames.INATransportURLName)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(ironicNames.Namespace))
		})

		It("reports that the CA secret is missing", func() {
			th.ExpectConditionWithDetails(
				ironicNames.INAName,
				ConditionGetterFunc(INAConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				fmt.Sprintf("TLSInput is missing: %s", ironicNames.CaBundleSecretName.Name),
			)
			th.ExpectCondition(
				ironicNames.INAName,
				ConditionGetterFunc(INAConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("creates a Deployment for ironic-neutronagent service with TLS CA cert attached", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(ironicNames.CaBundleSecretName))
			th.SimulateDeploymentReplicaReady(ironicNames.INAName)

			th.ExpectCondition(
				ironicNames.INAName,
				ConditionGetterFunc(INAConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionTrue,
			)

			depl := th.GetDeployment(ironicNames.INAName)
			// Check the resulting deployment fields
			Expect(int(*depl.Spec.Replicas)).To(Equal(1))
			Expect(depl.Spec.Template.Spec.Volumes).To(HaveLen(2))
			Expect(depl.Spec.Template.Spec.Containers).To(HaveLen(1))

			// cert deployment volumes
			th.AssertVolumeExists(ironicNames.CaBundleSecretName.Name, depl.Spec.Template.Spec.Volumes)

			// cert volumeMounts
			container := depl.Spec.Template.Spec.Containers[0]
			th.AssertVolumeMountExists(ironicNames.CaBundleSecretName.Name, "tls-ca-bundle.pem", container.VolumeMounts)
		})

		It("reconfigures the deployment when CA changes", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(ironicNames.CaBundleSecretName))
			th.SimulateDeploymentReplicaReady(ironicNames.INAName)

			th.ExpectCondition(
				ironicNames.INAName,
				ConditionGetterFunc(INAConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionTrue,
			)

			depl := th.GetDeployment(ironicNames.INAName)
			// Check the resulting deployment fields
			Expect(int(*depl.Spec.Replicas)).To(Equal(1))
			Expect(depl.Spec.Template.Spec.Volumes).To(HaveLen(2))
			Expect(depl.Spec.Template.Spec.Containers).To(HaveLen(1))

			// Grab the current config hash
			originalHash := GetEnvVarValue(
				depl.Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")
			Expect(originalHash).NotTo(BeEmpty())

			// Change the content of the CA secret
			th.UpdateSecret(ironicNames.CaBundleSecretName, "tls-ca-bundle.pem", []byte("DifferentCAData"))

			// Assert that the deployment is updated
			Eventually(func(g Gomega) {
				newHash := GetEnvVarValue(
					th.GetDeployment(ironicNames.INAName).Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")
				g.Expect(newHash).NotTo(BeEmpty())
				g.Expect(newHash).NotTo(Equal(originalHash))
			}, timeout, interval).Should(Succeed())
		})
	})

	When("IronicNeutronAgent is created with quorum queue enabled transport URL", func() {

		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateIronicSecret(ironicNames.Namespace, SecretName),
			)

			DeferCleanup(
				k8sClient.Delete,
				ctx,
				infra.CreateTransportURLSecret(ironicNames.Namespace, MessageBusSecretName, true),
			)

			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(ironicNames.Namespace))
			DeferCleanup(th.DeleteInstance, CreateIronicNeutronAgent(ironicNames.INAName, GetDefaultIronicNeutronAgentSpec()))

			infra.GetTransportURL(ironicNames.INATransportURLName)
			infra.SimulateTransportURLReady(ironicNames.INATransportURLName)
			th.SimulateDeploymentReplicaReady(ironicNames.INAName)
		})
		It("generates config with oslo_messaging_rabbit section when quorum queues enabled", func() {

			Eventually(func(g Gomega) {
				confSecret := th.GetSecret(ironicNames.INAConfigSecretName)
				g.Expect(confSecret).ShouldNot(BeNil())

				conf := confSecret.Data["01-ironic_neutron_agent.conf"]
				g.Expect(string(conf)).Should(
					ContainSubstring("[oslo_messaging_rabbit]"))
			}, timeout, interval).Should(Succeed())
		})
	})

	When("IronicNeutronAgent mirrors parent RBAC conditions", func() {

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

			spec := GetDefaultIronicNeutronAgentSpec()
			neutronAgent := CreateIronicNeutronAgent(ironicNames.INAName, spec)
			DeferCleanup(th.DeleteInstance, neutronAgent)

			th.ExpectCondition(
				ironicNames.INAName,
				ConditionGetterFunc(INAConditionGetter),
				condition.ServiceAccountReadyCondition,
				corev1.ConditionTrue,
			)

			// set neutron agent object is owned by parent ironic object
			Eventually(func(g Gomega) {
				parent := GetIronic(ironicNames.IronicName)
				neutronAgentObj := GetIronicNeutronAgent(ironicNames.INAName)
				neutronAgentObj.SetOwnerReferences([]metav1.OwnerReference{{
					APIVersion:         "ironic.openstack.org/v1beta1",
					Kind:               "Ironic",
					Name:               parent.Name,
					UID:                parent.UID,
					Controller:         ptr.To(true),
					BlockOwnerDeletion: ptr.To(true),
				}})
				g.Expect(k8sClient.Update(ctx, neutronAgentObj)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

			// checks that ironic neutron agent mirrors parent conditions
			Eventually(func(g Gomega) {
				neutronAgentObj := GetIronicNeutronAgent(ironicNames.INAName)
				serviceAccountCondition := neutronAgentObj.Status.Conditions.Get(condition.ServiceAccountReadyCondition)
				g.Expect(serviceAccountCondition).ToNot(BeNil())
				g.Expect(serviceAccountCondition.Status).To(Equal(corev1.ConditionTrue))
				g.Expect(serviceAccountCondition.Message).To(Equal("ServiceAccount created"))
			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				ironicNames.INAName,
				ConditionGetterFunc(INAConditionGetter),
				condition.RoleReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				ironicNames.INAName,
				ConditionGetterFunc(INAConditionGetter),
				condition.RoleBindingReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})

	When("An ApplicationCredential is created for IronicNeutronAgent", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete,
				ctx,
				CreateIronicSecret(ironicNames.Namespace, SecretName),
			)

			DeferCleanup(
				k8sClient.Delete,
				ctx,
				infra.CreateTransportURLSecret(ironicNames.Namespace, MessageBusSecretName, false),
			)

			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(ironicNames.Namespace))

			acSecretName := "ac-ironic-secret"
			acSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ironicNames.Namespace,
					Name:      acSecretName,
				},
				Data: map[string][]byte{
					keystonev1.ACIDSecretKey:     []byte("test-ac-id"),
					keystonev1.ACSecretSecretKey: []byte("test-ac-secret"),
				},
			}
			DeferCleanup(k8sClient.Delete, ctx, acSecret)
			Expect(k8sClient.Create(ctx, acSecret)).To(Succeed())

			spec := GetDefaultIronicNeutronAgentSpec()
			spec["auth"] = map[string]any{
				"applicationCredentialSecret": acSecretName,
			}
			DeferCleanup(th.DeleteInstance, CreateIronicNeutronAgent(ironicNames.INAName, spec))

			infra.GetTransportURL(ironicNames.INATransportURLName)
			infra.SimulateTransportURLReady(ironicNames.INATransportURLName)
			th.SimulateDeploymentReplicaReady(ironicNames.INAName)
		})

		It("should render ApplicationCredential auth in IronicNeutronAgent config", func() {
			Eventually(func(g Gomega) {
				cfgSecret := th.GetSecret(ironicNames.INAConfigSecretName)
				g.Expect(cfgSecret).NotTo(BeNil())

				conf := string(cfgSecret.Data["01-ironic_neutron_agent.conf"])

				// AC auth is configured
				g.Expect(conf).To(ContainSubstring("auth_type=v3applicationcredential"))
				g.Expect(conf).To(ContainSubstring("application_credential_id = test-ac-id"))
				g.Expect(conf).To(ContainSubstring("application_credential_secret = test-ac-secret"))

				// Password auth fields should not be present
				g.Expect(conf).NotTo(ContainSubstring("auth_type=password"))
				g.Expect(conf).NotTo(ContainSubstring("username=ironic"))
				g.Expect(conf).NotTo(ContainSubstring("project_name=service"))
			}, timeout, interval).Should(Succeed())
		})
	})
})
