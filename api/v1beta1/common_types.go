/*
Copyright 2023 Red Hat

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

package v1beta1

import (
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
	"github.com/openstack-k8s-operators/lib-common/modules/storage"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// HeatTemplate -
type HeatTemplate struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=heat
	// ServiceUser - optional username used for this service to register in heat
	ServiceUser string `json:"serviceUser"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=heat
	// DatabaseAccount - optional MariaDBAccount used for heat DB, defaults to heat.
	DatabaseAccount string `json:"databaseAccount"`

	// +kubebuilder:validation:Required
	// Secret containing OpenStack password information for heat HeatDatabasePassword, HeatPassword
	// and HeatAuthEncryptionKey
	Secret string `json:"secret"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default={service: HeatPassword, authEncryptionKey: HeatAuthEncryptionKey}
	// PasswordSelectors - Selectors to identify the DB and ServiceUser password from the Secret
	PasswordSelectors PasswordSelector `json:"passwordSelectors"`

	// +kubebuilder:validation:Optional
	// ExtraMounts containing files and plugins
	ExtraMounts []HeatExtraVolMounts `json:"extraMounts,omitempty"`

	// +kubebuilder:validation:Optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// Auth - Parameters related to authentication
	Auth AuthSpec `json:"auth,omitempty"`
}

// AuthSpec defines authentication parameters
type AuthSpec struct {
	// +kubebuilder:validation:Optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// ApplicationCredentialSecret - Secret containing Application Credential ID and Secret
	ApplicationCredentialSecret string `json:"applicationCredentialSecret,omitempty"`
}

// HeatServiceTemplate -
type HeatServiceTemplate struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Maximum=32
	// +kubebuilder:validation:Minimum=0
	// Replicas -
	Replicas *int32 `json:"replicas"`

	// +kubebuilder:validation:Optional
	// ConfigOverwrite - interface to overwrite default config files like e.g. policy.json.
	// But can also be used to add additional files. Those get added to the service config dir in /etc/<service> .
	// TODO: -> implement
	DefaultConfigOverwrite map[string]string `json:"defaultConfigOverwrite,omitempty"`

	// +kubebuilder:validation:Optional
	// NodeSelector to target subset of worker nodes for running the service
	NodeSelector *map[string]string `json:"nodeSelector,omitempty"`

	// +kubebuilder:validation:Optional
	// CustomServiceConfig - customize the service config using this parameter to change service defaults,
	// or overwrite rendered information using raw OpenStack config format. The content gets added to
	// to /etc/heat/heat.conf.d directory as 02-custom-service.conf file.
	CustomServiceConfig string `json:"customServiceConfig,omitempty"`

	// +kubebuilder:validation:Optional
	// CustomServiceConfigSecrets - customize the service config using this parameter to specify Secrets
	// that contain sensitive service config data. The content of each Secret gets added to the
	// /etc/heat/heat.conf.d directory as a custom config file.
	CustomServiceConfigSecrets []string `json:"customServiceConfigSecrets,omitempty"`

	// +kubebuilder:validation:Optional
	// Resources - Compute Resources required by this service (Limits/Requests).
	// https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// +kubebuilder:validation:Optional
	// TopologyRef to apply the Topology defined by the associated CR referenced
	// by name
	TopologyRef *topologyv1.TopoRef `json:"topologyRef,omitempty"`
}

// APIOverrideSpec to override the generated manifest of several child resources.
type APIOverrideSpec struct {
	// Override configuration for the Service created to serve traffic to the cluster.
	// The key must be the endpoint type (public, internal)
	Service map[service.Endpoint]service.RoutedOverrideSpec `json:"service,omitempty"`
}

// PasswordSelector ..
type PasswordSelector struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="HeatPassword"
	// Service - Selector to get the heat service password from the Secret
	Service string `json:"service"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="HeatAuthEncryptionKey"
	// AuthEncryptionKey - Selector to get the heat auth encryption key from the Secret
	AuthEncryptionKey string `json:"authEncryptionKey"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="HeatStackDomainAdminPassword"
	// StackDomainAdminPassword - Selector to get the heat stack domain admin password from the Secret
	StackDomainAdminPassword string `json:"stackDomainAdminPassword"`
}

// HeatExtraVolMounts exposes additional parameters processed by the heat-operator
// and defines the common VolMounts structure provided by the main storage module
type HeatExtraVolMounts struct {
	// +kubebuilder:validation:Optional
	Name string `json:"name,omitempty"`
	// +kubebuilder:validation:Optional
	Region string `json:"region,omitempty"`
	// +kubebuilder:validation:Required
	VolMounts []storage.VolMounts `json:"extraVol"`
}

// ValidateTopology -
func (instance *HeatServiceTemplate) ValidateTopology(
	basePath *field.Path,
	namespace string,
) field.ErrorList {
	var allErrs field.ErrorList
	allErrs = append(allErrs, topologyv1.ValidateTopologyRef(
		instance.TopologyRef,
		*basePath.Child("topologyRef"), namespace)...)
	return allErrs
}

// Propagate is a function used to filter VolMounts according to the specified
// PropagationType array
func (g *HeatExtraVolMounts) Propagate(svc []storage.PropagationType) []storage.VolMounts {
	var vl []storage.VolMounts
	for _, gv := range g.VolMounts {
		vl = append(vl, gv.Propagate(svc)...)
	}
	return vl
}
