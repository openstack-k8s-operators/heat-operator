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

package functional

import (
	"fmt"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/openstack-k8s-operators/lib-common/modules/test/helpers"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
)

var _ = Describe("Heat controller", func() {

	var secret *corev1.Secret
	var heatTransportURLName types.NamespacedName
	var heatName types.NamespacedName

	BeforeEach(func() {

		heatName = types.NamespacedName{
			Name:      "heat",
			Namespace: namespace,
		}
		heatTransportURLName = types.NamespacedName{
			Namespace: namespace,
			Name:      heatName.Name + "-heat-transport",
		}

		err := os.Setenv("OPERATOR_TEMPLATES", "../../templates")
		Expect(err).NotTo(HaveOccurred())
	})

	When("A Heat instance is created", func() {
		BeforeEach(func() {
			DeferCleanup(DeleteInstance, CreateHeat(heatName, GetDefaultHeatSpec()))
		})

		It("should have the Spec fields initialized", func() {
			Heat := GetHeat(heatName)
			Expect(Heat.Spec.DatabaseInstance).Should(Equal("openstack"))
			Expect(Heat.Spec.DatabaseUser).Should(Equal("heat"))
			Expect(Heat.Spec.RabbitMqClusterName).Should(Equal("rabbitmq"))
			Expect(Heat.Spec.ServiceUser).Should(Equal("heat"))
		})

		It("should have the Status fields initialized", func() {
			Heat := GetHeat(heatName)
			Expect(Heat.Status.Hash).To(BeEmpty())
			Expect(Heat.Status.DatabaseHostname).To(Equal(""))
			Expect(Heat.Status.TransportURLSecret).To(Equal(""))
			Expect(Heat.Status.APIEndpoints).To(BeEmpty())
			Expect(Heat.Status.ReadyCount).To(Equal(int32(0)))
		})

		It("should have Unknown Conditions initialized as transporturl not created", func() {
			th.ExpectCondition(
				heatName,
				ConditionGetterFunc(HeatConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionUnknown,
			)
		})

		It("should have a finalizer", func() {
			// the reconciler loop adds the finalizer so we have to wait for
			// it to run
			Eventually(func() []string {
				return GetHeat(heatName).Finalizers
			}, timeout, interval).Should(ContainElement("Heat"))
		})

		It("should not create a config map", func() {
			Eventually(func() []corev1.ConfigMap {
				return th.ListConfigMaps(fmt.Sprintf("%s-%s", heatName.Name, "config-data")).Items
			}, timeout, interval).Should(BeEmpty())
		})
	})

	When("the proper secret is provided and TransportURL Created", func() {
		BeforeEach(func() {
			DeferCleanup(DeleteInstance, CreateHeat(heatName, GetDefaultHeatSpec()))
			keystoneAPIName := th.CreateKeystoneAPI(namespace)
			DeferCleanup(th.DeleteKeystoneAPI, keystoneAPIName)
			keystoneAPI := th.GetKeystoneAPI(keystoneAPIName)
			keystoneAPI.Status.APIEndpoints["internal"] = "http://keystone-internal-openstack.testing"
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Status().Update(ctx, keystoneAPI.DeepCopy())).Should(Succeed())
			}, timeout, interval).Should(Succeed())

			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rabbitmq-secret",
					Namespace: namespace,
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, secret)
			DeferCleanup(
				k8sClient.Delete, ctx, CreateHeatSecret(namespace, SecretName))
			DeferCleanup(
				DeleteDBService,
				CreateDBService(
					namespace,
					GetHeat(heatName).Spec.DatabaseInstance,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			th.SimulateTransportURLReady(heatTransportURLName)
			th.SimulateMariaDBDatabaseCompleted(heatName)
		})

		It("should not create a config map", func() {
			Eventually(func() []corev1.ConfigMap {
				return th.ListConfigMaps(fmt.Sprintf("%s-%s", heatName.Name, "config-data")).Items
			}, timeout, interval).Should(BeEmpty())
		})
	})

	When("keystoneAPI instance is not available", func() {
		BeforeEach(func() {
			DeferCleanup(DeleteInstance, CreateHeat(heatName, GetDefaultHeatSpec()))
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      SecretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"HeatPassword": []byte("12345678"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, secret)
			th.SimulateTransportURLReady(heatTransportURLName)
		})
		It("should not create a config map", func() {
			Eventually(func() []corev1.ConfigMap {
				return th.ListConfigMaps(fmt.Sprintf("%s-%s", heatName.Name, "config-data")).Items
			}, timeout, interval).Should(BeEmpty())
		})
	})

	When("keystoneAPI instance is available", func() {
		BeforeEach(func() {
			DeferCleanup(DeleteInstance, CreateHeat(heatName, GetDefaultHeatSpec()))
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      SecretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"HeatPassword": []byte("12345678"),
				},
			}
			rmqSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rabbitmq-secret",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"transport_url": []byte("rabbit://fake"),
				},
			}
			Expect(k8sClient.Create(ctx, rmqSecret)).Should(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, rmqSecret)
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, secret)
			th.SimulateTransportURLReady(heatTransportURLName)
		})

		It("should create a ConfigMap for heat.conf with the heat_domain_admin config option set", func() {
			keystoneAPI := CreateKeystoneAPI(namespace)
			DeferCleanup(DeleteKeystoneAPI, keystoneAPI)

			configataCM := types.NamespacedName{
				Namespace: heatName.Namespace,
				Name:      fmt.Sprintf("%s-%s", heatName.Name, "config-data"),
			}

			Eventually(func() corev1.ConfigMap {
				return *th.GetConfigMap(configataCM)
			}, timeout, interval).ShouldNot(BeNil())

			//keystone := GetKeystoneAPI(keystoneAPI)
			Expect(th.GetConfigMap(configataCM).Data["heat.conf"]).Should(
				ContainSubstring("stack_domain_admin = heat_stack_domain_admin"))

			th.ExpectCondition(
				heatName,
				ConditionGetterFunc(HeatConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				heatName,
				ConditionGetterFunc(HeatConditionGetter),
				condition.DBReadyCondition,
				corev1.ConditionFalse,
			)
		})
	})
})
