package heatengine

import (
	"github.com/openstack-k8s-operators/heat-operator/pkg/heat"
	corev1 "k8s.io/api/core/v1"
)

// getVolumes -
func getVolumes(parentName string, name string) []corev1.Volume {

	volumes := []corev1.Volume{
		{
			Name: "config-data-custom",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: name + "-config-data",
				},
			},
		},
	}

	return append(heat.GetVolumes(parentName), volumes...)
}

// getVolumeMounts - heat API VolumeMounts
func getVolumeMounts() []corev1.VolumeMount {

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "config-data",
			MountPath: "/var/lib/kolla/config_files/config.json",
			SubPath:   "heat-engine-config.json",
			ReadOnly:  true,
		},
	}

	return append(heat.GetVolumeMounts(), volumeMounts...)
}
