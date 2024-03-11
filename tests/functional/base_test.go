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
	"time"

	. "github.com/onsi/gomega"

	ironic_pkg "github.com/openstack-k8s-operators/ironic-operator/pkg/ironic"
	ironic_inspector_pkg "github.com/openstack-k8s-operators/ironic-operator/pkg/ironicinspector"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ironicv1 "github.com/openstack-k8s-operators/ironic-operator/api/v1beta1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
)

const (
	timeout                = 20 * time.Second
	interval               = 20 * time.Millisecond
	DatabaseHostname       = "databasehost.example.org"
	DatabaseInstance       = "openstack"
	SecretName             = "test-secret"
	MessageBusSecretName   = "rabbitmq-secret"
	ContainerImage         = "test://ironic"
	PxeContainerImage      = "test://pxe-image"
	IronicPythonAgentImage = "test://ipa-image"
)

type IronicNames struct {
	Namespace                 string
	IronicName                types.NamespacedName
	IronicConfigSecretName    types.NamespacedName
	IronicRole                types.NamespacedName
	IronicRoleBinding         types.NamespacedName
	IronicServiceAccount      types.NamespacedName
	IronicTransportURLName    types.NamespacedName
	IronicDatabaseName        types.NamespacedName
	IronicDatabaseAccount     types.NamespacedName
	IronicDBSyncJobName       types.NamespacedName
	ServiceAccountName        types.NamespacedName
	APIName                   types.NamespacedName
	APIServiceAccount         types.NamespacedName
	APIRole                   types.NamespacedName
	APIRoleBinding            types.NamespacedName
	APIConfigSecretName       types.NamespacedName
	ConductorName             types.NamespacedName
	ConductorConfigSecretName types.NamespacedName
	ConductorServiceAccount   types.NamespacedName
	ConductorRole             types.NamespacedName
	ConductorRoleBinding      types.NamespacedName
	InspectorName             types.NamespacedName
	InspectorTransportURLName types.NamespacedName
	InspectorServiceAccount   types.NamespacedName
	InspectorRole             types.NamespacedName
	InspectorRoleBinding      types.NamespacedName
	InspectorDatabaseName     types.NamespacedName
	InspectorDatabaseAccount  types.NamespacedName
	InspectorDBSyncJobName    types.NamespacedName
	InspectorConfigSecretName types.NamespacedName
	INAName                   types.NamespacedName
	INATransportURLName       types.NamespacedName
	KeystoneServiceName       types.NamespacedName
	InternalCertSecretName    types.NamespacedName
	PublicCertSecretName      types.NamespacedName
	CaBundleSecretName        types.NamespacedName
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
		IronicConfigSecretName: types.NamespacedName{
			Namespace: ironic.Namespace,
			Name:      ironic.Name + "-config-data",
		},
		IronicTransportURLName: types.NamespacedName{
			Namespace: ironic.Namespace,
			Name:      ironic.Name + "-transport",
		},
		IronicDatabaseName: types.NamespacedName{
			Namespace: ironic.Namespace,
			Name:      ironic_pkg.DatabaseCRName,
		},
		IronicDatabaseAccount: types.NamespacedName{
			Namespace: ironic.Namespace,
			Name:      ironic.Name,
		},
		IronicDBSyncJobName: types.NamespacedName{
			Namespace: ironic.Namespace,
			Name:      ironic_pkg.ServiceName + "-db-sync",
		},
		IronicServiceAccount: types.NamespacedName{
			Namespace: ironic.Namespace,
			Name:      "ironic-" + ironic.Name,
		},
		IronicRole: types.NamespacedName{
			Namespace: ironic.Namespace,
			Name:      "ironic-" + ironic.Name + "-role",
		},
		IronicRoleBinding: types.NamespacedName{
			Namespace: ironic.Namespace,
			Name:      "ironic-" + ironic.Name + "-rolebinding",
		},
		APIName: types.NamespacedName{
			Namespace: ironicAPI.Namespace,
			Name:      ironicAPI.Name,
		},
		APIServiceAccount: types.NamespacedName{
			Namespace: ironicAPI.Namespace,
			Name:      "ironicapi-" + ironicAPI.Name,
		},
		APIRole: types.NamespacedName{
			Namespace: ironicAPI.Namespace,
			Name:      "ironicapi-" + ironicAPI.Name + "-role",
		},
		APIRoleBinding: types.NamespacedName{
			Namespace: ironicAPI.Namespace,
			Name:      "ironicapi-" + ironicAPI.Name + "-rolebinding",
		},
		APIConfigSecretName: types.NamespacedName{
			Namespace: ironicAPI.Namespace,
			Name:      "ironic-api-config-data",
		},
		ConductorName: types.NamespacedName{
			Namespace: ironicConductor.Namespace,
			Name:      ironicConductor.Name,
		},
		ConductorConfigSecretName: types.NamespacedName{
			Namespace: ironicAPI.Namespace,
			Name:      "ironic-conductor-config-data",
		},
		ConductorServiceAccount: types.NamespacedName{
			Namespace: ironicConductor.Namespace,
			Name:      "ironicconductor-" + ironicConductor.Name,
		},
		ConductorRole: types.NamespacedName{
			Namespace: ironicConductor.Namespace,
			Name:      "ironicconductor-" + ironicConductor.Name + "-role",
		},
		ConductorRoleBinding: types.NamespacedName{
			Namespace: ironicConductor.Namespace,
			Name:      "ironicconductor-" + ironicConductor.Name + "-rolebinding",
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
			Name:      ironic_inspector_pkg.DatabaseCRName,
		},
		InspectorDatabaseAccount: types.NamespacedName{
			Namespace: ironicInspector.Namespace,
			Name:      ironicInspector.Name,
		},
		InspectorDBSyncJobName: types.NamespacedName{
			Namespace: ironicInspector.Namespace,
			Name:      ironic_pkg.ServiceName + "-" + ironic_pkg.InspectorComponent + "-db-sync",
		},
		InspectorConfigSecretName: types.NamespacedName{
			Namespace: ironicAPI.Namespace,
			Name:      "ironic-inspector-config-data",
		},
		INAName: types.NamespacedName{
			Namespace: ironicNeutronAgent.Namespace,
			Name:      ironicNeutronAgent.Name,
		},
		INATransportURLName: types.NamespacedName{
			Namespace: ironicNeutronAgent.Namespace,
			Name:      ironicNeutronAgent.Name + "-transport",
		},
		InternalCertSecretName: types.NamespacedName{
			Namespace: ironicName.Namespace,
			Name:      "internal-tls-certs"},
		PublicCertSecretName: types.NamespacedName{
			Namespace: ironicName.Namespace,
			Name:      "public-tls-certs"},
		CaBundleSecretName: types.NamespacedName{
			Namespace: ironicName.Namespace,
			Name:      "combined-ca-bundle"},
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

func CreateMessageBusSecret(
	namespace string,
	name string,
) *corev1.Secret {
	s := th.CreateSecret(
		types.NamespacedName{Namespace: namespace, Name: name},
		map[string][]byte{
			"transport_url": []byte(fmt.Sprintf("rabbit://%s/fake", name)),
		},
	)
	logger.Info("Secret created", "name", name)
	return s
}

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

func IronicConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetIronic(name)
	return instance.Status.Conditions
}

func GetDefaultIronicSpec() map[string]interface{} {
	return map[string]interface{}{
		"databaseInstance":   DatabaseInstance,
		"secret":             SecretName,
		"ironicAPI":          GetDefaultIronicAPISpec(),
		"ironicConductors":   []map[string]interface{}{GetDefaultIronicConductorSpec()},
		"ironicInspector":    GetDefaultIronicInspectorSpec(),
		"ironicNeutronAgent": GetDefaultIronicNeutronAgentSpec(),
		"images": map[string]interface{}{
			"api":               ContainerImage,
			"conductor":         ContainerImage,
			"inspector":         ContainerImage,
			"neutronAgent":      ContainerImage,
			"pxe":               ContainerImage,
			"ironicPythonAgent": ContainerImage,
		},
	}
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

func IronicAPIConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetIronicAPI(name)
	return instance.Status.Conditions
}

func GetDefaultIronicAPISpec() map[string]interface{} {
	return map[string]interface{}{
		"secret":           SecretName,
		"databaseHostname": DatabaseHostname,
		"containerImage":   ContainerImage,
		"serviceAccount":   "ironic",
	}
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

func IronicConductorConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetIronicConductor(name)
	return instance.Status.Conditions
}

func GetDefaultIronicConductorSpec() map[string]interface{} {
	return map[string]interface{}{
		"databaseHostname":       DatabaseHostname,
		"databaseInstance":       DatabaseInstance,
		"secret":                 SecretName,
		"containerImage":         ContainerImage,
		"pxeContainerImage":      PxeContainerImage,
		"ironicPythonAgentImage": IronicPythonAgentImage,
		"serviceAccount":         "ironic",
		"storageRequest":         "10G",
	}
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
		"databaseInstance":       DatabaseInstance,
		"secret":                 SecretName,
		"containerImage":         ContainerImage,
		"ironicPythonAgentImage": IronicPythonAgentImage,
		"serviceAccount":         "ironic",
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

	crd := &unstructured.Unstructured{}
	crd.SetGroupVersionKind(
		schema.GroupVersionKind{
			Group:   "apiextensions.k8s.io",
			Version: "v1",
			Kind:    "CustomResourceDefinition",
		},
	)

	// Create fake custom resource, namespace and fake ingresscontroller
	th.CreateUnstructured(fakeCustomResorce)
	Eventually(func(g Gomega) {
		g.Expect(th.K8sClient.Get(th.Ctx, client.ObjectKey{
			Name: "ingresscontrollers.operator.openshift.io",
		}, crd)).Should(Succeed())
	}, th.Timeout, th.Interval).Should(Succeed())
	th.CreateNamespace(name.Namespace)

	fic := th.CreateUnstructured(fakeIngressController)

	// (zzzeek) if we proceed into the k8sManager.Start(ctx) step before
	// the above CreateUnstructured call is done, the above call
	// fails with a 404 error of some kind.  This is based on observing
	// if the CreateFakeIngressController() call is placed after the
	// call to k8sManager.Start(ctx), I get the same error.  On CI
	// (within the make docker-build target that calls the test target) and
	// sometimes locally, I get the same error without changing their order.
	// So ensure this operation is fully complete ahead of time
	Eventually(func(g Gomega) {
		g.Expect(th.K8sClient.Get(th.Ctx, name, fic)).Should(Succeed())
	}, th.Timeout, th.Interval).Should(Succeed())

}
