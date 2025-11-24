/*

"Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package heat

import (
	"strconv"

	heatv1 "github.com/openstack-k8s-operators/heat-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/storage"
	corev1 "k8s.io/api/core/v1"
)

// GetVolumes ...
func GetVolumes(parentName string, name string,
	extraVol []heatv1.HeatExtraVolMounts,
	svc []storage.PropagationType) []corev1.Volume {

	var config0644AccessMode int32 = 0644

	volumes := []corev1.Volume{
		{
			Name: "config-data-custom",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: &config0644AccessMode,
					SecretName:  name + "-config-data",
				},
			},
		},
		{
			Name: "config-data",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: parentName + "-config-data",
				},
			},
		},
	}
	// ExtraMounts
	for _, exv := range extraVol {
		for _, vol := range exv.Propagate(svc) {
			for _, v := range vol.Volumes {
				volumeSource, _ := v.ToCoreVolumeSource()
				convertedVolume := corev1.Volume{
					Name:         v.Name,
					VolumeSource: *volumeSource,
				}
				volumes = append(volumes, convertedVolume)
			}
		}
	}
	return volumes
}

// GetVolumeMounts ...
func GetVolumeMounts(name string,
	extraVol []heatv1.HeatExtraVolMounts,
	svc []storage.PropagationType) []corev1.VolumeMount {
	vm := []corev1.VolumeMount{
		{
			Name:      "config-data",
			MountPath: "/var/lib/kolla/config_files/config.json",
			SubPath:   name + "-config.json",
			ReadOnly:  true,
		},
		{
			Name:      "config-data-custom",
			MountPath: "/etc/heat/heat.conf.d",
			ReadOnly:  true,
		},
		{
			Name:      "config-data",
			MountPath: "/var/lib/config-data/default",
			ReadOnly:  true,
		},
		{
			Name:      "config-data",
			MountPath: "/etc/my.cnf",
			SubPath:   "my.cnf",
			ReadOnly:  true,
		},
	}
	for _, exv := range extraVol {
		for _, vol := range exv.Propagate(svc) {
			vm = append(vm, vol.Mounts...)
		}
	}
	return vm

}

// getDBSyncVolumeMounts ...
func getDBSyncVolumeMounts(
	extraVol []heatv1.HeatExtraVolMounts,
	svc []storage.PropagationType) []corev1.VolumeMount {
	volumeMounts := []corev1.VolumeMount{{
		Name:      "config-data",
		MountPath: "/etc/heat/heat.conf.d/" + DefaultsConfigFileName,
		SubPath:   DefaultsConfigFileName,
		ReadOnly:  true,
	},
		{
			Name:      "config-data",
			MountPath: "/etc/heat/heat.conf.d/" + CustomConfigFileName,
			SubPath:   CustomConfigFileName,
		},
		{
			Name:      "config-data",
			MountPath: "/etc/my.cnf",
			SubPath:   "my.cnf",
			ReadOnly:  true,
		},
	}
	for _, exv := range extraVol {
		for _, vol := range exv.Propagate(svc) {
			volumeMounts = append(volumeMounts, vol.Mounts...)
		}
	}
	return volumeMounts
}

// GetConfigSecretVolumes - Returns a list of volumes associated with a list of Secret names
func GetConfigSecretVolumes(secretNames []string) ([]corev1.Volume, []corev1.VolumeMount) {
	var config0640AccessMode int32 = 0640
	secretVolumes := []corev1.Volume{}
	secretMounts := []corev1.VolumeMount{}

	for idx, secretName := range secretNames {
		secretVol := corev1.Volume{
			Name: secretName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  secretName,
					DefaultMode: &config0640AccessMode,
				},
			},
		}
		secretMount := corev1.VolumeMount{
			Name: secretName,
			// Each secret needs its own MountPath
			MountPath: "/var/lib/config-data/secret-" + strconv.Itoa(idx),
			ReadOnly:  true,
		}
		secretVolumes = append(secretVolumes, secretVol)
		secretMounts = append(secretMounts, secretMount)
	}

	return secretVolumes, secretMounts
}
