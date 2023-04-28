package controllers

import (
	"context"
	"fmt"
	"testing"

	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common/endpoint"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	heatv1beta1 "github.com/openstack-k8s-operators/heat-operator/api/v1beta1"
)

func TestCreateHashOfInputHashes(t *testing.T) {
	instance := &heatv1beta1.HeatEngine{}

	envVars := map[string]env.Setter{}

	envVars["secret"] = env.SetValue("12345678")

	r := &HeatEngineReconciler{
		Client: &mockClient{},
	}

	hash, err := r.createHashOfInputHashes(context.Background(), instance, envVars)
	assert.NoError(t, err)
	assert.NotEmpty(t, hash)
}

func TestAPIDeploymentCreateOrUpdate(t *testing.T) {

	testCases := []struct {
		name           string
		instance       *heatv1beta1.Heat
		expectedAPI    *heatv1beta1.HeatAPI
		expectedOp     controllerutil.OperationResult
		expectedErrMsg string
		setOwnerRef    error
	}{
		{
			name: "CreateOrUpdate successful",
			instance: &heatv1beta1.Heat{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "heat",
					Namespace: "test-namespace",
				},
				Spec: heatv1beta1.HeatSpec{
					HeatAPI:      heatv1beta1.HeatAPISpec{},
					ServiceUser:  "test-service-user",
					DatabaseUser: "test-database-user",
					Secret:       "test-secret",
				},
			},
			expectedAPI: &heatv1beta1.HeatAPI{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "heat-api",
					Namespace: "test-namespace",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         heatv1beta1.GroupVersion.String(),
							Kind:               "Heat",
							Name:               "heat",
							Controller:         pointer.Bool(true),
							BlockOwnerDeletion: pointer.Bool(true),
						},
					},
					ResourceVersion: "1",
				},
				Spec: heatv1beta1.HeatAPISpec{
					ServiceUser:  "test-service-user",
					DatabaseUser: "test-database-user",
					Secret:       "test-secret",
				},
			},
			expectedOp:     controllerutil.OperationResult("created"),
			expectedErrMsg: "",
			setOwnerRef:    nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			err := heatv1beta1.AddToScheme(scheme)
			if err != nil {
				fmt.Printf("Unable to register type with scheme: %v", err)
			}
			r := &HeatReconciler{
				Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
				Scheme: scheme,
			}

			api, op, err := r.apiDeploymentCreateOrUpdate(tc.instance)
			assert.Equal(t, tc.expectedAPI, api)
			assert.Equal(t, tc.expectedOp, op)
			if tc.expectedErrMsg != "" {
				assert.EqualError(t, err, tc.expectedErrMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEngineDeploymentCreateOrUpdate(t *testing.T) {

	testCases := []struct {
		name           string
		instance       *heatv1beta1.Heat
		expectedAPI    *heatv1beta1.HeatEngine
		expectedOp     controllerutil.OperationResult
		expectedErrMsg string
		setOwnerRef    error
	}{
		{
			name: "CreateOrUpdate successful",
			instance: &heatv1beta1.Heat{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "heat",
					Namespace: "test-namespace",
				},
				Spec: heatv1beta1.HeatSpec{
					HeatEngine:   heatv1beta1.HeatEngineSpec{},
					ServiceUser:  "test-service-user",
					DatabaseUser: "test-database-user",
					Secret:       "test-secret",
				},
			},
			expectedAPI: &heatv1beta1.HeatEngine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "heat-engine",
					Namespace: "test-namespace",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         heatv1beta1.GroupVersion.String(),
							Kind:               "Heat",
							Name:               "heat",
							Controller:         pointer.Bool(true),
							BlockOwnerDeletion: pointer.Bool(true),
						},
					},
					ResourceVersion: "1",
				},
				Spec: heatv1beta1.HeatEngineSpec{
					ServiceUser:  "test-service-user",
					DatabaseUser: "test-database-user",
					Secret:       "test-secret",
				},
			},
			expectedOp:     controllerutil.OperationResult("created"),
			expectedErrMsg: "",
			setOwnerRef:    nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			err := heatv1beta1.AddToScheme(scheme)
			if err != nil {
				fmt.Printf("Unable to register type with scheme: %v", err)
			}
			r := &HeatReconciler{
				Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
				Scheme: scheme,
			}

			api, op, err := r.engineDeploymentCreateOrUpdate(tc.instance)
			assert.Equal(t, tc.expectedAPI, api)
			assert.Equal(t, tc.expectedOp, op)
			if tc.expectedErrMsg != "" {
				assert.EqualError(t, err, tc.expectedErrMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGenerateServiceConfigMaps(t *testing.T) {
	configMapVars := make(map[string]env.Setter)
	scheme := runtime.NewScheme()
	_ = heatv1beta1.AddToScheme(scheme)
	r := &HeatReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}
	testCases := []struct {
		name               string
		instance           *heatv1beta1.Heat
		endpointPublic     endpoint.Endpoint
		expectedAuthURL    string
		expectedError      error
		mockKeystoneAPI    *keystonev1.KeystoneAPI
		mockKeystoneAPIErr error
	}{
		{
			name: "EndpointPublic = endpoint.EndpointPublic",
			instance: &heatv1beta1.Heat{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "heat",
					Namespace: "test-namespace",
				},
				Spec: heatv1beta1.HeatSpec{
					HeatEngine:   heatv1beta1.HeatEngineSpec{},
					ServiceUser:  "test-service-user",
					DatabaseUser: "test-database-user",
					Secret:       "test-secret",
				},
			},
			endpointPublic:     endpoint.EndpointPublic,
			expectedAuthURL:    "http://example.com/public",
			expectedError:      nil,
			mockKeystoneAPI:    &keystonev1.KeystoneAPI{},
			mockKeystoneAPIErr: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockKeystoneAPIGetter := &MockKeystoneAPIGetter{
				KeystoneAPI: tc.mockKeystoneAPI,
				Err:         tc.mockKeystoneAPIErr,
			}

			mockEndpointGetter := &MockEndpointGetter{
				EndpointURL: tc.expectedAuthURL,
				Err:         nil,
			}

			mockConfigMapEnsurer := &MockConfigMapEnsurerImpl{}

			err := r.generateServiceConfigMaps(context.Background(), tc.instance, &helper.Helper{}, &configMapVars, mockKeystoneAPIGetter, mockEndpointGetter, mockConfigMapEnsurer)

			assert.Equal(t, tc.expectedError, err)
		})
	}
}
