/*
Copyright 2023 Red Hat Inc.

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

package ironicneutronagent

import (
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	ironicv1 "github.com/openstack-k8s-operators/ironic-operator/api/v1beta1"
	common "github.com/openstack-k8s-operators/lib-common/modules/common"
	affinity "github.com/openstack-k8s-operators/lib-common/modules/common/affinity"
	env "github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/pod"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	"github.com/openstack-k8s-operators/lib-common/modules/users"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

// OvnCertService returns the tls.Service for the OVN DB client cert.
// It is used for both the validation of the secret and the volume
// mounts so the two cannot diverge.
func OvnCertService(secretName string) tls.Service {
	return tls.Service{
		SecretName: secretName,
		CertMount:  ptr.To("/etc/pki/tls/certs/ovndb.crt"),
		KeyMount:   ptr.To("/etc/pki/tls/private/ovndb.key"),
		CaMount:    ptr.To("/etc/pki/tls/certs/ovndbca.crt"),
	}
}

// Deployment func
func Deployment(
	instance *ironicv1.IronicNeutronAgent,
	configHash string,
	labels map[string]string,
	topology *topologyv1.Topology,
	ovnCRDAvailable bool,
) *appsv1.Deployment {

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

	// https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/
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

	envVars := map[string]env.Setter{}
	envVars["CONFIG_HASH"] = env.SetValue(configHash)

	volumes := GetVolumes(instance.Name, instance.Spec.ExtraMounts, IronicNeutronAgentPropagation)
	volumeMounts := GetVolumeMounts(instance.Spec.ExtraMounts, IronicNeutronAgentPropagation)

	// Add the CA bundle
	if instance.Spec.TLS.CaBundleSecretName != "" {
		volumes = append(volumes, instance.Spec.TLS.CreateVolume())
		volumeMounts = append(volumeMounts, instance.Spec.TLS.CreateVolumeMounts(nil)...)
	}

	// Add the OVN DB client cert
	if ovnCRDAvailable && instance.Spec.TLS.Ovn.Enabled() {
		svc := OvnCertService(*instance.Spec.TLS.Ovn.SecretName)
		volumes = append(volumes, svc.CreateVolume("ovndb"))
		volumeMounts = append(volumeMounts, svc.CreateVolumeMounts("ovndb")...)
	}

	// Default oslo.service graceful_shutdown_timeout is 60, so align with that
	terminationGracePeriod := int64(60)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ServiceName,
			Namespace: instance.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Replicas: instance.Spec.Replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName:           instance.RbacResourceName(),
					AutomountServiceAccountToken: ptr.To(false),
					SecurityContext:              pod.RestrictivePodSecurityContext(users.NeutronUID, users.NeutronGID),
					Containers: []corev1.Container{
						{
							Name: ServiceName,
							Command: []string{
								"/usr/bin/ironic-neutron-agent",
							},
							Args:            []string{"--config-dir", "/etc/neutron/neutron.conf.d"},
							Image:           instance.Spec.ContainerImage,
							SecurityContext: pod.RestrictiveSecurityContext(users.NeutronUID, users.NeutronGID),
							Env:             env.MergeEnvs([]corev1.EnvVar{}, envVars),
							VolumeMounts:    volumeMounts,
							Resources:       instance.Spec.Resources,
							ReadinessProbe:  readinessProbe,
							LivenessProbe:   livenessProbe,
						},
					},
					TerminationGracePeriodSeconds: &terminationGracePeriod,
					Volumes:                       volumes,
				},
			},
		},
	}
	if instance.Spec.NodeSelector != nil {
		deployment.Spec.Template.Spec.NodeSelector = *instance.Spec.NodeSelector
	}
	if topology != nil {
		topology.ApplyTo(&deployment.Spec.Template)
	} else {
		// If possible two pods of the same service should not
		// run on the same worker node. If this is not possible
		// the get still created on the same worker node.
		deployment.Spec.Template.Spec.Affinity = affinity.DistributePods(
			common.AppSelector,
			[]string{
				ServiceName,
			},
			corev1.LabelHostname,
		)
	}

	return deployment
}
