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

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/openstack-k8s-operators/lib-common/modules/test/helpers"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	heatv1 "github.com/openstack-k8s-operators/heat-operator/api/v1beta1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
)

var _ = Describe("Heat controller", func() {

	var namespace string
	var secret *corev1.Secret
	var heatTransportURLName types.NamespacedName
	var heatName types.NamespacedName

	BeforeEach(func() {
		mockCtrl := gomock.NewController(GinkgoT())
		defer mockCtrl.Finish()

		mockOkoOpenStack := NewMockOkoOpenStack(mockCtrl)
		expected := "blahUserID"
		mockOkoOpenStack.EXPECT().CreateUser(gomock.Any(), gomock.Any()).Return(expected, nil)
		//domainExpected := "blahDomainID"
		//mockOkoOpenStack.EXPECT().CreateDomain(gomock.Any(), gomock.Any()).Return(domainExpected, nil)
		// NOTE(gibi): We need to create a unique namespace for each test run
		// as namespaces cannot be deleted in a locally running envtest. See
		// https://book.kubebuilder.io/reference/envtest.html#namespace-usage-limitation
		namespace = uuid.New().String()
		th.CreateNamespace(namespace)
		// We still request the delete of the Namespace to properly cleanup if
		// we run the test in an existing cluster.
		DeferCleanup(th.DeleteNamespace, namespace)
		// lib-common uses OPERATOR_TEMPLATES env var to locate the "templates"
		// directory of the operator. We need to set them otherwise lib-common
		// will fail to generate the ConfigMap as it does not find common.sh
		err := os.Setenv("OPERATOR_TEMPLATES", "../../templates")
		Expect(err).NotTo(HaveOccurred())

		//name := fmt.Sprintf("heat-%s", uuid.New().String())
		name := "heat"
		heatTransportURLName = types.NamespacedName{
			Namespace: namespace,
			Name:      name + "-heat-transport",
		}

		heatName = CreateHeat(namespace, name, GetDefaultHeatSpec())
		DeferCleanup(DeleteHeat, heatName)
	})

	When("A Heat instance is created", func() {
		It("should have the Spec fields initialized", func() {
			Heat := GetHeat(heatName)
			Expect(Heat.Spec.DatabaseInstance).Should(Equal("test-heat-db-instance"))
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

	When("an unrelated secret is provided", func() {
		It("should remain in a state of waiting for the proper secret", func() {
			SimulateTransportURLReady(heatTransportURLName)
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "an-unrelated-secret",
					Namespace: namespace,
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, secret)

			th.ExpectCondition(
				heatName,
				ConditionGetterFunc(HeatConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				heatName,
				ConditionGetterFunc(HeatConditionGetter),
				heatv1.HeatRabbitMqTransportURLReadyCondition,
				corev1.ConditionFalse,
			)

		})
		It("should not create a config map", func() {
			Eventually(func() []corev1.ConfigMap {
				return th.ListConfigMaps(fmt.Sprintf("%s-%s", heatName.Name, "config-data")).Items
			}, timeout, interval).Should(BeEmpty())
		})
	})

	When("the proper secret is provided and TransportURL Created", func() {
		BeforeEach(func() {
			SimulateTransportURLReady(heatTransportURLName)

			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      heatTransportURLName.Name,
					Namespace: namespace,
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, secret)
		})

		It("should be in a state of having the TransportURL ready", func() {
			th.ExpectCondition(
				heatName,
				ConditionGetterFunc(HeatConditionGetter),
				heatv1.HeatRabbitMqTransportURLReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("should not create a config map", func() {
			Eventually(func() []corev1.ConfigMap {
				return th.ListConfigMaps(fmt.Sprintf("%s-%s", heatName.Name, "config-data")).Items
			}, timeout, interval).Should(BeEmpty())
		})
	})

	When("keystoneAPI instance is not available", func() {
		BeforeEach(func() {
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
			SimulateTransportURLReady(heatTransportURLName)
		})
		It("should not create a config map", func() {
			Eventually(func() []corev1.ConfigMap {
				return th.ListConfigMaps(fmt.Sprintf("%s-%s", heatName.Name, "config-data")).Items
			}, timeout, interval).Should(BeEmpty())
		})
	})

	When("keystoneAPI instance is available", func() {
		BeforeEach(func() {
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
					Name:      heatTransportURLName.Name,
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
			SimulateTransportURLReady(heatTransportURLName)
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
				ContainSubstring("stack_domain_admin=heat_stack_domain_admin"))

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

		When("DB is created", func() {
			BeforeEach(func() {
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
				SimulateTransportURLReady(heatTransportURLName)
				DeferCleanup(DeleteKeystoneAPI, CreateKeystoneAPI(namespace))
			})
			It("Should set DBReady Condition and set DatabaseHostname Status when DB is Created", func() {
				SimulateMariaDBDatabaseCompleted(types.NamespacedName{Namespace: namespace, Name: "heat"})
				th.SimulateJobSuccess(types.NamespacedName{Namespace: namespace, Name: "heat-db-sync"})
				Heat := GetHeat(heatName)
				Expect(Heat.Status.DatabaseHostname).To(Equal("hostname-for-" + Heat.Spec.DatabaseInstance))
				th.ExpectCondition(
					heatName,
					ConditionGetterFunc(HeatConditionGetter),
					condition.DBReadyCondition,
					corev1.ConditionTrue,
				)
				th.ExpectCondition(
					heatName,
					ConditionGetterFunc(HeatConditionGetter),
					condition.DBSyncReadyCondition,
					corev1.ConditionFalse,
				)
			})
		})

		When("Keystone Resources are created", func() {
			BeforeEach(func() {
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
				SimulateTransportURLReady(heatTransportURLName)
				DeferCleanup(DeleteKeystoneAPI, CreateKeystoneAPI(namespace))
				SimulateMariaDBDatabaseCompleted(types.NamespacedName{Namespace: namespace, Name: "heat"})
				th.SimulateJobSuccess(types.NamespacedName{Namespace: namespace, Name: "heat-db-sync"})
				SimulateKeystoneServiceReady(types.NamespacedName{Namespace: namespace, Name: "heat"})
				SimulateKeystoneEndpointReady(types.NamespacedName{Namespace: namespace, Name: "heat"})
			})
			It("Should set ExposeServiceReadyCondition Condition", func() {

				th.ExpectCondition(
					heatName,
					ConditionGetterFunc(HeatConditionGetter),
					condition.ExposeServiceReadyCondition,
					corev1.ConditionTrue,
				)
			})

			It("Assert Services are created", func() {
				AssertServiceExists(types.NamespacedName{Namespace: namespace, Name: "heat-public"})
				AssertServiceExists(types.NamespacedName{Namespace: namespace, Name: "heat-internal"})
			})

			It("Assert Routes are created", func() {
				AssertRouteExists(types.NamespacedName{Namespace: namespace, Name: "heat-public"})
			})

			It("Endpoints are created", func() {

				th.ExpectCondition(
					heatName,
					ConditionGetterFunc(HeatConditionGetter),
					condition.KeystoneEndpointReadyCondition,
					corev1.ConditionTrue,
				)
				keystoneEndpoint := GetKeystoneEndpoint(types.NamespacedName{Namespace: namespace, Name: "heat"})
				endpoints := keystoneEndpoint.Spec.Endpoints
				regexp := `http:.*:?\d*$`
				Expect(endpoints).To(HaveKeyWithValue("public", MatchRegexp(regexp)))
				Expect(endpoints).To(HaveKeyWithValue("internal", MatchRegexp(regexp)))
			})

			It("Deployment is created as expected", func() {
				deployment := th.GetDeployment(
					types.NamespacedName{
						Namespace: heatName.Namespace,
						Name:      "heat",
					},
				)
				Expect(int(*deployment.Spec.Replicas)).To(Equal(1))
				Expect(deployment.Spec.Template.Spec.Volumes).To(HaveLen(5))
				Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))

				container := deployment.Spec.Template.Spec.Containers[0]
				Expect(container.LivenessProbe.HTTPGet.Port.IntVal).To(Equal(int32(9696)))
				Expect(container.ReadinessProbe.HTTPGet.Port.IntVal).To(Equal(int32(9696)))
				Expect(container.VolumeMounts).To(HaveLen(4))
				Expect(container.Image).To(Equal("test-heat-container-image"))

				Expect(container.LivenessProbe.HTTPGet.Port.IntVal).To(Equal(int32(9696)))
				Expect(container.ReadinessProbe.HTTPGet.Port.IntVal).To(Equal(int32(9696)))
			})
		})

		When("Heat CR is deleted", func() {
			BeforeEach(func() {
				SimulateTransportURLReady(heatTransportURLName)
			})

			It("removes the Config MAP", func() {
				keystoneAPI := CreateKeystoneAPI(namespace)
				DeferCleanup(DeleteKeystoneAPI, keystoneAPI)
				configataCM := types.NamespacedName{
					Namespace: heatName.Namespace,
					Name:      fmt.Sprintf("%s-%s", heatName.Name, "config-data"),
				}

				scriptsCM := types.NamespacedName{
					Namespace: heatName.Namespace,
					Name:      fmt.Sprintf("%s-%s", heatName.Name, "scripts"),
				}

				Eventually(func() corev1.ConfigMap {
					return *th.GetConfigMap(configataCM)
				}, timeout, interval).ShouldNot(BeNil())
				Eventually(func() corev1.ConfigMap {
					return *th.GetConfigMap(scriptsCM)
				}, timeout, interval).ShouldNot(BeNil())

				DeleteHeat(heatName)

				Eventually(func() []corev1.ConfigMap {
					return th.ListConfigMaps(configataCM.Name).Items
				}, timeout, interval).Should(BeEmpty())
				Eventually(func() []corev1.ConfigMap {
					return th.ListConfigMaps(scriptsCM.Name).Items
				}, timeout, interval).Should(BeEmpty())
			})
		})
	})
})
