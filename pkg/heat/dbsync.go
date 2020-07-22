package heat

import (
	comv1 "github.com/openstack-k8s-operators/heat-operator/pkg/apis/heat/v1"
	util "github.com/openstack-k8s-operators/heat-operator/pkg/util"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type dbCreateOptions struct {
	HeatDatabaseUsername  string
	HeatDatabasePassword  string
	DatabaseAdminPassword string
	DatabaseAdminUsername string
}

// DbSyncJob func
func DbSyncJob(cr *comv1.Heat, cmName string) *batchv1.Job {

	opts := dbCreateOptions{cr.Spec.HeatDatabaseUsername, cr.Spec.HeatDatabasePassword, cr.Spec.DatabaseAdminPassword, cr.Spec.DatabaseAdminUsername}
	runAsUser := int64(0)

	labels := map[string]string{
		"app": "heat",
	}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName + "-db-sync",
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:      "OnFailure",
					ServiceAccountName: "heat-operator",
					Containers: []corev1.Container{
						{
							Name:  "heat-db-sync",
							Image: cr.Spec.HeatEngineContainerImage,
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: &runAsUser,
							},
							Env: []corev1.EnvVar{
								{
									Name:  "KOLLA_CONFIG_STRATEGY",
									Value: "COPY_ALWAYS",
								},
								{
									Name:  "KOLLA_BOOTSTRAP",
									Value: "TRUE",
								},
							},
							VolumeMounts: getHeatAPIVolumeMounts(),
							Command:      []string{"/bin/sh", "-c", "heat-manage --config-file /var/lib/config-data/heat.conf db_sync"},
						},
					},
					InitContainers: []corev1.Container{
						{
							Name:    "heat-db-create",
							Image:   "docker.io/tripleomaster/centos-binary-mariadb:current-tripleo",
							Command: []string{"/bin/sh", "-c", util.ExecuteTemplateFile("db_create.sh", &opts)},
							Env: []corev1.EnvVar{
								{
									Name:  "MYSQL_PWD",
									Value: cr.Spec.DatabaseAdminPassword,
								},
							},
						},
					},
				},
			},
		},
	}
	job.Spec.Template.Spec.Volumes = getVolumes(cmName)
	return job
}
