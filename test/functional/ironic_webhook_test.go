/*
Copyright 2025 Red Hat Inc.

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
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports

	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ = Describe("Ironic webhook", func() {
	It("rejects update to deprecated rabbitMqClusterName field", func() {
		spec := GetDefaultIronicSpec()
		spec["rabbitMqClusterName"] = "rabbitmq"

		ironicName := types.NamespacedName{
			Namespace: ironicNames.Namespace,
			Name:      "ironic-webhook-test",
		}

		raw := map[string]any{
			"apiVersion": "ironic.openstack.org/v1beta1",
			"kind":       "Ironic",
			"metadata": map[string]any{
				"name":      ironicName.Name,
				"namespace": ironicName.Namespace,
			},
			"spec": spec,
		}

		// Create the Ironic instance
		unstructuredObj := &unstructured.Unstructured{Object: raw}
		_, err := controllerutil.CreateOrPatch(
			ctx, k8sClient, unstructuredObj, func() error { return nil })
		Expect(err).ShouldNot(HaveOccurred())

		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, unstructuredObj)
		})

		// Verify that messagingBus.cluster was defaulted to "rabbitmq"
		// Note: The webhook does NOT migrate the deprecated field - migration is handled
		// by openstack-operator. The webhook just defaults messagingBus.cluster.
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, ironicName, unstructuredObj)).Should(Succeed())
			specMap := unstructuredObj.Object["spec"].(map[string]any)
			messagingBus, exists := specMap["messagingBus"].(map[string]any)
			g.Expect(exists).To(BeTrue(), "messagingBus should exist")
			g.Expect(messagingBus["cluster"]).To(Equal("rabbitmq"),
				"messagingBus.cluster should be defaulted to 'rabbitmq'")
		}, timeout, interval).Should(Succeed())

		// Try to update rabbitMqClusterName
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, ironicName, unstructuredObj)).Should(Succeed())
			specMap := unstructuredObj.Object["spec"].(map[string]any)
			specMap["rabbitMqClusterName"] = "rabbitmq2"
			err := k8sClient.Update(ctx, unstructuredObj)
			g.Expect(err).Should(HaveOccurred())

			var statusError *k8s_errors.StatusError
			g.Expect(errors.As(err, &statusError)).To(BeTrue())
			g.Expect(statusError.ErrStatus.Details.Kind).To(Equal("Ironic"))
			g.Expect(statusError.ErrStatus.Message).To(
				ContainSubstring("field \"spec.rabbitMqClusterName\" is deprecated"))
			g.Expect(statusError.ErrStatus.Message).To(
				ContainSubstring("use \"spec.messagingBus.cluster\" instead"))
		}, timeout, interval).Should(Succeed())
	})

	Describe("service override validation", func() {
		// createIronicWithOverride creates an Ironic CR with the given
		// component ("ironicAPI"/"ironicInspector") service override for one
		// endpoint and returns the admission result.
		createIronicWithOverride := func(component string, endpt string, override map[string]any) error {
			spec := GetDefaultIronicSpec()
			spec[component].(map[string]any)["override"] = map[string]any{
				"service": map[string]any{endpt: override},
			}
			obj := &unstructured.Unstructured{Object: map[string]any{
				"apiVersion": "ironic.openstack.org/v1beta1",
				"kind":       "Ironic",
				"metadata": map[string]any{
					"name":      "ironic-override-test",
					"namespace": ironicNames.Namespace,
				},
				"spec": spec,
			}}
			// Dry run exercises the webhook without persisting (and thereby
			// reconciling) the CR.
			return k8sClient.Create(ctx, obj, client.DryRunAll)
		}

		ports := func(name string, port int) map[string]any {
			return map[string]any{"ports": []any{map[string]any{"name": name, "port": port}}}
		}

		for _, comp := range []struct {
			field string
			port  int
		}{{"ironicAPI", 6385}, {"ironicInspector", 5050}} {
			for _, endpt := range []string{"public", "internal"} {
				It("accepts the bare "+endpt+" port name for "+comp.field, func() {
					Expect(createIronicWithOverride(comp.field, endpt,
						map[string]any{"spec": ports(endpt, comp.port)})).To(Succeed())
				})

				It("rejects a wrong "+endpt+" port name for "+comp.field, func() {
					err := createIronicWithOverride(comp.field, endpt,
						map[string]any{"spec": ports("wrong", comp.port)})
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring(
						"spec." + comp.field + ".override.service[" + endpt + "].spec.ports[0].name: " +
							"Invalid value: \"wrong\": must be \"" + endpt + "\""))
				})

				It("accepts an FQDN "+endpt+" endpointURL for "+comp.field, func() {
					Expect(createIronicWithOverride(comp.field, endpt, map[string]any{
						"endpointURL": fmt.Sprintf("https://ironic.example.com:%d", comp.port),
					})).To(Succeed())
				})

				for name, url := range map[string]string{
					"a bare short name": fmt.Sprintf("https://ironic:%d", comp.port),
					"a missing scheme":  fmt.Sprintf("ironic.example.com:%d", comp.port),
					"an IP literal":     fmt.Sprintf("https://192.168.1.10:%d", comp.port),
				} {
					It("rejects "+name+" as "+endpt+" endpointURL for "+comp.field, func() {
						err := createIronicWithOverride(comp.field, endpt, map[string]any{"endpointURL": url})
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring(
							"spec." + comp.field + ".override.service[" + endpt + "].endpointURL"))
					})
				}

				It("rejects a "+endpt+" ports[].port that differs from the endpointURL port for "+comp.field, func() {
					err := createIronicWithOverride(comp.field, endpt, map[string]any{
						"endpointURL": "https://ironic.example.com:8443",
						"spec":        ports(endpt, comp.port),
					})
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("must match the port in endpointURL"))
				})

				It("accepts a "+endpt+" ports[].port matching the endpointURL port for "+comp.field, func() {
					Expect(createIronicWithOverride(comp.field, endpt, map[string]any{
						"endpointURL": "https://ironic.example.com:8443",
						"spec":        ports(endpt, 8443),
					})).To(Succeed())
				})
			}
		}
	})
})
