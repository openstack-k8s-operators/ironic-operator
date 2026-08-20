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
	ironicv1 "github.com/openstack-k8s-operators/ironic-operator/api/v1beta1"
	ironic "github.com/openstack-k8s-operators/ironic-operator/internal/ironic"

	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/pod"
	"github.com/openstack-k8s-operators/lib-common/modules/users"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

// DbSyncJob func
func DbSyncJob(
	instance *ironicv1.IronicInspector,
	labels map[string]string,
) *batchv1.Job {

	envVars := map[string]env.Setter{}

	volumes := GetVolumes(ironic.ServiceName + "-" + ironic.InspectorComponent)
	volumeMounts := GetInspectorVolumeMounts()
	initVolumeMounts := GetInitVolumeMounts()

	// add CA cert if defined
	if instance.Spec.TLS.CaBundleSecretName != "" {
		volumes = append(volumes, instance.Spec.TLS.CreateVolume())
		volumeMounts = append(volumeMounts, instance.Spec.TLS.CreateVolumeMounts(nil)...)
		initVolumeMounts = append(initVolumeMounts, instance.Spec.TLS.CreateVolumeMounts(nil)...)
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ironic.ServiceName + "-" + ironic.InspectorComponent + "-db-sync",
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:                corev1.RestartPolicyOnFailure,
					ServiceAccountName:           instance.RbacResourceName(),
					AutomountServiceAccountToken: ptr.To(false),
					SecurityContext:              pod.RestrictivePodSecurityContext(users.IronicInspectorUID, users.IronicInspectorGID),
					Containers: []corev1.Container{
						{
							Name: ironic.ServiceName + "-" + ironic.InspectorComponent + "-db-sync",
							Command: []string{
								"ironic-inspector-dbsync",
							},
							Args:            []string{"--config-file", "/etc/ironic-inspector/inspector.conf", "--config-dir", "/etc/ironic-inspector/inspector.conf.d", "upgrade"},
							Image:           instance.Spec.ContainerImage,
							SecurityContext: pod.RestrictiveSecurityContext(users.IronicInspectorUID, users.IronicInspectorGID),
							Env:             env.MergeEnvs([]corev1.EnvVar{}, envVars),
							VolumeMounts:    volumeMounts,
						},
					},
					Volumes: volumes,
				},
			},
		},
	}

	initContainerDetails := APIDetails{
		ContainerImage:       instance.Spec.ContainerImage,
		DatabaseHost:         instance.Status.DatabaseHostname,
		DatabaseName:         DatabaseName,
		OSPSecret:            instance.Spec.Secret,
		UserPasswordSelector: instance.Spec.PasswordSelectors.Service,
		VolumeMounts:         initVolumeMounts,
	}
	job.Spec.Template.Spec.InitContainers = InitContainer(initContainerDetails)

	if instance.Spec.NodeSelector != nil {
		job.Spec.Template.Spec.NodeSelector = *instance.Spec.NodeSelector
	}

	return job
}
