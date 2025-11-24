/*
Copyright 2023.

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
	"fmt"
	"os"

	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	heatv1 "github.com/openstack-k8s-operators/heat-operator/api/v1beta1"
)

var _ = Describe("Heat Webhook", func() {
	var heatName types.NamespacedName

	BeforeEach(func() {
		heatName = types.NamespacedName{
			Name:      "heat",
			Namespace: namespace,
		}
		err := os.Setenv("OPERATOR_TEMPLATES", "../../templates")
		Expect(err).NotTo(HaveOccurred())
	})

	When("A Heat instance is created without container images", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateHeat(heatName, GetDefaultHeatSpec()))
		})

		It("should have the defaults initialized by webhook", func() {
			Heat := GetHeat(heatName)
			Expect(Heat.Spec.HeatAPI.ContainerImage).Should(Equal(
				heatv1.HeatAPIContainerImage,
			))
			Expect(Heat.Spec.HeatCfnAPI.ContainerImage).Should(Equal(
				heatv1.HeatCfnAPIContainerImage,
			))
			Expect(Heat.Spec.HeatEngine.ContainerImage).Should(Equal(
				heatv1.HeatEngineContainerImage,
			))
		})
	})

	When("A Heat instance is created with container images", func() {
		BeforeEach(func() {
			heatSpec := GetDefaultHeatSpec()
			heatSpec["heatAPI"] = map[string]any{
				"containerImage": "api-container-image",
			}
			heatSpec["heatCfnAPI"] = map[string]any{
				"containerImage": "cfnapi-container-image",
			}
			heatSpec["heatEngine"] = map[string]any{
				"containerImage": "engine-container-image",
			}
			DeferCleanup(th.DeleteInstance, CreateHeat(heatName, heatSpec))
		})

		It("should use the given values", func() {
			Heat := GetHeat(heatName)
			Expect(Heat.Spec.HeatAPI.ContainerImage).Should(Equal(
				"api-container-image",
			))
			Expect(Heat.Spec.HeatCfnAPI.ContainerImage).Should(Equal(
				"cfnapi-container-image",
			))
			Expect(Heat.Spec.HeatEngine.ContainerImage).Should(Equal(
				"engine-container-image",
			))
		})
	})

	When("The DatabaseInstance is changed for existing deployments", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateHeat(heatName, GetDefaultHeatSpec()))
		})

		It("Should be blocked by the webhook", func() {
			Eventually(func(_ Gomega) string {
				instance := GetHeat(heatName)
				instance.Spec.DatabaseInstance = "new-database"
				err := th.K8sClient.Update(th.Ctx, instance)
				return fmt.Sprintf("%s", err)
			}).Should(ContainSubstring("Changing the DatabaseInstance is not supported for existing deployments"))
		})
	})

	It("rejects with wrong HeatAPI service override endpoint type", func() {
		spec := GetDefaultHeatSpec()
		apiSpec := GetDefaultHeatAPISpec()
		apiSpec["override"] = map[string]any{
			"service": map[string]any{
				"internal": map[string]any{},
				"wrooong":  map[string]any{},
			},
		}
		spec["heatAPI"] = apiSpec

		raw := map[string]any{
			"apiVersion": "heat.openstack.org/v1beta1",
			"kind":       "Heat",
			"metadata": map[string]any{
				"name":      heatName.Name,
				"namespace": heatName.Namespace,
			},
			"spec": spec,
		}

		unstructuredObj := &unstructured.Unstructured{Object: raw}
		_, err := controllerutil.CreateOrPatch(
			th.Ctx, th.K8sClient, unstructuredObj, func() error { return nil })
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(
			ContainSubstring(
				"invalid: spec.heatAPI.override.service[wrooong]: " +
					"Invalid value: \"wrooong\": invalid endpoint type: wrooong"),
		)
	})

	It("rejects with wrong HeatCfnAPI service override endpoint type", func() {
		spec := GetDefaultHeatSpec()
		apiSpec := GetDefaultHeatAPISpec()
		apiSpec["override"] = map[string]any{
			"service": map[string]any{
				"internal": map[string]any{},
				"wrooong":  map[string]any{},
			},
		}
		spec["heatCfnAPI"] = apiSpec

		raw := map[string]any{
			"apiVersion": "heat.openstack.org/v1beta1",
			"kind":       "Heat",
			"metadata": map[string]any{
				"name":      heatName.Name,
				"namespace": heatName.Namespace,
			},
			"spec": spec,
		}

		unstructuredObj := &unstructured.Unstructured{Object: raw}
		_, err := controllerutil.CreateOrPatch(
			th.Ctx, th.K8sClient, unstructuredObj, func() error { return nil })
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(
			ContainSubstring(
				"invalid: spec.heatCfnAPI.override.service[wrooong]: " +
					"Invalid value: \"wrooong\": invalid endpoint type: wrooong"),
		)
	})

	When("A user provides the skip-validations annotation", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateHeat(heatName, GetDefaultHeatSpec()))
		})

		It("should skip the validation when the DatabaseInstance is updated", func() {
			Eventually(func(g Gomega) {
				instance := GetHeat(heatName)
				instance.SetAnnotations(map[string]string{
					heatv1.HeatDatabaseMigrationAnnotation: "true",
				})
				instance.Spec.DatabaseInstance = "new-database"
				g.Expect(th.K8sClient.Update(th.Ctx, instance)).Should(Succeed())
			}).Should(Succeed())
		})
	})

	When("The DatabaseInstance is changed for existing deployments from null to something valid", func() {
		BeforeEach(func() {

			heatSpecNullDBInstance := map[string]any{
				"databaseInstance": "",
				"secret":           SecretName,
				"heatEngine":       GetDefaultHeatEngineSpec(),
				"heatAPI":          GetDefaultHeatAPISpec(),
				"heatCfnAPI":       GetDefaultHeatCFNAPISpec(),
			}
			DeferCleanup(th.DeleteInstance, CreateHeat(heatName, heatSpecNullDBInstance))
		})

		It("Should be accepted by the webhook", func() {
			Eventually(func(g Gomega) {
				instance := GetHeat(heatName)
				instance.Spec.DatabaseInstance = "new-database"
				g.Expect(th.K8sClient.Update(th.Ctx, instance)).Should(Succeed())
			}).Should(Succeed())
		})
	})

	DescribeTable("rejects wrong topology for",
		func(serviceNameFunc func() (string, string)) {

			component, errorPath := serviceNameFunc()
			expectedErrorMessage := fmt.Sprintf("spec.%s.namespace: Invalid value: \"namespace\": Customizing namespace field is not supported", errorPath)

			spec := GetDefaultHeatSpec()
			// API, CfnApi and Engine
			if component != "top-level" {
				spec[component] = map[string]any{
					"topologyRef": map[string]any{
						"name":      "bar",
						"namespace": "foo",
					},
				}
				// top-level
			} else {
				spec["topologyRef"] = map[string]any{
					"name":      "bar",
					"namespace": "foo",
				}
			}
			// Build Heat CR
			raw := map[string]any{
				"apiVersion": "heat.openstack.org/v1beta1",
				"kind":       "Heat",
				"metadata": map[string]any{
					"name":      heatName.Name,
					"namespace": heatName.Namespace,
				},
				"spec": spec,
			}
			unstructuredObj := &unstructured.Unstructured{Object: raw}
			_, err := controllerutil.CreateOrPatch(
				th.Ctx, th.K8sClient, unstructuredObj, func() error { return nil })
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(
				ContainSubstring(expectedErrorMessage))
		},
		Entry("top-level topologyRef", func() (string, string) {
			return "top-level", "topologyRef"
		}),
		Entry("heatAPI topologyRef", func() (string, string) {
			component := "heatAPI"
			return component, fmt.Sprintf("%s.topologyRef", component)
		}),
		Entry("heatCfnAPI topologyRef", func() (string, string) {
			component := "heatCfnAPI"
			return component, fmt.Sprintf("%s.topologyRef", component)
		}),
		Entry("heatEngine topologyRef", func() (string, string) {
			component := "heatEngine"
			return component, fmt.Sprintf("%s.topologyRef", component)
		}),
	)
})
