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
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	heatv1 "github.com/openstack-k8s-operators/heat-operator/api/v1beta1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
)

func CreateUnstructured(rawObj map[string]interface{}) *unstructured.Unstructured {
	logger.Info("Creating", "raw", rawObj)
	unstructuredObj := &unstructured.Unstructured{Object: rawObj}
	_, err := controllerutil.CreateOrPatch(
		ctx, k8sClient, unstructuredObj, func() error { return nil })
	Expect(err).ShouldNot(HaveOccurred())
	return unstructuredObj
}

func GetDefaultHeatSpec() map[string]interface{} {
	return map[string]interface{}{
		"databaseInstance": "openstack",
		"secret":           SecretName,
		"heatEngine":       GetDefaultHeatEngineSpec(),
		"heatAPI":          GetDefaultHeatAPISpec(),
		"heatCfnAPI":       GetDefaultHeatCFNAPISpec(),
		"passwordSelectors": heatv1.PasswordSelector{
			Database:          "HeatDatabasePassword",
			Service:           "HeatPassword",
			AuthEncryptionKey: "HeatAuthEncryptionKey",
		},
	}
}

func GetDefaultHeatAPISpec() map[string]interface{} {
	return map[string]interface{}{
		"replicas":       1,
		"containerImage": "quay.io/podified-antelope-centos9/openstack-heat-api:current-podified",
	}
}

func GetDefaultHeatEngineSpec() map[string]interface{} {
	return map[string]interface{}{
		"replicas":       1,
		"containerImage": "quay.io/podified-antelope-centos9/openstack-heat-engine:current-podified",
	}
}

func GetDefaultHeatCFNAPISpec() map[string]interface{} {
	return map[string]interface{}{
		"replicas":       1,
		"containerImage": "quay.io/podified-antelope-centos9/openstack-heat-cfn:current-podified",
	}
}

func CreateHeat(name types.NamespacedName, spec map[string]interface{}) client.Object {

	raw := map[string]interface{}{
		"apiVersion": "heat.openstack.org/v1beta1",
		"kind":       "Heat",
		"metadata": map[string]interface{}{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return CreateUnstructured(raw)

	// return types.NamespacedName{Name: HeatName, Namespace: namespace}
}

func GetHeat(name types.NamespacedName) *heatv1.Heat {
	instance := &heatv1.Heat{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func CreateSecret(name types.NamespacedName, data map[string][]byte) *corev1.Secret {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
		},
		Data: data,
	}
	Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
	return secret
}

func CreateHeatSecret(namespace string, name string) *corev1.Secret {
	return CreateSecret(
		types.NamespacedName{Namespace: namespace, Name: name},
		map[string][]byte{
			"HeatPassword":         []byte("12345678"),
			"HeatDatabasePassword": []byte("12345678"),
			"AuthEncryptionKey":    []byte("1234567812345678123456781212345678345678"),
		},
	)
}

func CreateHeatMessageBusSecret(namespace string, name string) *corev1.Secret {
	return CreateSecret(
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
