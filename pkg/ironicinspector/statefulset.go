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

package ironicinspector

import (
	"fmt"

	ironicv1 "github.com/openstack-k8s-operators/ironic-operator/api/v1beta1"
	ironic "github.com/openstack-k8s-operators/ironic-operator/pkg/ironic"
	common "github.com/openstack-k8s-operators/lib-common/modules/common"
	affinity "github.com/openstack-k8s-operators/lib-common/modules/common/affinity"
	"github.com/openstack-k8s-operators/lib-common/modules/common/annotations"
	env "github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	intstr "k8s.io/apimachinery/pkg/util/intstr"
)

const (
	// ServiceCommand -
	ServiceCommand = "/usr/local/bin/kolla_set_configs && /usr/local/bin/kolla_start"
)

// StatefulSet func
func StatefulSet(
	instance *ironicv1.IronicInspector,
	configHash string,
	labels map[string]string,
	ingressDomain string,
) (*appsv1.StatefulSet, error) {
	runAsUser := int64(0)

	livenessProbe := &corev1.Probe{
		TimeoutSeconds:      5,
		PeriodSeconds:       5,
		InitialDelaySeconds: 3,
	}
	readinessProbe := &corev1.Probe{
		TimeoutSeconds:      5,
		PeriodSeconds:       5,
		InitialDelaySeconds: 3,
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
	} else {
		args = append(args, ServiceCommand)

		//
		// https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/
		//
		livenessProbe.HTTPGet = &corev1.HTTPGetAction{
			Path: "/v1",
			Port: intstr.IntOrString{Type: intstr.Int, IntVal: int32(IronicInspectorInternalPort)},
		}
		readinessProbe.HTTPGet = &corev1.HTTPGetAction{
			Path: "/v1",
			Port: intstr.IntOrString{Type: intstr.Int, IntVal: int32(IronicInspectorInternalPort)},
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
	}

	containers := []corev1.Container{}

	inspectorEnvVars := map[string]env.Setter{}
	inspectorEnvVars["KOLLA_CONFIG_FILE"] = env.SetValue(KollaConfig)
	inspectorEnvVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")
	inspectorEnvVars["CONFIG_HASH"] = env.SetValue(configHash)
	inspectorContainer := corev1.Container{
		Name:  ironic.ServiceName + "-" + ironic.InspectorComponent,
		Image: instance.Spec.ContainerImage,
		Command: []string{
			"/bin/bash",
		},
		Args:            args,
		SecurityContext: &corev1.SecurityContext{RunAsUser: &runAsUser},
		Env:             env.MergeEnvs([]corev1.EnvVar{}, inspectorEnvVars),
		VolumeMounts:    GetVolumeMounts(),
		Resources:       instance.Spec.Resources,
		ReadinessProbe:  readinessProbe,
		LivenessProbe:   livenessProbe,
	}
	containers = append(containers, inspectorContainer)

	httpbootEnvVars := map[string]env.Setter{}
	httpbootEnvVars["KOLLA_CONFIG_FILE"] = env.SetValue(HttpbootKollaConfig)
	httpbootEnvVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")
	httpbootEnvVars["CONFIG_HASH"] = env.SetValue(configHash)
	httpbootContainer := corev1.Container{
		Name:  ironic.InspectorComponent + "-" + "httpboot",
		Image: instance.Spec.PxeContainerImage,
		Command: []string{
			"/bin/bash",
		},
		Args: args,
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
	containers = append(containers, httpbootContainer)

	if instance.Spec.InspectionNetwork != "" {
		// Only include dnsmasq container if there is an inspection network
		dnsmasqEnvVars := map[string]env.Setter{}
		dnsmasqEnvVars["KOLLA_CONFIG_FILE"] = env.SetValue(DnsmasqKollaConfig)
		dnsmasqEnvVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")
		dnsmasqEnvVars["CONFIG_HASH"] = env.SetValue(configHash)
		dnsmasqContainer := corev1.Container{
			Name:  ironic.InspectorComponent + "-" + "dnsmasq",
			Image: instance.Spec.ContainerImage,
			Command: []string{
				"/bin/bash",
			},
			Args: args,
			SecurityContext: &corev1.SecurityContext{
				RunAsUser: &runAsUser,
				Capabilities: &corev1.Capabilities{
					Add: []corev1.Capability{
						"NET_ADMIN", "NET_RAW",
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
		containers = append(containers, dnsmasqContainer)
	}

	// Default oslo.service graceful_shutdown_timeout is 60, so align with that
	terminationGracePeriod := int64(60)

	statefulset := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ironic.ServiceName + "-" + ironic.InspectorComponent,
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
				},
				Spec: corev1.PodSpec{
					ServiceAccountName:            ServiceAccount,
					Containers:                    containers,
					TerminationGracePeriodSeconds: &terminationGracePeriod,
				},
			},
		},
	}
	statefulset.Spec.Template.Spec.Volumes = GetVolumes(ironic.ServiceName + "-" + ironic.InspectorComponent)

	// networks to attach to
	nwAnnotation, err := annotations.GetNADAnnotation(instance.Namespace, instance.Spec.NetworkAttachments)
	if err != nil {
		return nil, fmt.Errorf("failed create network annotation from %s: %w",
			instance.Spec.NetworkAttachments, err)
	}
	statefulset.Spec.Template.Annotations = util.MergeStringMaps(statefulset.Spec.Template.Annotations, nwAnnotation)

	// If possible two pods of the same service should not
	// run on the same worker node. If this is not possible
	// the get still created on the same worker node.
	statefulset.Spec.Template.Spec.Affinity = affinity.DistributePods(
		common.AppSelector,
		[]string{ironic.ServiceName},
		corev1.LabelHostname,
	)
	if instance.Spec.NodeSelector != nil && len(instance.Spec.NodeSelector) > 0 {
		statefulset.Spec.Template.Spec.NodeSelector = instance.Spec.NodeSelector
	}

	// init.sh needs to detect and set InspectionNetworkIP
	inspectorHTTPURL := "http://%(InspectorNetworkIP)s:8088/"
	if instance.Spec.InspectionNetwork == "" {
		// Build what the fully qualified Route hostname will be when the Route exists
		inspectorHTTPURL = "http://%(PodName)s-%(PodNamespace)s.%(IngressDomain)s/"
	}

	initContainerDetails := APIDetails{
		ContainerImage:       instance.Spec.ContainerImage,
		PxeContainerImage:    instance.Spec.PxeContainerImage,
		DatabaseHost:         instance.Status.DatabaseHostname,
		DatabaseUser:         instance.Spec.DatabaseUser,
		DatabaseName:         DatabaseName,
		OSPSecret:            instance.Spec.Secret,
		TransportURLSecret:   instance.Status.TransportURLSecret,
		DBPasswordSelector:   instance.Spec.PasswordSelectors.Database,
		UserPasswordSelector: instance.Spec.PasswordSelectors.Service,
		VolumeMounts:         GetInitVolumeMounts(),
		PxeInit:              true,
		InspectorHTTPURL:     inspectorHTTPURL,
		IngressDomain:        ingressDomain,
		InspectionNetwork:    instance.Spec.InspectionNetwork,
	}
	statefulset.Spec.Template.Spec.InitContainers = InitContainer(initContainerDetails)

	return statefulset, err
}
