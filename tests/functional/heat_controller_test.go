package functional_test

import (
	"fmt"
	"os"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	heat "github.com/openstack-k8s-operators/heat-operator/api/v1beta1"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
)

func DefaultheatTemplate() *heat.Heat {
	return &heat.Heat{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "heat.openstack.org/v1alpha1",
			Kind:       "heat",
		},
		ObjectMeta: metav1.ObjectMeta{},
		Spec: heat.HeatSpec{
			DatabaseInstance: "test-db-instance",
			Secret:           SecretName,
		},
	}
}

type Testheat struct {
	LookupKey types.NamespacedName
	// The input data for creating a heat
	Template *heat.Heat
	// The current state of the heat resource updated by Refresh()
	Instance *heat.Heat
	// TransportURLName -
	TransportURLName types.NamespacedName
}

// NewTestheat initializes the the input for a heat instance
// but does not create it yet. So the client can finetuned the Template data
// before calling Create()
func NewTestheat(namespace string) Testheat {
	name := fmt.Sprintf("heat-%s", uuid.New().String())
	template := DefaultheatTemplate()
	template.ObjectMeta.Name = name
	template.ObjectMeta.Namespace = namespace
	return Testheat{
		LookupKey: types.NamespacedName{Name: name, Namespace: namespace},
		Template:  template,
		Instance:  &heat.Heat{},
		TransportURLName: types.NamespacedName{
			Namespace: namespace,
			Name:      fmt.Sprintf("%s-heat-transport", name),
		},
	}
}

// Creates the heat resource in k8s based on the Template. The Template
// is not updated during create. This call waits until the resource is created.
// The last known state of the resource is available via Instance.
func (t Testheat) Create() {
	Expect(k8sClient.Create(ctx, t.Template.DeepCopy())).Should(Succeed())
	t.Refresh()
}

// Deletes the heat resource from k8s and waits until it is deleted.
func (t Testheat) Delete() {
	Expect(k8sClient.Delete(ctx, t.Instance)).Should(Succeed())
	// We have to wait for the heat instance to be fully deleted
	// by the controller
	Eventually(func() bool {
		err := k8sClient.Get(ctx, t.LookupKey, t.Instance)
		return k8s_errors.IsNotFound(err)
	}, timeout, interval).Should(BeTrue())
}

// Refreshes the state of the Instance from k8s
func (t Testheat) Refresh() *heat.Heat {
	Eventually(func() bool {
		err := k8sClient.Get(ctx, t.LookupKey, t.Instance)
		return err == nil
	}, timeout, interval).Should(BeTrue())
	return t.Instance
}

// Gets the condition of given type from the resource.
func (t Testheat) GetCondition(conditionType condition.Type, reason condition.Reason) condition.Condition {
	t.Refresh()
	if t.Instance.Status.Conditions == nil {
		return condition.Condition{}
	}

	cond := t.Instance.Status.Conditions.Get(conditionType)
	if cond != nil && cond.Reason == reason {
		return *cond
	}

	return condition.Condition{}

}

type TestKeystoneAPI struct {
	LookupKey types.NamespacedName
	// The input data for creating a KeystoneAPI
	Template *keystonev1.KeystoneAPI
	Instance *keystonev1.KeystoneAPI
}

// NewTestKeystoneAPI initializes the the input for a KeystoneAPI instance
// but does not create it yet. So the client can finetuned the Template data
// before calling Create()
func NewTestKeystoneAPI(namespace string) *TestKeystoneAPI {
	name := fmt.Sprintf("keystone-%s", uuid.New().String())
	template := &keystonev1.KeystoneAPI{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "keystone.openstack.org/v1beta1",
			Kind:       "KeystoneAPI",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: TestNamespace,
		},
		Spec: keystonev1.KeystoneAPISpec{
			DatabaseUser: "foo-bar-baz",
		},
		Status: keystonev1.KeystoneAPIStatus{
			APIEndpoints: map[string]string{
				"internal": "fake-keystone-internal-endpoint",
				"public":   "fake-keystone-public-endpoint",
			},
			DatabaseHostname: "fake-database-hostname",
		},
	}
	return &TestKeystoneAPI{
		LookupKey: types.NamespacedName{Name: name, Namespace: namespace},
		Template:  template,
		Instance:  &keystonev1.KeystoneAPI{},
	}
}

// Creates the KeystoneAPI resource in k8s based on the Template. The Template
// is not updated with the result of the create.
func (t TestKeystoneAPI) Create() {
	t.Instance = t.Template.DeepCopy()
	Expect(k8sClient.Create(ctx, t.Instance)).Should(Succeed())

	// the Status field needs to be written via a separate client
	t.Instance.Status = t.Template.Status
	Expect(k8sClient.Status().Update(ctx, t.Instance)).Should(Succeed())
}

// Deletes the KeystoneAPI resource from k8s and waits until it is deleted.
func (t TestKeystoneAPI) Delete() {
	Expect(k8sClient.Delete(ctx, t.Template.DeepCopy())).Should(Succeed())
	// We have to wait for the instance to be fully deleted
	// by the controller
	Eventually(func() bool {
		err := k8sClient.Get(ctx, t.LookupKey, t.Instance)
		return k8s_errors.IsNotFound(err)
	}, timeout, interval).Should(BeTrue())
}

var _ = Describe("heat controller", func() {

	var heat Testheat
	var secret *corev1.Secret
	var keystoneAPI *TestKeystoneAPI
	var heatTransportURLName types.NamespacedName

	BeforeEach(func() {
		// lib-common uses OPERATOR_TEMPLATES env var to locate the "templates"
		// directory of the operator. We need to set them othervise lib-common
		// will fail to generate the ConfigMap as it does not find common.sh
		err := os.Setenv("OPERATOR_TEMPLATES", "../../templates")
		Expect(err).NotTo(HaveOccurred())
		heatTransportURLName = types.NamespacedName{
			Name: heat.LookupKey.Name + "-heat-transport",
		}

		heat = NewTestheat(TestNamespace)
		heat.Create()

	})

	AfterEach(func() {
		heat.Delete()
		if secret != nil {
			Expect(k8sClient.Delete(ctx, secret)).Should(Succeed())
		}
		if keystoneAPI != nil {
			keystoneAPI.Delete()
		}
	})

	When("A heat instance is created", func() {
		BeforeEach(func() {
			DeferCleanup(DeleteInstance, CreateHeat(heat.LookupKey, GetDefaultHeatSpec()))
		})

		It("should have the Spec and Status fields initialized", func() {
			Expect(heat.Instance.Spec.Secret).Should(Equal("osp-secret"))
			// TODO(gibi): Why defaulting does not work?
			// Expect(heat.Instance.Spec.ServiceUser).Should(Equal("heat"))
		})

		It("should have a finalizer", func() {
			th.GetTransportURL(heatTransportURLName)
			th.SimulateTransportURLReady(heatTransportURLName)
			// the reconciler loop adds the finalizer so we have to wait for
			// it to run
			Eventually(func() []string {
				return heat.Refresh().ObjectMeta.Finalizers
			}, timeout, interval).Should(ContainElement("heat"))
		})

		It("should be in a state of not having the input ready as the secrete is not create yet", func() {
			Eventually(func() condition.Condition {
				// TODO (mschuppert) change conditon package to be able to use haveSameStateOf Matcher here
				return heat.GetCondition(condition.InputReadyCondition, condition.RequestedReason)
			}, timeout, interval).Should(HaveField("Status", corev1.ConditionFalse))
		})
	})

	When("an unrelated secret is provided", func() {
		It("should remain in a state of waiting for the proper secret", func() {
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "an-unrelated-secret",
					Namespace: TestNamespace,
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())

			Eventually(func() condition.Condition {
				return heat.GetCondition(condition.InputReadyCondition, condition.RequestedReason)
			}, timeout, interval).Should(HaveField("Status", corev1.ConditionFalse))
		})
	})

	When("the proper secret is provided", func() {
		It("should not be in a state of having the input ready", func() {
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      SecretName,
					Namespace: TestNamespace,
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
			Eventually(func() condition.Condition {
				return heat.GetCondition(condition.InputReadyCondition, condition.ReadyReason)
			}, timeout, interval).Should(HaveField("Status", corev1.ConditionTrue))
		})
	})

	When("keystoneAPI instance is available", func() {
		It("should create a ConfigMap for local_settings.py with some config options set based on the KeystoneAPI", func() {
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      SecretName,
					Namespace: TestNamespace,
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())

			keystoneAPI = NewTestKeystoneAPI(TestNamespace)
			keystoneAPI.Create()

			configData := th.GetConfigMap(
				types.NamespacedName{
					Namespace: heat.LookupKey.Namespace,
					Name:      fmt.Sprintf("%s-%s", heat.LookupKey.Name, "config-data"),
				},
			)

			Eventually(configData).ShouldNot(BeNil())
		})
	})
})
