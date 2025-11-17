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
	heatv1beta1 "github.com/openstack-k8s-operators/heat-operator/api/v1beta1"

	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DBSyncCommand
const (
	DBSyncCommand = "/usr/bin/heat-manage --config-dir /etc/heat/heat.conf.d db_sync"
)

// DBSyncJob function
func DBSyncJob(
	instance *heatv1beta1.Heat,
	labels map[string]string,
) *batchv1.Job {

	args := []string{"-c", DBSyncCommand}

	envVars := map[string]env.Setter{}
	envVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")
	envVars["KOLLA_BOOTSTRAP"] = env.SetValue("true")

	volumes := GetVolumes(ServiceName, instance.Name,
		instance.Spec.ExtraMounts, DbsyncPropagation)
	volumeMounts := getDBSyncVolumeMounts(instance.Spec.ExtraMounts, DbsyncPropagation)
	secretVolumes, secretMounts := GetConfigSecretVolumes(instance.Spec.CustomServiceConfigSecrets)
	volumes = append(volumes, secretVolumes...)
	volumeMounts = append(volumeMounts, secretMounts...)

	// add CA cert if defined
	if instance.Spec.HeatAPI.TLS.CaBundleSecretName != "" {
		volumes = append(volumes, instance.Spec.HeatAPI.TLS.CreateVolume())
		volumeMounts = append(volumeMounts, instance.Spec.HeatAPI.TLS.CreateVolumeMounts(nil)...)
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
							Args:            args,
							Image:           instance.Spec.HeatEngine.ContainerImage,
							SecurityContext: GetHeatDBSecurityContext(),
							Env:             env.MergeEnvs([]corev1.EnvVar{}, envVars),
							VolumeMounts:    volumeMounts,
						},
					},
					Volumes: volumes,
				},
			},
		},
	}

	if instance.Spec.NodeSelector != nil {
		job.Spec.Template.Spec.NodeSelector = *instance.Spec.NodeSelector
	}

	return job
}
