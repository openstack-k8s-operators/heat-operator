package controllers

import (
	"context"

	heatv1beta1 "github.com/openstack-k8s-operators/heat-operator/api/v1beta1"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common/endpoint"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	"github.com/stretchr/testify/mock"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type mockClient struct {
	client.Client
	statusError error
}

func (c *mockClient) Status() client.StatusWriter {
	return &mockStatusWriter{statusError: c.statusError}
}

type mockStatusWriter struct {
	client.StatusWriter
	statusError error
}

func (s *mockStatusWriter) Update(ctx context.Context, obj client.Object, opt ...client.SubResourceUpdateOption) error {
	return nil
}

func (s *mockStatusWriter) Create(ctx context.Context, obj client.Object, obj1 client.Object, opt ...client.SubResourceCreateOption) error {
	return nil
}

func (s *mockStatusWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opt ...client.SubResourcePatchOption) error {
	return nil
}

type MockKeystoneAPIGetter struct {
	KeystoneAPI *keystonev1.KeystoneAPI
	Err         error
}

func (m *MockKeystoneAPIGetter) GetKeystoneAPI(ctx context.Context, h *helper.Helper, namespace string, labelSelector map[string]string) (*keystonev1.KeystoneAPI, error) {
	return m.KeystoneAPI, m.Err
}

type MockEndpointGetter struct {
	EndpointURL string
	Err         error
}

func (m *MockEndpointGetter) GetEndpoint(api *keystonev1.KeystoneAPI, endpointType endpoint.Endpoint) (string, error) {
	return "https://totally-legit-keystone-endpoint", m.Err
}

type MockHelper struct {
	*helper.Helper
	mock.Mock
}

type MockConfigMapEnsurer interface {
	EnsureConfigMaps(ctx context.Context, h *helper.Helper, instance *heatv1beta1.Heat, cms []util.Template, envVars *map[string]env.Setter) error
}

type MockConfigMapEnsurerImpl struct{}

func (c *MockConfigMapEnsurerImpl) EnsureConfigMaps(ctx context.Context, h *helper.Helper, instance *heatv1beta1.Heat, cms []util.Template, envVars *map[string]env.Setter) error {
	return nil
}
