/*
Copyright 2022.

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
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/types"
)

const SamplesDir = "../../config/samples/"

func ReadSample(sampleFileName string) map[string]interface{} {
	rawSample := make(map[string]interface{})

	bytes, err := os.ReadFile(filepath.Join(SamplesDir, sampleFileName))
	Expect(err).ShouldNot(HaveOccurred())
	Expect(yaml.Unmarshal(bytes, rawSample)).Should(Succeed())

	return rawSample
}

func CreateIronicFromSample(sampleFileName string, name types.NamespacedName) types.NamespacedName {
	raw := ReadSample(sampleFileName)
	instance := CreateIronic(name, raw["spec"].(map[string]interface{}))
	DeferCleanup(th.DeleteInstance, instance)
	return types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
}

func CreateIronicAPIFromSample(sampleFileName string, name types.NamespacedName) types.NamespacedName {
	raw := ReadSample(sampleFileName)
	instance := CreateIronicAPI(name, raw["spec"].(map[string]interface{}))
	DeferCleanup(th.DeleteInstance, instance)
	return types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
}

func CreateIronicConductorFromSample(sampleFileName string, name types.NamespacedName) types.NamespacedName {
	raw := ReadSample(sampleFileName)
	instance := CreateIronicConductor(name, raw["spec"].(map[string]interface{}))
	DeferCleanup(th.DeleteInstance, instance)
	return types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
}

func CreateIronicInspectorFromSample(sampleFileName string, name types.NamespacedName) types.NamespacedName {
	raw := ReadSample(sampleFileName)
	instance := CreateIronicInspector(name, raw["spec"].(map[string]interface{}))
	DeferCleanup(th.DeleteInstance, instance)
	return types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
}

func CreateIronicNeutronAgentFromSample(sampleFileName string, name types.NamespacedName) types.NamespacedName {
	raw := ReadSample(sampleFileName)
	instance := CreateIronicNeutronAgent(name, raw["spec"].(map[string]interface{}))
	DeferCleanup(th.DeleteInstance, instance)
	return types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
}

// This is a set of test for our samples. It only validates that the sample
// file has all the required field with proper types. But it does not
// validate that using a sample file will result in a working deployment.
// TODO(gibi): By building up all the prerequisites (e.g. MariaDBDatabase) in
// the test and by simulating Job and Deployment success we could assert
// that each sample creates a CR in Ready state.
var _ = Describe("Samples", func() {

	When("ironic_v1beta1_ironicneutronagent.yaml sample is applied", func() {
		It("IronicNeutronAgent is created", func() {
			name := CreateIronicNeutronAgentFromSample(
				"ironic_v1beta1_ironicneutronagent.yaml",
				ironicNames.INAName,
			)
			GetIronicNeutronAgent(name)
		})
	})

	When("ironic_v1beta1_ironicinspector.yaml sample is applied", func() {
		It("IronicInspector is created", func() {
			name := CreateIronicInspectorFromSample(
				"ironic_v1beta1_ironicinspector.yaml",
				ironicNames.InspectorName,
			)
			GetIronicInspector(name)
		})
	})

	When("ironic_v1beta1_ironicconductor.yaml sample is applied", func() {
		It("IronicInspector is created", func() {
			name := CreateIronicConductorFromSample(
				"ironic_v1beta1_ironicconductor.yaml",
				ironicNames.ConductorName,
			)
			GetIronicConductor(name)
		})
	})

	When("ironic_v1beta1_ironicapi.yaml sample is applied", func() {
		It("IronicInspector is created", func() {
			name := CreateIronicAPIFromSample(
				"ironic_v1beta1_ironicapi.yaml",
				ironicNames.ConductorName,
			)
			GetIronicAPI(name)
		})
	})

	// TODO(hjensas): Need to simulate the RBAC ServiceAccount?
	// When("ironic_v1beta1_ironic.yaml sample is applied", func() {
	// 	It("IronicInspector is created", func() {
	// 		name := CreateIronicFromSample(
	// 			"ironic_v1beta1_ironic.yaml",
	// 			ironicNames.ConductorName,
	// 		)
	// 		GetIronic(name)
	// 	})
	// })
})
