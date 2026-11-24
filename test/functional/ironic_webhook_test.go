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

	Describe("ironicNeutronAgent extraMounts validation", func() {
		// createIronicWithExtraMountsVolume creates an Ironic CR whose
		// ironicNeutronAgent extraMounts reference a volume with the given
		// name and returns the admission result.
		createIronicWithExtraMountsVolume := func(volumeName string) error {
			spec := GetDefaultIronicSpec()
			spec["ironicNeutronAgent"].(map[string]any)["extraMounts"] = GetExtraMounts(
				volumeName, "/etc/neutron/"+volumeName)
			obj := &unstructured.Unstructured{Object: map[string]any{
				"apiVersion": "ironic.openstack.org/v1beta1",
				"kind":       "Ironic",
				"metadata": map[string]any{
					"name":      "ironic-extramounts-test",
					"namespace": ironicNames.Namespace,
				},
				"spec": spec,
			}}
			// Dry run exercises the webhook without persisting (and thereby
			// reconciling) the CR.
			return k8sClient.Create(ctx, obj, client.DryRunAll)
		}

		for _, reserved := range []string{
			"config",
			"var-log-neutron",
			"combined-ca-bundle",
			"ovndb-tls-certs",
		} {
			It("rejects an extraMounts volume named "+reserved, func() {
				err := createIronicWithExtraMountsVolume(reserved)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(
					"spec.ironicNeutronAgent.extraMounts[0].extraVol[0].volumes[0].name"))
				Expect(err.Error()).To(ContainSubstring(
					"is reserved for an operator-owned volume of the ironic-neutron-agent pod"))
				Expect(err.Error()).To(ContainSubstring(reserved))
			})
		}

		It("rejects a duplicate extraMounts volume name", func() {
			spec := GetDefaultIronicSpec()
			spec["ironicNeutronAgent"].(map[string]any)["extraMounts"] = append(
				GetExtraMounts("dup-volume", "/etc/neutron/dup1.yaml"),
				GetExtraMounts("dup-volume", "/etc/neutron/dup2.yaml")...)
			obj := &unstructured.Unstructured{Object: map[string]any{
				"apiVersion": "ironic.openstack.org/v1beta1",
				"kind":       "Ironic",
				"metadata": map[string]any{
					"name":      "ironic-extramounts-test",
					"namespace": ironicNames.Namespace,
				},
				"spec": spec,
			}}
			err := k8sClient.Create(ctx, obj, client.DryRunAll)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(
				"spec.ironicNeutronAgent.extraMounts[1].extraVol[0].volumes[0].name: Duplicate value"))
		})

		It("accepts an extraMounts volume with a non-reserved name", func() {
			Expect(createIronicWithExtraMountsVolume("l2vni-network-nodes")).To(Succeed())
		})
	})
})
