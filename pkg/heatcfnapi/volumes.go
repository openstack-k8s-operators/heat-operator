package heatcfnapi

import (
	"github.com/openstack-k8s-operators/heat-operator/pkg/heat"
	corev1 "k8s.io/api/core/v1"
)

// GetVolumes -
func GetVolumes(parentName string, name string) []corev1.Volume {
	var config0640AccessMode int32 = 0640

	backupVolumes := []corev1.Volume{
		{
			Name: "config-data-custom",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					DefaultMode: &config0640AccessMode,
					LocalObjectReference: corev1.LocalObjectReference{
						Name: name + "-config-data",
					},
				},
			},
		},
	}

	return append(heat.GetVolumes(parentName), backupVolumes...)
}

// GetInitVolumeMounts - heat Cfn API init task VolumeMounts
func GetInitVolumeMounts() []corev1.VolumeMount {

	customConfVolumeMount := corev1.VolumeMount{
		Name:      "config-data-custom",
		MountPath: "/var/lib/config-data/custom",
		ReadOnly:  true,
	}

	return append(heat.GetInitVolumeMounts(), customConfVolumeMount)
}

// GetVolumeMounts - heat Cfn API VolumeMounts
func GetVolumeMounts() []corev1.VolumeMount {
	return heat.GetVolumeMounts()
}
