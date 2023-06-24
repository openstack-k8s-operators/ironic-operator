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
	"time"

	. "github.com/onsi/gomega"

	ironic_pkg "github.com/openstack-k8s-operators/ironic-operator/pkg/ironic"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ironicv1 "github.com/openstack-k8s-operators/ironic-operator/api/v1beta1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
)

const (
	timeout              = 10 * time.Second
	interval             = 10 * time.Millisecond
	SecretName           = "test-secret"
	MessageBusSecretName = "rabbitmq-secret"
	ContainerImage       = "test://ironic"
)

type IronicNames struct {
	Namespace                 string
	IronicName                types.NamespacedName
	ServiceAccountName        types.NamespacedName
	APIName                   types.NamespacedName
	ConductorName             types.NamespacedName
	InspectorName             types.NamespacedName
	InspectorTransportURLName types.NamespacedName
	InspectorServiceAccount   types.NamespacedName
	InspectorRole             types.NamespacedName
	InspectorRoleBinding      types.NamespacedName
	InspectorDatabaseName     types.NamespacedName
	InspectorDBSyncJobName    types.NamespacedName
	INAName                   types.NamespacedName
	INATransportURLName       types.NamespacedName
	KeystoneServiceName       types.NamespacedName
}

func GetIronicNames(
	ironicName types.NamespacedName,
) IronicNames {
	ironic := types.NamespacedName{
		Namespace: ironicName.Namespace,
		Name:      "ironic",
	}
	ironicAPI := types.NamespacedName{
		Namespace: ironicName.Namespace,
		Name:      "ironic-api",
	}
	ironicConductor := types.NamespacedName{
		Namespace: ironicName.Namespace,
		Name:      "ironic-conductor",
	}
	ironicInspector := types.NamespacedName{
		Namespace: ironicName.Namespace,
		Name:      "ironic-inspector",
	}
	ironicNeutronAgent := types.NamespacedName{
		Namespace: ironicName.Namespace,
		Name:      "ironic-neutron-agent",
	}

	return IronicNames{
		Namespace: ironicName.Namespace,
		IronicName: types.NamespacedName{
			Namespace: ironic.Namespace,
			Name:      ironic.Name,
		},
		APIName: types.NamespacedName{
			Namespace: ironicAPI.Namespace,
			Name:      ironicAPI.Name,
		},
		ConductorName: types.NamespacedName{
			Namespace: ironicConductor.Namespace,
			Name:      ironicConductor.Name,
		},
		InspectorName: types.NamespacedName{
			Namespace: ironicInspector.Namespace,
			Name:      ironicInspector.Name,
		},
		InspectorTransportURLName: types.NamespacedName{
			Namespace: ironicInspector.Namespace,
			Name:      ironicInspector.Name + "-transport",
		},
		InspectorServiceAccount: types.NamespacedName{
			Namespace: ironicInspector.Namespace,
			Name:      "ironicinspector-" + ironicInspector.Name,
		},
		InspectorRole: types.NamespacedName{
			Namespace: ironicInspector.Namespace,
			Name:      "ironicinspector-" + ironicInspector.Name + "-role",
		},
		InspectorRoleBinding: types.NamespacedName{
			Namespace: ironicInspector.Namespace,
			Name:      "ironicinspector-" + ironicInspector.Name + "-rolebinding",
		},
		InspectorDatabaseName: types.NamespacedName{
			Namespace: ironicInspector.Namespace,
			Name:      ironicInspector.Name,
		},
		InspectorDBSyncJobName: types.NamespacedName{
			Namespace: ironicInspector.Namespace,
			Name:      ironic_pkg.ServiceName + "-" + ironic_pkg.InspectorComponent + "-db-sync",
		},
		INAName: types.NamespacedName{
			Namespace: ironicNeutronAgent.Namespace,
			Name:      ironicNeutronAgent.Name,
		},
		INATransportURLName: types.NamespacedName{
			Namespace: ironicNeutronAgent.Namespace,
			Name:      ironicNeutronAgent.Name + "-transport",
		},
	}
}

func CreateIronicSecret(namespace string, name string) *corev1.Secret {
	return th.CreateSecret(
		types.NamespacedName{Namespace: namespace, Name: name},
		map[string][]byte{
			"IronicPassword":                  []byte("12345678"),
			"IronicInspectorPassword":         []byte("12345678"),
			"IronicDatabasePassword":          []byte("12345678"),
			"IronicInspectorDatabasePassword": []byte("12345678"),
		},
	)
}

// func CreateMessageBusSecret(
// 	namespace string,
// 	name string,
// ) *corev1.Secret {
// 	s := th.CreateSecret(
// 		types.NamespacedName{Namespace: namespace, Name: name},
// 		map[string][]byte{
// 			"transport_url": []byte(fmt.Sprintf("rabbit://%s/fake", name)),
// 		},
// 	)
// 	logger.Info("Secret created", "name", name)
// 	return s
// }

func CreateIronic(
	name types.NamespacedName,
	spec map[string]interface{},
) client.Object {
	raw := map[string]interface{}{
		"apiVersion": "ironic.openstack.org/v1beta1",
		"kind":       "Ironic",
		"metadata": map[string]interface{}{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return th.CreateUnstructured(raw)
}

func GetIronic(
	name types.NamespacedName,
) *ironicv1.Ironic {
	instance := &ironicv1.Ironic{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func CreateIronicAPI(
	name types.NamespacedName,
	spec map[string]interface{},
) client.Object {
	raw := map[string]interface{}{
		"apiVersion": "ironic.openstack.org/v1beta1",
		"kind":       "IronicAPI",
		"metadata": map[string]interface{}{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return th.CreateUnstructured(raw)
}

func GetIronicAPI(
	name types.NamespacedName,
) *ironicv1.IronicAPI {
	instance := &ironicv1.IronicAPI{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func CreateIronicConductor(
	name types.NamespacedName,
	spec map[string]interface{},
) client.Object {
	raw := map[string]interface{}{
		"apiVersion": "ironic.openstack.org/v1beta1",
		"kind":       "IronicConductor",
		"metadata": map[string]interface{}{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return th.CreateUnstructured(raw)
}

func GetIronicConductor(
	name types.NamespacedName,
) *ironicv1.IronicConductor {
	instance := &ironicv1.IronicConductor{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func CreateIronicInspector(
	name types.NamespacedName,
	spec map[string]interface{},
) client.Object {
	raw := map[string]interface{}{
		"apiVersion": "ironic.openstack.org/v1beta1",
		"kind":       "IronicInspector",
		"metadata": map[string]interface{}{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return th.CreateUnstructured(raw)
}

func GetIronicInspector(
	name types.NamespacedName,
) *ironicv1.IronicInspector {
	instance := &ironicv1.IronicInspector{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func GetDefaultIronicInspectorSpec() map[string]interface{} {
	return map[string]interface{}{
		"databaseInstance": "openstack",
		"secret":           SecretName,
		"containerImage":   ContainerImage,
		"serviceAccount":   "ironic",
	}
}

func IronicInspectorConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetIronicInspector(name)
	return instance.Status.Conditions
}

func CreateIronicNeutronAgent(
	name types.NamespacedName,
	spec map[string]interface{},
) client.Object {
	raw := map[string]interface{}{
		"apiVersion": "ironic.openstack.org/v1beta1",
		"kind":       "IronicNeutronAgent",
		"metadata": map[string]interface{}{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return th.CreateUnstructured(raw)
}

func GetIronicNeutronAgent(
	name types.NamespacedName,
) *ironicv1.IronicNeutronAgent {
	instance := &ironicv1.IronicNeutronAgent{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func GetDefaultIronicNeutronAgentSpec() map[string]interface{} {
	return map[string]interface{}{
		"secret":         SecretName,
		"containerImage": ContainerImage,
		"serviceAccount": "ironic",
	}
}

func INAConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetIronicNeutronAgent(name)
	return instance.Status.Conditions
}

// func GetEnvValue(envs []corev1.EnvVar, name string, defaultValue string) string {
// 	for _, e := range envs {
// 		if e.Name == name {
// 			return e.Value
// 		}
// 	}
// 	return defaultValue
// }

func CreateFakeIngressController() {
	// Namespace and Name for fake "default" ingresscontroller
	name := types.NamespacedName{
		Namespace: "openshift-ingress-operator",
		Name:      "default",
	}

	// Fake IngressController custom resource
	fakeCustomResorce := map[string]interface{}{
		"apiVersion": "apiextensions.k8s.io/v1",
		"kind":       "CustomResourceDefinition",
		"metadata": map[string]interface{}{
			"name": "ingresscontrollers.operator.openshift.io",
		},
		"spec": map[string]interface{}{
			"group": "operator.openshift.io",
			"names": map[string]interface{}{
				"kind":     "IngressController",
				"listKind": "IngressControllerList",
				"plural":   "ingresscontrollers",
				"singular": "ingresscontroller",
			},
			"scope": "Namespaced",
			"versions": []map[string]interface{}{{
				"name":    "v1",
				"served":  true,
				"storage": true,
				"schema": map[string]interface{}{
					"openAPIV3Schema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"status": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"domain": map[string]interface{}{
										"type": "string",
									},
								},
							},
						},
					},
				},
			}},
		},
	}

	// Fake ingresscontroller
	fakeIngressController := map[string]interface{}{
		"apiVersion": "operator.openshift.io/v1",
		"kind":       "IngressController",
		"metadata": map[string]interface{}{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"status": map[string]interface{}{
			"domain": "test.example.com",
		},
	}

	// Create fake custom resource, namespace and fake ingresscontroller
	th.CreateUnstructured(fakeCustomResorce)
	th.CreateNamespace(name.Namespace)
	th.CreateUnstructured(fakeIngressController)
}
