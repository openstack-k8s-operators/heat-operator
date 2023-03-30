package functional_test

import (
	"github.com/google/uuid"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	routev1 "github.com/openshift/api/route/v1"
	heatv1 "github.com/openstack-k8s-operators/heat-operator/api/v1beta1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
)

const (
	MessageBusSecretName = "rabbitmq-secret"
	ContainerImage       = "test://heat"

	// consistencyTimeout is the amount of time we use to repeatedly check
	// that a condition is still valid. This is intendet to be used in
	// asserts using `Consistently`.
	consistencyTimeout = timeout
)

func CreateUnstructured(rawObj map[string]interface{}) *unstructured.Unstructured {
	logger.Info("Creating", "raw", rawObj)
	unstructuredObj := &unstructured.Unstructured{Object: rawObj}
	_, err := controllerutil.CreateOrPatch(
		ctx, k8sClient, unstructuredObj, func() error { return nil })
	Expect(err).ShouldNot(HaveOccurred())
	return unstructuredObj
}

func HeatAPIConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetHeatAPI(name)
	return instance.Status.Conditions
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

// CreateSecret creates a secret that has all the information heatAPI needs
func CreateheatAPISecret(namespace string, name string) *corev1.Secret {
	return CreateSecret(
		types.NamespacedName{Namespace: namespace, Name: name},
		map[string][]byte{
			"heatPassword":            []byte("12345678"),
			"heatAPIDatabasePassword": []byte("12345678"),
		},
	)
}

func GetDefaultHeatSpec() map[string]interface{} {
	return map[string]interface{}{
		"databaseInstance":    "test-database",
		"serviceUser":         "heat",
		"rabbitMqClusterName": "rabbitmq",
		"secret":              SecretName,
		"databaseUser":        "heat",
	}
}

func CreateHeat(name types.NamespacedName, spec map[string]interface{}) client.Object {
	raw := map[string]interface{}{
		"apiVersion": "heat.openstack.org/v1beta1",
		"kind":       "heat",
		"metadata": map[string]interface{}{
			"name":      name.Name,
			"namespace": name.Namespace,
		},
		"spec": spec,
	}
	return CreateUnstructured(raw)
}

func DeleteInstance(instance client.Object) {
	// We have to wait for the controller to fully delete the instance
	logger.Info("Deleting", "Name", instance.GetName(), "Namespace", instance.GetNamespace(), "Kind", instance.GetObjectKind().GroupVersionKind().Kind)
	Eventually(func(g Gomega) {
		name := types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
		err := k8sClient.Get(ctx, name, instance)
		// if it is already gone that is OK
		if k8s_errors.IsNotFound(err) {
			return
		}
		g.Expect(err).ShouldNot(HaveOccurred())

		g.Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())

		err = k8sClient.Get(ctx, name, instance)
		g.Expect(k8s_errors.IsNotFound(err)).To(BeTrue())
	}, timeout, interval).Should(Succeed())
}

func Getheat(name types.NamespacedName) *heatv1.Heat {
	instance := &heatv1.Heat{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func heatConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := Getheat(name)
	return instance.Status.Conditions
}

func GetDefaultHeatEngineSpec() map[string]interface{} {
	return map[string]interface{}{
		"containerImage": ContainerImage,
	}
}

func CreateHeatEngine(namespace string, spec map[string]interface{}) client.Object {
	HeatEngineName := uuid.New().String()

	raw := map[string]interface{}{
		"apiVersion": "heat.openstack.org/v1beta1",
		"kind":       "HeatEngine",
		"metadata": map[string]interface{}{
			"name":      HeatEngineName,
			"namespace": namespace,
		},
		"spec": spec,
	}
	return CreateUnstructured(raw)
}

func GetHeatEngine(name types.NamespacedName) *heatv1.HeatEngine {
	instance := &heatv1.HeatEngine{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func HeatEngineConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetHeatEngine(name)
	return instance.Status.Conditions
}

// CreateheatConductorSecret creates a secret that has all the information
// heatConductor needs
func CreateHeatEngineSecret(namespace string, name string) *corev1.Secret {
	return CreateSecret(
		types.NamespacedName{Namespace: namespace, Name: name},
		map[string][]byte{
			"HeatPassword": []byte("12345678"),
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

func CreateHeatSecret(namespace string, name string) *corev1.Secret {
	return CreateSecret(
		types.NamespacedName{Namespace: namespace, Name: name},
		map[string][]byte{
			"HeatPassword": []byte("12345678"),
		},
	)
}

func GetService(name types.NamespacedName) *corev1.Service {
	instance := &corev1.Service{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func AssertRouteExists(name types.NamespacedName) *routev1.Route {
	instance := &routev1.Route{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func AssertRouteNotExists(name types.NamespacedName) *routev1.Route {
	instance := &routev1.Route{}
	Consistently(func(g Gomega) {
		err := k8sClient.Get(ctx, name, instance)
		g.Expect(k8s_errors.IsNotFound(err)).To(BeTrue())
	}, consistencyTimeout, interval).Should(Succeed())
	return instance
}

func GetDefaultHeatAPISpec() map[string]interface{} {
	return map[string]interface{}{
		"containerImage": ContainerImage,
	}
}

func CreateHeatAPI(namespace string, spec map[string]interface{}) client.Object {
	name := uuid.New().String()

	raw := map[string]interface{}{
		"apiVersion": "heat.openstack.org/v1beta1",
		"kind":       "HeatAPI",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
		"spec": spec,
	}
	return CreateUnstructured(raw)
}

func GetHeatAPI(name types.NamespacedName) *heatv1.HeatAPI {
	instance := &heatv1.HeatAPI{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func HeatAPINotExists(name types.NamespacedName) {
	Consistently(func(g Gomega) {
		instance := &heatv1.HeatAPI{}
		err := k8sClient.Get(ctx, name, instance)
		g.Expect(k8s_errors.IsNotFound(err)).To(BeTrue())
	}, consistencyTimeout, interval).Should(Succeed())
}

func CreateEmptySecret(name types.NamespacedName) *corev1.Secret {
	return CreateSecret(name, map[string][]byte{})
}

func CreateEmptyConfigMap(name types.NamespacedName) *corev1.ConfigMap {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
		},
		Data: map[string]string{},
	}
	Expect(k8sClient.Create(ctx, configMap)).Should(Succeed())
	return configMap
}

func DeleteSecret(name types.NamespacedName) {
	Eventually(func(g Gomega) {
		secret := &corev1.Secret{}
		err := k8sClient.Get(ctx, name, secret)
		// if it is already gone that is OK
		if k8s_errors.IsNotFound(err) {
			return
		}
		g.Expect(err).ShouldNot(HaveOccurred())

		g.Expect(k8sClient.Delete(ctx, secret)).Should(Succeed())

		err = k8sClient.Get(ctx, name, secret)
		g.Expect(k8s_errors.IsNotFound(err)).To(BeTrue())
	}, timeout, interval).Should(Succeed())
}

func DeleteConfigMap(name types.NamespacedName) {
	Eventually(func(g Gomega) {
		configMap := &corev1.ConfigMap{}
		err := k8sClient.Get(ctx, name, configMap)
		// if it is already gone that is OK
		if k8s_errors.IsNotFound(err) {
			return
		}
		g.Expect(err).ShouldNot(HaveOccurred())

		g.Expect(k8sClient.Delete(ctx, configMap)).Should(Succeed())

		err = k8sClient.Get(ctx, name, configMap)
		g.Expect(k8s_errors.IsNotFound(err)).To(BeTrue())
	}, timeout, interval).Should(Succeed())
}
