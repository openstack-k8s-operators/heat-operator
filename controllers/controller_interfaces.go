package controllers

import (
	"context"

	heatv1beta1 "github.com/openstack-k8s-operators/heat-operator/api/v1beta1"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	configmap "github.com/openstack-k8s-operators/lib-common/modules/common/configmap"
	"github.com/openstack-k8s-operators/lib-common/modules/common/endpoint"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
)

type KeystoneAPIGetter interface {
	GetKeystoneAPI(ctx context.Context, h *helper.Helper, namespace string, labelSelector map[string]string) (*keystonev1.KeystoneAPI, error)
}

type EndpointGetter interface {
	GetEndpoint(api *keystonev1.KeystoneAPI, endpointType endpoint.Endpoint) (string, error)
}

type KeystoneAPIGetterImpl struct{}

func (k KeystoneAPIGetterImpl) GetKeystoneAPI(ctx context.Context, h *helper.Helper, namespace string, labelSelector map[string]string) (*keystonev1.KeystoneAPI, error) {
	return keystonev1.GetKeystoneAPI(ctx, h, namespace, labelSelector)
}

type EndpointGetterImpl struct{}

func (e EndpointGetterImpl) GetEndpoint(api *keystonev1.KeystoneAPI, endpointType endpoint.Endpoint) (string, error) {
	return api.GetEndpoint(endpointType)
}

type ConfigMapEnsurer interface {
	EnsureConfigMaps(ctx context.Context, h *helper.Helper, instance *heatv1beta1.Heat, cms []util.Template, envVars *map[string]env.Setter) error
}

type ConfigMapEnsurerImpl struct{}

func (c *ConfigMapEnsurerImpl) EnsureConfigMaps(ctx context.Context, h *helper.Helper, instance *heatv1beta1.Heat, cms []util.Template, envVars *map[string]env.Setter) error {
	return configmap.EnsureConfigMaps(ctx, h, instance, cms, envVars)
}
