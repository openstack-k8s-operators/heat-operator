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

package heatengine

import (
	"fmt"

	heatv1beta1 "github.com/openstack-k8s-operators/heat-operator/api/v1beta1"
	heat "github.com/openstack-k8s-operators/heat-operator/pkg/heat"
	common "github.com/openstack-k8s-operators/lib-common/modules/common"
	affinity "github.com/openstack-k8s-operators/lib-common/modules/common/affinity"
	env "github.com/openstack-k8s-operators/lib-common/modules/common/env"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// ServiceCommand -
	ServiceCommand = "/usr/local/bin/kolla_set_configs && /usr/local/bin/kolla_start"
)

// Deployment func
func Deployment(instance *heatv1beta1.HeatEngine, configHash string, labels map[string]string) *appsv1.Deployment {
	runAsUser := int64(0)

	livenessProbe := &corev1.Probe{
		TimeoutSeconds: 5,
		PeriodSeconds:  5,
	}
	readinessProbe := &corev1.Probe{
		TimeoutSeconds: 5,
		PeriodSeconds:  5,
	}

	args := []string{"-c"}
	if instance.Spec.Debug.Service {
		args = append(args, common.DebugCommand)
		livenessProbe.Exec = &corev1.ExecAction{
			Command: []string{
				"/bin/true",
			},
		}

		readinessProbe.Exec = &corev1.ExecAction{
			Command: []string{
				"/bin/true",
			},
		}
	} else {
		args = append(args, ServiceCommand)

		//
		// https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/
		//
		livenessProbe.Exec = &corev1.ExecAction{
			Command: []string{
				"/usr/bin/pgrep", "-r", "DRST", "heat-engine",
			},
		}
		readinessProbe.Exec = &corev1.ExecAction{
			Command: []string{
				"/usr/bin/pgrep", "-r", "DRST", "heat-engine",
			},
		}
	}

	envVars := map[string]env.Setter{}
	envVars["KOLLA_CONFIG_FILE"] = env.SetValue(KollaConfig)
	envVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")
	envVars["CONFIG_HASH"] = env.SetValue(configHash)

	// Default oslo.service graceful_shutdown_timeout is 60, so align with that
	terminationGracePeriod := int64(60)
	serviceName := fmt.Sprintf("%s-%s", heat.ServiceName, heat.EngineComponent)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Replicas: &instance.Spec.Replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: heat.ServiceAccount,
					Containers: []corev1.Container{
						{
							Name: serviceName,
							Command: []string{
								"/bin/bash",
							},
							Args:  args,
							Image: instance.Spec.ContainerImage,
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: &runAsUser,
							},
							Env:            env.MergeEnvs([]corev1.EnvVar{}, envVars),
							VolumeMounts:   GetVolumeMounts(),
							Resources:      instance.Spec.Resources,
							ReadinessProbe: readinessProbe,
							LivenessProbe:  livenessProbe,
						},
					},
					TerminationGracePeriodSeconds: &terminationGracePeriod,
				},
			},
		},
	}
	deployment.Spec.Template.Spec.Volumes = GetVolumes(heat.ServiceName, instance.Name)
	// If possible two pods of the same service should not
	// run on the same worker node. If this is not possible
	// the get still created on the same worker node.
	deployment.Spec.Template.Spec.Affinity = affinity.DistributePods(
		common.AppSelector,
		[]string{
			serviceName,
		},
		corev1.LabelHostname,
	)
	if instance.Spec.NodeSelector != nil && len(instance.Spec.NodeSelector) > 0 {
		deployment.Spec.Template.Spec.NodeSelector = instance.Spec.NodeSelector
	}

	initContainerDetails := heat.APIDetails{
		ContainerImage:            instance.Spec.ContainerImage,
		DatabaseHost:              instance.Spec.DatabaseHostname,
		DatabaseUser:              instance.Spec.DatabaseUser,
		DatabaseName:              heat.DatabaseName,
		OSPSecret:                 instance.Spec.Secret,
		DBPasswordSelector:        instance.Spec.PasswordSelectors.Database,
		UserPasswordSelector:      instance.Spec.PasswordSelectors.Service,
		AuthEncryptionKeySelector: instance.Spec.PasswordSelectors.AuthEncryptionKey,
		VolumeMounts:              GetInitVolumeMounts(),
		TransportURL:              instance.Spec.TransportURLSecret,
	}
	deployment.Spec.Template.Spec.InitContainers = heat.InitContainer(initContainerDetails)

	return deployment
}
