package heat

import (
	corev1 "k8s.io/api/core/v1"
)

// common Heat Volumes
func getVolumes(name string) []corev1.Volume {

	return []corev1.Volume{

		{
			Name: "kolla-config-heat-api",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: name,
					},
					Items: []corev1.KeyToPath{
						{
							Key:  "kolla_config_heat_api.json",
							Path: "config.json",
						},
					},
				},
			},
		},
		{
			Name: "kolla-config-heat-engine",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: name,
					},
					Items: []corev1.KeyToPath{
						{
							Key:  "kolla_config_heat_engine.json",
							Path: "config.json",
						},
					},
				},
			},
		},
		{
			Name: "config-data",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: name,
					},
					Items: []corev1.KeyToPath{
						{
							Key:  "heat.conf",
							Path: "heat.conf",
						},
						{
							Key:  "10-heat_api_wsgi.conf",
							Path: "10-heat_api_wsgi.conf",
						},
					},
				},
			},
		},
	}

}

// Heat API VolumeMounts
func getHeatAPIVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			MountPath: "/var/lib/config-data",
			ReadOnly:  true,
			Name:      "config-data",
		},
		{
			MountPath: "/var/lib/kolla/config_files",
			ReadOnly:  true,
			Name:      "kolla-config-heat-api",
		},
	}

}

// Heat Engine VolumeMounts
func getHeatEngineVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			MountPath: "/var/lib/config-data",
			ReadOnly:  true,
			Name:      "config-data",
		},
		{
			MountPath: "/var/lib/kolla/config_files",
			ReadOnly:  true,
			Name:      "kolla-config-heat-engine",
		},
	}

}
