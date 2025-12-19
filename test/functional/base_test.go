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

	. "github.com/onsi/gomega" //revive:disable:dot-imports

	ironicv1 "github.com/openstack-k8s-operators/ironic-operator/api/v1beta1"
	ironic_pkg "github.com/openstack-k8s-operators/ironic-operator/internal/ironic"
	ironic_inspector_pkg "github.com/openstack-k8s-operators/ironic-operator/internal/ironicinspector"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
	INAConfigSecretName       types.NamespacedName
	KeystoneServiceName       types.NamespacedName
	InternalCertSecretName    types.NamespacedName
	PublicCertSecretName      types.NamespacedName
	CaBundleSecretName        types.NamespacedName
	IronicTopologies          []types.NamespacedName
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
		INAConfigSecretName: types.NamespacedName{
			Namespace: ironicNeutronAgent.Namespace,
			Name:      ironicNeutronAgent.Name + "-config-data",
		},
		InternalCertSecretName: types.NamespacedName{
			Namespace: ironicName.Namespace,
			Name:      "internal-tls-certs",
		},
		PublicCertSecretName: types.NamespacedName{
			Namespace: ironicName.Namespace,
			Name:      "public-tls-certs",
		},
		CaBundleSecretName: types.NamespacedName{
			Namespace: ironicName.Namespace,
			Name:      "combined-ca-bundle",
		},
		// A set of topologies to Test how the reference is propagated to the
		// resulting StatefulSets and if a potential override produces the
		// expected values
		IronicTopologies: []types.NamespacedName{
			{
				Namespace: ironicName.Namespace,
				Name:      fmt.Sprintf("%s-global-topology", ironicName.Name),
			},
			{
				Namespace: ironicName.Namespace,
				Name:      fmt.Sprintf("%s-api-topology", ironicName.Name),
			},
			{
				Namespace: ironicName.Namespace,
				Name:      fmt.Sprintf("%s-conductor-topology", ironicName.Name),
			},
			{
				Namespace: ironicName.Namespace,
				Name:      fmt.Sprintf("%s-inspector-topology", ironicName.Name),
			},
			{
				Namespace: ironicName.Namespace,
				Name:      fmt.Sprintf("%s-nagent-topology", ironicName.Name),
			},
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

func CreateMessageBusSecret(
	namespace string,
	name string,
) *corev1.Secret {
	s := th.CreateSecret(
		types.NamespacedName{Namespace: namespace, Name: name},
		map[string][]byte{
			"transport_url": fmt.Appendf(nil, "rabbit://%s/fake", name),
		},
	)
	logger.Info("Secret created", "name", name)
	return s
}

func CreateTransportURL(name types.NamespacedName, rabbitmqClusterName string, username string, vhost string) client.Object {
	spec := map[string]any{
		"rabbitmqClusterName": rabbitmqClusterName,
	}
	if username != "" {
		spec["username"] = username
	}
	// Always set vhost - empty string means default "/" vhost
	spec["vhost"] = vhost

	raw := map[string]any{
		"apiVersion": "rabbitmq.openstack.org/v1beta1",
		"kind":       "TransportURL",
		"metadata": map[string]any{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return CreateUnstructured(raw)
}

func CreateTransportURLSecret(name types.NamespacedName) *corev1.Secret {
	s := th.CreateSecret(
		name,
		map[string][]byte{
			"transport_url": fmt.Appendf(nil, "rabbit://%s/fake", name.Name),
		},
	)
	logger.Info("TransportURL secret created", "name", name.Name)
	return s
}

func CreateIronic(
	name types.NamespacedName,
	spec map[string]any,
) client.Object {
	raw := map[string]any{
		"apiVersion": "ironic.openstack.org/v1beta1",
		"kind":       "Ironic",
		"metadata": map[string]any{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return CreateUnstructured(raw)
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

func GetDefaultIronicSpec() map[string]any {
	return map[string]any{
		"databaseInstance":   DatabaseInstance,
		"secret":             SecretName,
		"ironicAPI":          GetDefaultIronicAPISpec(),
		"ironicConductors":   []map[string]any{GetDefaultIronicConductorSpec()},
		"ironicInspector":    GetDefaultIronicInspectorSpec(),
		"ironicNeutronAgent": GetDefaultIronicNeutronAgentSpec(),
		"images": map[string]any{
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
	spec map[string]any,
) client.Object {
	raw := map[string]any{
		"apiVersion": "ironic.openstack.org/v1beta1",
		"kind":       "IronicAPI",
		"metadata": map[string]any{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return CreateUnstructured(raw)
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

func GetIronicAPISpec(
	name types.NamespacedName,
) ironicv1.IronicAPITemplate {
	instance := &ironicv1.IronicAPI{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance.Spec.IronicAPITemplate
}

func IronicAPIConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetIronicAPI(name)
	return instance.Status.Conditions
}

func GetDefaultIronicAPISpec() map[string]any {
	return map[string]any{
		"secret":           SecretName,
		"databaseHostname": DatabaseHostname,
		"containerImage":   ContainerImage,
		"serviceAccount":   "ironic",
	}
}

func CreateIronicConductor(
	name types.NamespacedName,
	spec map[string]any,
) client.Object {
	raw := map[string]any{
		"apiVersion": "ironic.openstack.org/v1beta1",
		"kind":       "IronicConductor",
		"metadata": map[string]any{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return CreateUnstructured(raw)
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

func GetIronicConductorSpec(
	name types.NamespacedName,
) ironicv1.IronicConductorTemplate {
	instance := &ironicv1.IronicConductor{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance.Spec.IronicConductorTemplate
}

func IronicConductorConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetIronicConductor(name)
	return instance.Status.Conditions
}

func GetDefaultIronicConductorSpec() map[string]any {
	return map[string]any{
		"databaseHostname":              DatabaseHostname,
		"databaseInstance":              DatabaseInstance,
		"secret":                        SecretName,
		"containerImage":                ContainerImage,
		"pxeContainerImage":             PxeContainerImage,
		"ironicPythonAgentImage":        IronicPythonAgentImage,
		"serviceAccount":                "ironic",
		"storageRequest":                "10G",
		"terminationGracePeriodSeconds": 120,
	}
}

func CreateIronicInspector(
	name types.NamespacedName,
	spec map[string]any,
) client.Object {
	raw := map[string]any{
		"apiVersion": "ironic.openstack.org/v1beta1",
		"kind":       "IronicInspector",
		"metadata": map[string]any{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return CreateUnstructured(raw)
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

func GetIronicInspectorSpec(
	name types.NamespacedName,
) ironicv1.IronicInspectorTemplate {
	instance := &ironicv1.IronicInspector{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance.Spec.IronicInspectorTemplate
}

func GetDefaultIronicInspectorSpec() map[string]any {
	return map[string]any{
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
	spec map[string]any,
) client.Object {
	raw := map[string]any{
		"apiVersion": "ironic.openstack.org/v1beta1",
		"kind":       "IronicNeutronAgent",
		"metadata": map[string]any{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return CreateUnstructured(raw)
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

func GetDefaultIronicNeutronAgentSpec() map[string]any {
	return map[string]any{
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
	fakeCustomResorce := map[string]any{
		"apiVersion": "apiextensions.k8s.io/v1",
		"kind":       "CustomResourceDefinition",
		"metadata": map[string]any{
			"name": "ingresscontrollers.operator.openshift.io",
		},
		"spec": map[string]any{
			"group": "operator.openshift.io",
			"names": map[string]any{
				"kind":     "IngressController",
				"listKind": "IngressControllerList",
				"plural":   "ingresscontrollers",
				"singular": "ingresscontroller",
			},
			"scope": "Namespaced",
			"versions": []map[string]any{{
				"name":    "v1",
				"served":  true,
				"storage": true,
				"schema": map[string]any{
					"openAPIV3Schema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"status": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"domain": map[string]any{
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
	fakeIngressController := map[string]any{
		"apiVersion": "operator.openshift.io/v1",
		"kind":       "IngressController",
		"metadata": map[string]any{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"status": map[string]any{
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
	CreateUnstructured(fakeCustomResorce)
	Eventually(func(g Gomega) {
		g.Expect(th.K8sClient.Get(th.Ctx, client.ObjectKey{
			Name: "ingresscontrollers.operator.openshift.io",
		}, crd)).Should(Succeed())
	}, th.Timeout, th.Interval).Should(Succeed())
	th.CreateNamespace(name.Namespace)

	fic := CreateUnstructured(fakeIngressController)

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

func CreateUnstructured(rawObj map[string]any) *unstructured.Unstructured {
	logger.Info("Creating", "raw", rawObj)
	unstructuredObj := &unstructured.Unstructured{Object: rawObj}
	Eventually(func(g Gomega) {
		_, err := controllerutil.CreateOrPatch(ctx, k8sClient, unstructuredObj, func() error { return nil })
		g.Expect(err).ShouldNot(HaveOccurred())
	}, th.Timeout, th.Interval).Should(Succeed())
	return unstructuredObj
}

// GetSampleTopologySpec - An opinionated Topology Spec sample used to
// test Service components. It returns both the user input representation
// in the form of map[string]string, and the Golang expected representation
// used in the test asserts.
func GetSampleTopologySpec(label string) (map[string]any, []corev1.TopologySpreadConstraint) {
	// Build the topology Spec
	topologySpec := map[string]any{
		"topologySpreadConstraints": []map[string]any{
			{
				"maxSkew":           1,
				"topologyKey":       corev1.LabelHostname,
				"whenUnsatisfiable": "ScheduleAnyway",
				"labelSelector": map[string]any{
					"matchLabels": map[string]any{
						"component": label,
					},
				},
			},
		},
	}
	// Build the topologyObj representation
	topologySpecObj := []corev1.TopologySpreadConstraint{
		{
			MaxSkew:           1,
			TopologyKey:       corev1.LabelHostname,
			WhenUnsatisfiable: corev1.ScheduleAnyway,
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"component": label,
				},
			},
		},
	}
	return topologySpec, topologySpecObj
}
