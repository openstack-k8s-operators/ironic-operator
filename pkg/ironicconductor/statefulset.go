/*

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

package ironicconductor

import (
	"fmt"

	ironicv1 "github.com/openstack-k8s-operators/ironic-operator/api/v1beta1"
	ironic "github.com/openstack-k8s-operators/ironic-operator/pkg/ironic"
	common "github.com/openstack-k8s-operators/lib-common/modules/common"
	affinity "github.com/openstack-k8s-operators/lib-common/modules/common/affinity"
	env "github.com/openstack-k8s-operators/lib-common/modules/common/env"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// ServiceCommand -
	ServiceCommand = "/usr/local/bin/kolla_set_configs && /usr/local/bin/kolla_start"
)

// getNetworks List -
func getNetworksList(
	instance *ironicv1.IronicConductor,
) string {
	networks := "["
	if instance.Spec.ProvisionNetwork != "" {
		networks += fmt.Sprintf(
			`{"name": "%s", "namespace": "%s"}`,
			instance.Spec.ProvisionNetwork, instance.Namespace,
		)
	}
	networks += "]"
	return networks
}

// StatefulSet func
func StatefulSet(
	instance *ironicv1.IronicConductor,
	configHash string,
	labels map[string]string,
	ingressDomain string,
) *appsv1.StatefulSet {
	runAsUser := int64(0)

	livenessProbe := &corev1.Probe{
		TimeoutSeconds:      5,
		PeriodSeconds:       30,
		InitialDelaySeconds: 5,
	}
	readinessProbe := &corev1.Probe{
		TimeoutSeconds:      5,
		PeriodSeconds:       30,
		InitialDelaySeconds: 5,
	}
	dnsmasqLivenessProbe := &corev1.Probe{
		TimeoutSeconds:      10,
		PeriodSeconds:       30,
		InitialDelaySeconds: 3,
	}
	dnsmasqReadinessProbe := &corev1.Probe{
		TimeoutSeconds:      10,
		PeriodSeconds:       30,
		InitialDelaySeconds: 3,
	}
	httpbootLivenessProbe := &corev1.Probe{
		TimeoutSeconds:      10,
		PeriodSeconds:       30,
		InitialDelaySeconds: 5,
	}
	httpbootReadinessProbe := &corev1.Probe{
		TimeoutSeconds:      10,
		PeriodSeconds:       30,
		InitialDelaySeconds: 5,
	}

	args := []string{"-c"}
	if instance.Spec.Debug.Service {
		args = append(args, common.DebugCommand)
		livenessProbe.Exec = &corev1.ExecAction{
			Command: []string{
				"/bin/true",
			},
		}

		readinessProbe.Exec = &corev1.ExecAction{
			Command: []string{
				"/bin/true",
			},
		}
		httpbootLivenessProbe.Exec = &corev1.ExecAction{
			Command: []string{
				"/bin/true",
			},
		}

		httpbootReadinessProbe.Exec = &corev1.ExecAction{
			Command: []string{
				"/bin/true",
			},
		}
		dnsmasqLivenessProbe.Exec = &corev1.ExecAction{
			Command: []string{
				"/bin/true",
			},
		}

		dnsmasqReadinessProbe.Exec = &corev1.ExecAction{
			Command: []string{
				"/bin/true",
			},
		}
	} else {
		args = append(args, ServiceCommand)

		//
		// https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/
		//
		// Make a POST request to the JSON-RPC port
		livenessProbe.Exec = &corev1.ExecAction{
			Command: []string{
				"sh", "-c", "ss -ltn | grep :8089",
			},
		}
		// Make a POST request to the JSON-RPC port
		readinessProbe.Exec = &corev1.ExecAction{
			Command: []string{
				"sh", "-c", "ss -ltn | grep :8089",
			},
		}
		httpbootLivenessProbe.Exec = &corev1.ExecAction{
			Command: []string{
				"sh", "-c", "ss -ltn | grep :8088",
			},
		}

		httpbootReadinessProbe.Exec = &corev1.ExecAction{
			Command: []string{
				"sh", "-c", "ss -ltn | grep :8088",
			},
		}
		dnsmasqLivenessProbe.Exec = &corev1.ExecAction{
			Command: []string{
				"sh", "-c", "ss -lun | grep :67 && ss -lun | grep :69",
			},
		}

		dnsmasqReadinessProbe.Exec = &corev1.ExecAction{
			Command: []string{
				"sh", "-c", "ss -lun | grep :67 && ss -lun | grep :69",
			},
		}
	}

	envVars := map[string]env.Setter{}
	envVars["KOLLA_CONFIG_FILE"] = env.SetValue(KollaConfig)
	envVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")
	envVars["CONFIG_HASH"] = env.SetValue(configHash)

	dnsmasqEnvVars := map[string]env.Setter{}
	dnsmasqEnvVars["KOLLA_CONFIG_FILE"] = env.SetValue(DnsmasqKollaConfig)
	dnsmasqEnvVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")
	dnsmasqEnvVars["CONFIG_HASH"] = env.SetValue(configHash)

	httpbootEnvVars := map[string]env.Setter{}
	httpbootEnvVars["KOLLA_CONFIG_FILE"] = env.SetValue(HttpbootKollaConfig)
	httpbootEnvVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")
	httpbootEnvVars["CONFIG_HASH"] = env.SetValue(configHash)

	conductorContainer := corev1.Container{
		Name: ironic.ServiceName + "-" + ironic.ConductorComponent,
		Command: []string{
			"/bin/bash",
		},
		Args:  args,
		Image: instance.Spec.ContainerImage,
		SecurityContext: &corev1.SecurityContext{
			RunAsUser: &runAsUser,
		},
		Env:            env.MergeEnvs([]corev1.EnvVar{}, envVars),
		VolumeMounts:   GetVolumeMounts(),
		Resources:      instance.Spec.Resources,
		ReadinessProbe: readinessProbe,
		LivenessProbe:  livenessProbe,
		// StartupProbe:   startupProbe,
	}
	httpbootContainer := corev1.Container{
		Name: "httpboot",
		Command: []string{
			"/bin/bash",
		},
		Args:  args,
		Image: instance.Spec.PxeContainerImage,
		SecurityContext: &corev1.SecurityContext{
			RunAsUser: &runAsUser,
		},
		Env:            env.MergeEnvs([]corev1.EnvVar{}, httpbootEnvVars),
		VolumeMounts:   GetVolumeMounts(),
		Resources:      instance.Spec.Resources,
		ReadinessProbe: httpbootReadinessProbe,
		LivenessProbe:  httpbootLivenessProbe,
		// StartupProbe:   startupProbe,
	}

	containers := []corev1.Container{
		conductorContainer,
		httpbootContainer,
	}

	if instance.Spec.ProvisionNetwork != "" {
		// Only include the dnsmasq container if there is a provisioning network to listen on.
		dnsmasqContainer := corev1.Container{
			Name: "dnsmasq",
			Command: []string{
				"/bin/bash",
			},
			Args:  args,
			Image: instance.Spec.PxeContainerImage,
			SecurityContext: &corev1.SecurityContext{
				RunAsUser: &runAsUser,
				Capabilities: &corev1.Capabilities{
					Add: []corev1.Capability{
						"NET_ADMIN",
						"NET_RAW",
					},
				},
			},
			Env:            env.MergeEnvs([]corev1.EnvVar{}, dnsmasqEnvVars),
			VolumeMounts:   GetVolumeMounts(),
			Resources:      instance.Spec.Resources,
			ReadinessProbe: dnsmasqReadinessProbe,
			LivenessProbe:  dnsmasqLivenessProbe,
			// StartupProbe:   startupProbe,
		}
		containers = []corev1.Container{
			conductorContainer,
			httpbootContainer,
			dnsmasqContainer,
		}
	}

	statefulset := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Replicas: &instance.Spec.Replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
					Annotations: map[string]string{
						"k8s.v1.cni.cncf.io/networks": getNetworksList(instance),
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: ironic.ServiceAccount,
					Containers:         containers,
				},
			},
		},
	}
	statefulset.Spec.Template.Spec.Volumes = GetVolumes(ironic.ServiceName, instance.Name)
	// If possible two pods of the same service should not
	// run on the same worker node. If this is not possible
	// the get still created on the same worker node.
	statefulset.Spec.Template.Spec.Affinity = affinity.DistributePods(
		common.AppSelector,
		[]string{
			ironic.ServiceName,
		},
		corev1.LabelHostname,
	)
	if instance.Spec.NodeSelector != nil && len(instance.Spec.NodeSelector) > 0 {
		statefulset.Spec.Template.Spec.NodeSelector = instance.Spec.NodeSelector
	}
	// init.sh needs to detect and set ProvisionNetworkIP
	deployHTTPURL := "http://%(ProvisionNetworkIP)s:8088/"
	if instance.Spec.ProvisionNetwork == "" {
		// Build what the fully qualified Route hostname will be when the Route exists
		deployHTTPURL = "http://%(PodName)s-%(PodNamespace)s.apps.%(IngressDomain)s/"
	}

	initContainerDetails := ironic.APIDetails{
		ContainerImage:       instance.Spec.ContainerImage,
		PxeContainerImage:    instance.Spec.PxeContainerImage,
		DatabaseHost:         instance.Spec.DatabaseHostname,
		DatabaseUser:         instance.Spec.DatabaseUser,
		DatabaseName:         ironic.DatabaseName,
		OSPSecret:            instance.Spec.Secret,
		TransportURLSecret:   instance.Spec.TransportURLSecret,
		DBPasswordSelector:   instance.Spec.PasswordSelectors.Database,
		UserPasswordSelector: instance.Spec.PasswordSelectors.Service,
		VolumeMounts:         GetInitVolumeMounts(),
		PxeInit:              true,
		ConductorInit:        true,
		DeployHTTPURL:        deployHTTPURL,
		IngressDomain:        ingressDomain,
	}
	statefulset.Spec.Template.Spec.InitContainers = ironic.InitContainer(initContainerDetails)

	return statefulset
}
