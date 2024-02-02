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

package ironic

import (
	ironicv1 "github.com/openstack-k8s-operators/ironic-operator/api/v1beta1"

	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// DBSyncCommand -
	DBSyncCommand = "/usr/local/bin/container-scripts/dbsync.sh"
)

// DbSyncJob func
func DbSyncJob(
	instance *ironicv1.Ironic,
	labels map[string]string,
) *batchv1.Job {
	runAsUser := int64(0)

	args := []string{"-c", DBSyncCommand}

	envVars := map[string]env.Setter{}
	envVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")
	envVars["KOLLA_BOOTSTRAP"] = env.SetValue("true")

	volumes := GetVolumes(ServiceName)
	volumeMounts := GetDBSyncVolumeMounts()
	initVolumeMounts := GetInitVolumeMounts()

	// add CA cert if defined
	if instance.Spec.IronicAPI.TLS.CaBundleSecretName != "" {
		volumes = append(volumes, instance.Spec.IronicAPI.TLS.CreateVolume())
		volumeMounts = append(volumeMounts, instance.Spec.IronicAPI.TLS.CreateVolumeMounts(nil)...)
		initVolumeMounts = append(initVolumeMounts, instance.Spec.IronicAPI.TLS.CreateVolumeMounts(nil)...)
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ServiceName + "-db-sync",
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:      corev1.RestartPolicyOnFailure,
					ServiceAccountName: instance.RbacResourceName(),
					Containers: []corev1.Container{
						{
							Name: ServiceName + "-db-sync",
							Command: []string{
								"/bin/bash",
							},
							Args:  args,
							Image: instance.Spec.Images.API,
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: &runAsUser,
							},
							Env:          env.MergeEnvs([]corev1.EnvVar{}, envVars),
							VolumeMounts: volumeMounts,
						},
					},
					Volumes: volumes,
				},
			},
		},
	}

	initContainerDetails := APIDetails{
		ContainerImage:       instance.Spec.Images.Conductor,
		DatabaseHost:         instance.Status.DatabaseHostname,
		DatabaseName:         DatabaseName,
		OSPSecret:            instance.Spec.Secret,
		DBPasswordSelector:   instance.Spec.PasswordSelectors.Database,
		UserPasswordSelector: instance.Spec.PasswordSelectors.Service,
		VolumeMounts:         initVolumeMounts,
	}
	job.Spec.Template.Spec.InitContainers = InitContainer(initContainerDetails)

	return job
}
