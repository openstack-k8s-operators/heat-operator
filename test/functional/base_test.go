/*
Copyright 2022.

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

package functional_test

import (
	. "github.com/onsi/gomega" //revive:disable:dot-imports

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	heatv1 "github.com/openstack-k8s-operators/heat-operator/api/v1beta1"
	rabbitmqv1 "github.com/openstack-k8s-operators/infra-operator/apis/rabbitmq/v1beta1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
)

func GetDefaultHeatSpec() map[string]any {
	return map[string]any{
		"databaseInstance": "openstack",
		"secret":           SecretName,
		"heatEngine":       GetDefaultHeatEngineSpec(),
		"heatAPI":          GetDefaultHeatAPISpec(),
		"heatCfnAPI":       GetDefaultHeatCFNAPISpec(),
		"passwordSelectors": map[string]any{
			"AuthEncryptionKey":        "HeatAuthEncryptionKey",
			"StackDomainAdminPassword": "HeatStackDomainAdminPassword",
			"Service":                  "HeatPassword",
		},
	}
}

func GetDefaultHeatAPISpec() map[string]any {
	return map[string]any{}
}

func GetDefaultHeatEngineSpec() map[string]any {
	return map[string]any{}
}

func GetDefaultHeatCFNAPISpec() map[string]any {
	return map[string]any{}
}

func CreateHeat(name types.NamespacedName, spec map[string]any) client.Object {

	raw := map[string]any{
		"apiVersion": "heat.openstack.org/v1beta1",
		"kind":       "Heat",
		"metadata": map[string]any{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return th.CreateUnstructured(raw)
}

func GetHeat(name types.NamespacedName) *heatv1.Heat {
	instance := &heatv1.Heat{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func CreateHeatSecret(namespace string, name string) *corev1.Secret {
	return th.CreateSecret(
		types.NamespacedName{Namespace: namespace, Name: name},
		map[string][]byte{
			"HeatPassword":                 []byte("12345678"),
			"HeatAuthEncryptionKey":        []byte("1234567812345678123456781212345678345678"),
			"HeatStackDomainAdminPassword": []byte("12345678"),
		},
	)
}

func CreateHeatMessageBusSecret(namespace string, name string) *corev1.Secret {
	return th.CreateSecret(
		types.NamespacedName{Namespace: namespace, Name: name},
		map[string][]byte{
			"transport_url": []byte("rabbit://fake"),
		},
	)
}

func HeatConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetHeat(name)
	return instance.Status.Conditions
}

func GetCronJob(name types.NamespacedName) *batchv1.CronJob {
	cron := &batchv1.CronJob{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, cron)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return cron
}

// AssertCronJobDoesNotExist ensures the CronJob resource does not exist in a
// k8s cluster.
func AssertCronJobDoesNotExist(name types.NamespacedName) {
	instance := &batchv1.CronJob{}
	Eventually(func(g Gomega) {
		err := k8sClient.Get(ctx, name, instance)
		g.Expect(k8s_errors.IsNotFound(err)).To(BeTrue())
	}, timeout, interval).Should(Succeed())
}

func GetHeatSpecWithRabbitMQ(rabbitmqUser *string, rabbitmqVHost *string) map[string]any {
	spec := GetDefaultHeatSpec()
	if rabbitmqUser != nil || rabbitmqVHost != nil {
		messagingBus := map[string]any{}
		if rabbitmqUser != nil {
			messagingBus["user"] = *rabbitmqUser
		}
		if rabbitmqVHost != nil {
			messagingBus["vhost"] = *rabbitmqVHost
		}
		spec["messagingBus"] = messagingBus
	}
	return spec
}

func GetHeatSpecWithNotificationsBus(notificationsCluster *string, notificationsUser *string, notificationsVHost *string) map[string]any {
	spec := GetDefaultHeatSpec()
	if notificationsCluster != nil || notificationsUser != nil || notificationsVHost != nil {
		notificationsBus := map[string]any{}
		if notificationsCluster != nil {
			notificationsBus["cluster"] = *notificationsCluster
		}
		if notificationsUser != nil {
			notificationsBus["user"] = *notificationsUser
		}
		if notificationsVHost != nil {
			notificationsBus["vhost"] = *notificationsVHost
		}
		spec["notificationsBus"] = notificationsBus
	}
	return spec
}

func GetTransportURL(name types.NamespacedName) *rabbitmqv1.TransportURL {
	instance := &rabbitmqv1.TransportURL{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout*5, interval).Should(Succeed())
	return instance
}
