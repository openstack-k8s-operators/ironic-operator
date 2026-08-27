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
	"context"
	"fmt"
	"net"

	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	ironicv1 "github.com/openstack-k8s-operators/ironic-operator/api/v1beta1"
	ironic "github.com/openstack-k8s-operators/ironic-operator/internal/ironic"
	common "github.com/openstack-k8s-operators/lib-common/modules/common"
	affinity "github.com/openstack-k8s-operators/lib-common/modules/common/affinity"
	env "github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/pod"
	"github.com/openstack-k8s-operators/lib-common/modules/users"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	resource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	intstr "k8s.io/apimachinery/pkg/util/intstr"
	k8snet "k8s.io/utils/net"
	"k8s.io/utils/ptr"
)

// StatefulSet func
func StatefulSet(
	ctx context.Context,
	instance *ironicv1.IronicConductor,
	configHash string,
	labels map[string]string,
	ingressDomain string,
	annotations map[string]string,
	topology *topologyv1.Topology,
) (*appsv1.StatefulSet, error) {

	livenessProbe := &corev1.Probe{
		TimeoutSeconds: 5,
		// [conductor]heartbeat_timeout is set to 120 so make PeriodSeconds
		// more frequent to catch an offline conductor earlier
		PeriodSeconds: 30,
	}
	startupProbe := &corev1.Probe{
		TimeoutSeconds:   5,
		FailureThreshold: 30,
		PeriodSeconds:    2,
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

	//
	// https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/
	//

	livenessProbe.Exec = &corev1.ExecAction{
		Command: []string{
			"/usr/local/bin/container-scripts/live_check_conductor",
		},
	}
	startupProbe.Exec = &corev1.ExecAction{
		Command: []string{
			"/usr/local/bin/container-scripts/live_check_conductor",
		},
	}

	httpbootLivenessProbe.TCPSocket = &corev1.TCPSocketAction{
		Port: intstr.IntOrString{Type: intstr.Int, IntVal: int32(8088)},
	}
	httpbootReadinessProbe.TCPSocket = &corev1.TCPSocketAction{
		Port: intstr.IntOrString{Type: intstr.Int, IntVal: int32(8088)},
	}

	// Parse the storageRequest defined in the CR
	storageRequest, err := resource.ParseQuantity(instance.Spec.StorageRequest)
	if err != nil {
		return nil, err
	}
	// dnsmasq only listen on ports 67 and/or 547 when DHCPRanges are configured.
	dnsmasqProbeCommand := []string{"sh", "-c", "ss -lun | grep :69"}
	ipv6Probe := false
	ipv4Probe := false
	for _, dhcpRangeSpec := range instance.Spec.DHCPRanges {
		_, ipPrefix, _ := net.ParseCIDR(dhcpRangeSpec.Cidr)
		if k8snet.IsIPv4CIDR(ipPrefix) {
			ipv4Probe = true
		}
		if k8snet.IsIPv6CIDR(ipPrefix) {
			ipv6Probe = true
		}
	}
	if ipv4Probe && !ipv6Probe {
		dnsmasqProbeCommand = []string{"sh", "-c", "ss -lun | grep :67 && ss -lun | grep :69"}
	} else if !ipv4Probe && ipv6Probe {
		dnsmasqProbeCommand = []string{"sh", "-c", "ss -lun | grep :547 && ss -lun | grep :69"}
	} else if ipv4Probe && ipv6Probe {
		dnsmasqProbeCommand = []string{"sh", "-c", "ss -lun | grep :547 && ss -lun | grep :67 && ss -lun | grep :69"}
	}
	dnsmasqLivenessProbe.Exec = &corev1.ExecAction{Command: dnsmasqProbeCommand}
	dnsmasqReadinessProbe.Exec = &corev1.ExecAction{Command: dnsmasqProbeCommand}

	envVars := map[string]env.Setter{}
	envVars["CONFIG_HASH"] = env.SetValue(configHash)

	dnsmasqEnvVars := map[string]env.Setter{}
	dnsmasqEnvVars["CONFIG_HASH"] = env.SetValue(configHash)

	httpbootEnvVars := map[string]env.Setter{}
	httpbootEnvVars["CONFIG_HASH"] = env.SetValue(configHash)

	ramdiskLogsEnvVars := map[string]env.Setter{}
	ramdiskLogsEnvVars["CONFIG_HASH"] = env.SetValue(configHash)

	volumes := GetVolumes(ctx, instance)
	conductorVolumeMounts := GetConductorVolumeMounts(instance)
	httpbootVolumeMounts := GetHttpbootVolumeMounts()
	dnsmasqVolumeMounts := GetDnsmasqVolumeMounts()
	ramdiskLogsVolumeMounts := GetRamdiskLogsVolumeMounts()
	initVolumeMounts := GetInitVolumeMounts(instance)

	// Add the CA bundle
	if instance.Spec.TLS.CaBundleSecretName != "" {
		volumes = append(volumes, instance.Spec.TLS.CreateVolume())
		conductorVolumeMounts = append(conductorVolumeMounts, instance.Spec.TLS.CreateVolumeMounts(nil)...)
		httpbootVolumeMounts = append(httpbootVolumeMounts, instance.Spec.TLS.CreateVolumeMounts(nil)...)
		dnsmasqVolumeMounts = append(dnsmasqVolumeMounts, instance.Spec.TLS.CreateVolumeMounts(nil)...)
		ramdiskLogsVolumeMounts = append(ramdiskLogsVolumeMounts, instance.Spec.TLS.CreateVolumeMounts(nil)...)
		initVolumeMounts = append(initVolumeMounts, instance.Spec.TLS.CreateVolumeMounts(nil)...)
	}

	resourceName := fmt.Sprintf("%s-%s", ironic.ServiceName, ironic.ConductorComponent)
	conductorContainer := corev1.Container{
		Name: resourceName,
		Command: []string{
			"/usr/bin/ironic-conductor",
		},
		Args:            []string{"--config-file", "/etc/ironic/ironic.conf", "--config-dir", "/etc/ironic/ironic.conf.d"},
		Image:           instance.Spec.ContainerImage,
		SecurityContext: pod.RestrictiveSecurityContext(users.IronicUID, users.IronicGID),
		Env:             env.MergeEnvs([]corev1.EnvVar{}, envVars),
		VolumeMounts:    conductorVolumeMounts,
		Resources:       instance.Spec.Resources,
		LivenessProbe:   livenessProbe,
		StartupProbe:    startupProbe,
	}
	httpbootContainer := corev1.Container{
		Name: "httpboot",
		Command: []string{
			"/usr/sbin/httpd",
		},
		Args:            []string{"-DFOREGROUND"},
		Image:           instance.Spec.PxeContainerImage,
		SecurityContext: pod.RestrictiveSecurityContext(users.IronicUID, users.IronicGID),
		Env:             env.MergeEnvs([]corev1.EnvVar{}, httpbootEnvVars),
		VolumeMounts:    httpbootVolumeMounts,
		Resources:       instance.Spec.Resources,
		ReadinessProbe:  httpbootReadinessProbe,
		LivenessProbe:   httpbootLivenessProbe,
		// StartupProbe:   startupProbe,
	}
	ramdiskLogsContainer := corev1.Container{
		Name: "ramdisk-logs",
		Command: []string{
			"/usr/local/bin/container-scripts/runlogwatch.sh",
		},
		Image:           instance.Spec.ContainerImage,
		SecurityContext: pod.RestrictiveSecurityContext(users.IronicUID, users.IronicGID),
		Env:             env.MergeEnvs([]corev1.EnvVar{}, ramdiskLogsEnvVars),
		VolumeMounts:    ramdiskLogsVolumeMounts,
	}

	containers := []corev1.Container{
		conductorContainer,
		httpbootContainer,
		ramdiskLogsContainer,
	}

	if instance.Spec.ProvisionNetwork != "" {
		// Only include the dnsmasq container if there is a provisioning network to listen on.
		dnsmasqContainer := corev1.Container{
			Name: "dnsmasq",
			Command: []string{
				"/usr/sbin/dnsmasq",
			},
			Args:  []string{"-k"},
			Image: instance.Spec.PxeContainerImage,
			// dnsmasq binds ports <1024 and handles raw DHCP packets --
			// genuine need. Explicit RunAsUser 0 / RunAsNonRoot false:
			// this pod carries a RestrictivePodSecurityContext for its
			// non-root siblings; a container's own explicit fields
			// override the pod-level default, exempting dnsmasq.
			SecurityContext: &corev1.SecurityContext{
				RunAsUser:    ptr.To(int64(0)),
				RunAsNonRoot: ptr.To(false),
				Capabilities: &corev1.Capabilities{
					Drop: []corev1.Capability{"ALL"},
					// With ALL dropped, root grants nothing on its own --
					// each capability dnsmasq uses must be listed:
					Add: []corev1.Capability{
						"NET_BIND_SERVICE", // bind TFTP 69 / DHCP 67,547 (<1024)
						"NET_ADMIN",
						"NET_RAW",
						"SETGID", // drop to the unprivileged dnsmasq group
					},
				},
			},
			Env:            env.MergeEnvs([]corev1.EnvVar{}, dnsmasqEnvVars),
			VolumeMounts:   dnsmasqVolumeMounts,
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

	// Use terminationGracePeriodSeconds from CR
	terminationGracePeriod := *instance.Spec.TerminationGracePeriodSeconds

	statefulset := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Replicas: instance.Spec.Replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: annotations,
					Labels:      labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName:           instance.RbacResourceName(),
					AutomountServiceAccountToken: ptr.To(false),
					// dnsmasq (added above, when present) explicitly
					// overrides RunAsUser/RunAsNonRoot back to root.
					SecurityContext:               pod.RestrictivePodSecurityContext(users.IronicUID, users.IronicGID, users.ApacheGID),
					Containers:                    containers,
					TerminationGracePeriodSeconds: &terminationGracePeriod,
					Volumes:                       volumes,
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "var-lib-ironic",
						Labels: labels,
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{
							"ReadWriteOnce",
						},
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: storageRequest,
							},
						},
						StorageClassName: &instance.Spec.StorageClass,
					},
				},
			},
		},
	}
	if instance.Spec.NodeSelector != nil {
		statefulset.Spec.Template.Spec.NodeSelector = *instance.Spec.NodeSelector
	}
	if topology != nil {
		topology.ApplyTo(&statefulset.Spec.Template)
	} else {
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
	}

	// init.sh needs to detect and set ProvisionNetworkIP
	deployHTTPURL := "http://%(ProvisionNetworkIP)s:8088/"
	if instance.Spec.ProvisionNetwork == "" {
		// Build what the fully qualified Route hostname will be when the Route exists
		deployHTTPURL = "http://%(PodName)s-%(PodNamespace)s.%(IngressDomain)s/"
	}

	initContainerDetails := ironic.APIDetails{
		ContainerImage:         instance.Spec.ContainerImage,
		PxeContainerImage:      instance.Spec.PxeContainerImage,
		IronicPythonAgentImage: instance.Spec.IronicPythonAgentImage,
		ImageDirectory:         ironic.ImageDirectory,
		DatabaseHost:           instance.Spec.DatabaseHostname,
		DatabaseName:           ironic.DatabaseName,
		OSPSecret:              instance.Spec.Secret,
		TransportURLSecret:     instance.Spec.TransportURLSecret,
		UserPasswordSelector:   instance.Spec.PasswordSelectors.Service,
		VolumeMounts:           initVolumeMounts,
		PxeInit:                true,
		ConductorInit:          true,
		DeployHTTPURL:          deployHTTPURL,
		IngressDomain:          ingressDomain,
		ProvisionNetwork:       instance.Spec.ProvisionNetwork,
	}
	statefulset.Spec.Template.Spec.InitContainers = ironic.InitContainer(initContainerDetails)

	return statefulset, nil
}
