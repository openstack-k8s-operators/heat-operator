package controllers

import (
	"context"

	"github.com/go-logr/logr"
	heatv1beta1 "github.com/openstack-k8s-operators/heat-operator/api/v1beta1"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	configmap "github.com/openstack-k8s-operators/lib-common/modules/common/configmap"
	"github.com/openstack-k8s-operators/lib-common/modules/common/endpoint"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	"github.com/openstack-k8s-operators/lib-common/modules/openstack"
)

// KeystoneAPIGetter provides a interface to the Keystone operator methods.
type KeystoneAPIGetter interface {
	GetKeystoneAPI(ctx context.Context, h *helper.Helper, namespace string, labelSelector map[string]string) (*keystonev1.KeystoneAPI, error)
}

// KeystoneAPIGetterImpl implements the KeystoneAPIGetter interface
type KeystoneAPIGetterImpl struct{}

// GetKeystoneAPI wraps the Keystone GetKeystoneAPI method and is used to retrive the
// KeystoneAPI resource from the OpenShift API.
func (k KeystoneAPIGetterImpl) GetKeystoneAPI(ctx context.Context, h *helper.Helper, namespace string, labelSelector map[string]string) (*keystonev1.KeystoneAPI, error) {
	return keystonev1.GetKeystoneAPI(ctx, h, namespace, labelSelector)
}

// EndpointGetter provides a interface to the endpoint package from lib-common.
type EndpointGetter interface {
	GetEndpoint(api *keystonev1.KeystoneAPI, endpointType endpoint.Endpoint) (string, error)
}

// EndpointGetterImpl implements the EndpointGetter interface.
type EndpointGetterImpl struct{}

// GetEndpoint wraps the Keystone GetEndpoint method and is used to retrieve the Keystone API endpoint
func (e EndpointGetterImpl) GetEndpoint(api *keystonev1.KeystoneAPI, endpointType endpoint.Endpoint) (string, error) {
	return api.GetEndpoint(endpointType)
}

// ConfigMapEnsurer provides a interface to the configmap package from lib-common.
type ConfigMapEnsurer interface {
	EnsureConfigMaps(ctx context.Context, h *helper.Helper, instance *heatv1beta1.Heat, cms []util.Template, envVars *map[string]env.Setter) error
}

// ConfigMapEnsurerImpl implements the ConfigMapEnsurer interface.
type ConfigMapEnsurerImpl struct{}

// EnsureConfigMaps wraps the lib-common EnsureConfigMaps method from the configmaps package.
func (c *ConfigMapEnsurerImpl) EnsureConfigMaps(ctx context.Context, h *helper.Helper, instance *heatv1beta1.Heat, cms []util.Template, envVars *map[string]env.Setter) error {
	return configmap.EnsureConfigMaps(ctx, h, instance, cms, envVars)
}

// OkoOpenStack interface provides an abstraction from the openstack library itself to allow
// for better decoupling and integration testing.
type OkoOpenStack interface {
	CreateUser(log logr.Logger, user openstack.User) (string, error)
	CreateDomain(log logr.Logger, domain openstack.Domain) (string, error)
}
