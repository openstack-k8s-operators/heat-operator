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

package heat

import (
	"fmt"

	heatv1beta1 "github.com/openstack-k8s-operators/heat-operator/api/v1beta1"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CronJobSpec defines the spec for a CronJob
type CronJobSpec struct {
	Name        string
	Schedule    string
	Command     string
	Labels      map[string]string
	Annotations map[string]string
}

// DBPurgeJob creates a CronJob to purge soft-deleted database records
func DBPurgeJob(
	instance *heatv1beta1.Heat,
	cronSpec CronJobSpec,
) *batchv1.CronJob {
	var config0644AccessMode int32 = 0644

	cronCommand := fmt.Sprintf(
		"%s --config-dir /etc/heat/heat.conf.d purge_deleted %d",
		cronSpec.Command,
		instance.Spec.DBPurge.Age,
	)

	args := []string{"-c", cronCommand}

	parallelism := int32(1)
	completions := int32(1)
	backoffLimit := int32(3)
	ttlSecondsAfterFinished := int32(86400) // 24 hours
	successfulJobsHistoryLimit := int32(3)
	failedJobsHistoryLimit := int32(1)

	cronJobVolume := []corev1.Volume{
		{
			Name: "db-purge-config-data",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: &config0644AccessMode,
					SecretName:  instance.Name + "-config-data",
					Items: []corev1.KeyToPath{
						{
							Key:  DefaultsConfigFileName,
							Path: DefaultsConfigFileName,
						},
					},
				},
			},
		},
		{
			Name: "config-data",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: &config0644AccessMode,
					SecretName:  instance.Name + "-config-data",
				},
			},
		},
	}
	cronJobVolumeMounts := []corev1.VolumeMount{
		{
			Name:      "db-purge-config-data",
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

	// add CA cert if defined
	if instance.Spec.HeatAPI.TLS.CaBundleSecretName != "" {
		cronJobVolume = append(cronJobVolume, instance.Spec.HeatAPI.TLS.CreateVolume())
		cronJobVolumeMounts = append(cronJobVolumeMounts, instance.Spec.HeatAPI.TLS.CreateVolumeMounts(nil)...)
	}

	var nodeSelector map[string]string
	if instance.Spec.NodeSelector != nil && len(*instance.Spec.NodeSelector) > 0 {
		nodeSelector = *instance.Spec.NodeSelector
	}

	cronjob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cronSpec.Name,
			Namespace: instance.Namespace,
			Labels:    cronSpec.Labels,
		},
		Spec: batchv1.CronJobSpec{
			Schedule:                   cronSpec.Schedule,
			ConcurrencyPolicy:          batchv1.ForbidConcurrent,
			SuccessfulJobsHistoryLimit: &successfulJobsHistoryLimit,
			FailedJobsHistoryLimit:     &failedJobsHistoryLimit,
			JobTemplate: batchv1.JobTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: cronSpec.Annotations,
					Labels:      cronSpec.Labels,
				},
				Spec: batchv1.JobSpec{
					Parallelism:             &parallelism,
					Completions:             &completions,
					BackoffLimit:            &backoffLimit,
					TTLSecondsAfterFinished: &ttlSecondsAfterFinished,
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  ServiceName + "-dbpurge",
									Image: instance.Spec.HeatEngine.ContainerImage,
									Command: []string{
										"/bin/bash",
									},
									Args:            args,
									VolumeMounts:    cronJobVolumeMounts,
									SecurityContext: GetHeatDBSecurityContext(),
								},
							},
							Volumes:            cronJobVolume,
							RestartPolicy:      corev1.RestartPolicyNever,
							ServiceAccountName: instance.RbacResourceName(),
							NodeSelector:       nodeSelector,
						},
					},
				},
			},
		},
	}
	return cronjob
}
