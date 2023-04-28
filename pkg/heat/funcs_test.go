package heat

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestGetOwningHeatName(t *testing.T) {
	tests := []struct {
		name           string
		instance       client.Object
		expectedResult string
	}{
		{
			name: "Instance with Heat OwnerRef",
			instance: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "Heat",
							Name: "heat-test-1",
						},
					},
				},
			},
			expectedResult: "heat-test-1",
		},
		{
			name: "Instance with multiple OwnerRefs",
			instance: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "RandomKind",
							Name: "not-heat",
						},
						{
							Kind: "AnotherRandomKind",
							Name: "not-heat-1",
						},
					},
				},
			},
			expectedResult: "",
		},
		{
			name: "Instance with multiple OwnerRefs but one Heat",
			instance: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "NotHeatKind",
							Name: "NoHeat",
						},
						{
							Kind: "Heat",
							Name: "ThisIsHeat",
						},
						{
							Kind: "AnotherNotHeat",
							Name: "NotHeat2",
						},
					},
				},
			},
			expectedResult: "ThisIsHeat",
		},
		{
			name:           "No OwnerRef",
			instance:       &corev1.Pod{},
			expectedResult: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := GetOwningHeatName(test.instance)
			if result != test.expectedResult {
				t.Errorf("Expected result to be %s, but got %s", test.expectedResult, result)
			}
		})
	}
}
