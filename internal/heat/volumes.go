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
	heatv1 "github.com/openstack-k8s-operators/heat-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/storage"
	corev1 "k8s.io/api/core/v1"
)

var configMode int32 = 0440

// GetVolumes ...
func GetVolumes(parentName string, name string,
	extraVol []heatv1.HeatExtraVolMounts,
	svc []storage.PropagationType) []corev1.Volume {

	volumes := []corev1.Volume{
		{
			Name: "config-data-custom",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: &configMode,
					SecretName:  name + "-config-data",
				},
			},
		},
		{
			Name: "config-data",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: &configMode,
					SecretName:  parentName + "-config-data",
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
func GetVolumeMounts(
	extraVol []heatv1.HeatExtraVolMounts,
	svc []storage.PropagationType) []corev1.VolumeMount {
	vm := []corev1.VolumeMount{
		{
			Name:      "config-data-custom",
			MountPath: "/etc/heat/heat.conf.d",
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
