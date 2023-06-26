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

package functional

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Heat webhook", func() {

	var heatName types.NamespacedName

	BeforeEach(func() {
		heatName = types.NamespacedName{
			Name:      "heat",
			Namespace: namespace,
		}

		err := os.Setenv("OPERATOR_TEMPLATES", "../../templates")
		Expect(err).NotTo(HaveOccurred())
	})

	When("databaseInstance is being updated", func() {
		BeforeEach(func() {
			DeferCleanup(DeleteInstance, CreateHeat(heatName, GetDefaultHeatSpec()))
		})

		It("should have created a Heat", func() {
			Eventually(func(g Gomega) {
				GetHeat(heatName)
			}, timeout, interval).Should(Succeed())
		})

		It("should be blocked by the webhook and fail", func() {
			heat := GetHeat(heatName)
			_, err := controllerutil.CreateOrPatch(
				th.Ctx, th.K8sClient, heat, func() error {
					heat.Spec.DatabaseInstance = "changed"
					return nil
				})
			Expect(err).To(HaveOccurred())
		})
	})
})
