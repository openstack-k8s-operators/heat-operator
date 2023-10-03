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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"

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
			heatSpec["heatAPI"] = map[string]interface{}{
				"containerImage": "api-container-image",
			}
			heatSpec["heatCfnAPI"] = map[string]interface{}{
				"containerImage": "cfnapi-container-image",
			}
			heatSpec["heatEngine"] = map[string]interface{}{
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
			Eventually(func(g Gomega) string {
				instance := GetHeat(heatName)
				instance.Spec.DatabaseInstance = "new-database"
				err := th.K8sClient.Update(th.Ctx, instance)
				return fmt.Sprintf("%s", err)
			}).Should(ContainSubstring("Changing the DatabaseInstance is not supported for existing deployments"))
		})
	})
})
