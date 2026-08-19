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

package heatcfnapi

import (
	"fmt"

	heatv1beta1 "github.com/openstack-k8s-operators/heat-operator/api/v1beta1"
	heat "github.com/openstack-k8s-operators/heat-operator/internal/heat"
	memcachedv1 "github.com/openstack-k8s-operators/infra-operator/apis/memcached/v1beta1"
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	common "github.com/openstack-k8s-operators/lib-common/modules/common"
	affinity "github.com/openstack-k8s-operators/lib-common/modules/common/affinity"
	env "github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/pod"
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	"github.com/openstack-k8s-operators/lib-common/modules/common/volume"
	"github.com/openstack-k8s-operators/lib-common/modules/users"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

// Deployment func
func Deployment(
	instance *heatv1beta1.HeatCfnAPI,
	configHash string,
	labels map[string]string,
	topology *topologyv1.Topology,
	memcached *memcachedv1.Memcached,
) (*appsv1.Deployment, error) {

	livenessProbe := heat.FormatProbes(heat.HeatCfnInternalPort)
	readinessProbe := heat.FormatProbes(heat.HeatCfnInternalPort)

	if instance.Spec.TLS.API.Enabled(service.EndpointPublic) {
		livenessProbe.HTTPGet.Scheme = corev1.URISchemeHTTPS
		readinessProbe.HTTPGet.Scheme = corev1.URISchemeHTTPS
	}

	// create Volume and VolumeMounts
	volumes := heat.GetVolumes(heat.ServiceName, instance.Name,
		instance.Spec.ExtraMounts, heat.HeatAPIPropagation)
	volumeMounts := heat.GetVolumeMounts(instance.Spec.ExtraMounts,
		heat.HeatAPIPropagation)
	secretVolumes, secretMounts := volume.ConfigSecretVolumes(instance.Spec.CustomServiceConfigSecrets)
	volumes = append(volumes, secretVolumes...)
	volumeMounts = append(volumeMounts, secretMounts...)

	volumes = append(volumes, volume.WritableDirVolume(volume.RunHttpdVolumeName))
	volumeMounts = append(volumeMounts,
		corev1.VolumeMount{
			Name:      "config-data",
			MountPath: "/etc/httpd/conf/httpd.conf",
			SubPath:   ServiceName + "-httpd.conf",
			ReadOnly:  true,
		},
		corev1.VolumeMount{
			Name:      "config-data",
			MountPath: "/etc/httpd/conf.d/ssl.conf",
			SubPath:   "ssl.conf",
			ReadOnly:  true,
		},
		volume.WritableDirVolumeMount(volume.RunHttpdVolumeName, volume.RunHttpdMountPath),
	)

	if err := formatTLS(instance, &volumes, &volumeMounts, memcached); err != nil {
		return nil, err
	}

	envVars := map[string]env.Setter{}
	envVars["CONFIG_HASH"] = env.SetValue(configHash)

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
					ServiceAccountName:           instance.Spec.ServiceAccount,
					AutomountServiceAccountToken: ptr.To(false),
					SecurityContext:              pod.RestrictivePodSecurityContext(users.HeatUID, users.HeatGID),
					Containers: []corev1.Container{
						{
							Name: heat.ServiceName + "-" + heat.CfnAPIComponent,
							Command: []string{
								"/usr/sbin/httpd",
							},
							Args:            []string{"-DFOREGROUND"},
							Image:           instance.Spec.ContainerImage,
							SecurityContext: pod.RestrictiveSecurityContext(users.HeatUID, users.HeatGID),
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
				heat.ServiceName,
			},
			corev1.LabelHostname,
		)
	}

	return deployment, nil
}

func formatTLS(instance *heatv1beta1.HeatCfnAPI, volumes *[]corev1.Volume, volumeMounts *[]corev1.VolumeMount, memcached *memcachedv1.Memcached) error {

	if instance.Spec.TLS.CaBundleSecretName != "" {
		*volumes = append(*volumes, instance.Spec.TLS.CreateVolume())
		*volumeMounts = append(*volumeMounts, instance.Spec.TLS.CreateVolumeMounts(nil)...)
	}

	// add MTLS cert if defined
	if memcached.GetMemcachedMTLSSecret() != "" {
		certMountPath := memcachedv1.CertPathDst
		keyMountPath := memcachedv1.KeyPathDst
		*volumes = append(*volumes, memcached.CreateMTLSVolume())
		*volumeMounts = append(*volumeMounts, memcached.CreateMTLSVolumeMounts(&certMountPath, &keyMountPath)...)
	}

	for _, endpt := range []service.Endpoint{service.EndpointInternal, service.EndpointPublic} {
		if instance.Spec.TLS.API.Enabled(endpt) {
			var tlsEndptCfg tls.GenericService
			switch endpt {
			case service.EndpointPublic:
				tlsEndptCfg = instance.Spec.TLS.API.Public
			case service.EndpointInternal:
				tlsEndptCfg = instance.Spec.TLS.API.Internal
			}

			svc, err := tlsEndptCfg.ToService()
			if err != nil {
				return err
			}
			certMount := fmt.Sprintf("/etc/pki/tls/certs/%s.crt", endpt.String())
			keyMount := fmt.Sprintf("/etc/pki/tls/private/%s.key", endpt.String())
			svc.CertMount = &certMount
			svc.KeyMount = &keyMount
			*volumes = append(*volumes, svc.CreateVolume(endpt.String()))
			*volumeMounts = append(*volumeMounts, svc.CreateVolumeMounts(endpt.String())...)
		}
	}

	return nil
}
