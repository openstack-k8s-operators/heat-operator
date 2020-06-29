package heat

import (
	comv1 "github.com/openstack-k8s-operators/heat-operator/pkg/apis/heat/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Deployment func
func HeatAPIDeployment(cr *comv1.Heat, cmName string, configHash string) *appsv1.Deployment {
	runAsUser := int64(0)

	labels := map[string]string{
		"app": "heat-api",
	}
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: cr.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Replicas: &cr.Spec.HeatAPIReplicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "heat-operator",
					Containers: []corev1.Container{
						{
							Name:  "heat-api",
							Image: cr.Spec.HeatAPIContainerImage,
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: &runAsUser,
							},
							Env: []corev1.EnvVar{
								{
									Name:  "KOLLA_CONFIG_STRATEGY",
									Value: "COPY_ALWAYS",
								},
								{
									Name:  "CONFIG_HASH",
									Value: configHash,
								},
							},
							VolumeMounts: getHeatAPIVolumeMounts(),
						},
					},
				},
			},
		},
	}
	deployment.Spec.Template.Spec.Volumes = getVolumes("heat")
	return deployment
}

func HeatEngineDeployment(cr *comv1.Heat, cmName string, configHash string) *appsv1.Deployment {
	runAsUser := int64(0)

	labels := map[string]string{
		"app": "heat-engine",
	}
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: cr.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Replicas: &cr.Spec.HeatEngineReplicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "heat-operator",
					Containers: []corev1.Container{
						{
							Name:  "heat-engine",
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
									Name:  "CONFIG_HASH",
									Value: configHash,
								},
							},
							VolumeMounts: getHeatEngineVolumeMounts(),
						},
					},
				},
			},
		},
	}
	deployment.Spec.Template.Spec.Volumes = getVolumes("heat")
	return deployment
}
