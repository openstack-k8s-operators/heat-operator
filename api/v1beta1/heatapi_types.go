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

package v1beta1

import (
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HeatAPISpec defines the desired state of Heat
type HeatAPISpec struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=heat
	// ServiceUser - optional username used for this service to register in heat
	ServiceUser string `json:"serviceUser"`

	// +kubebuilder:validation:Required
	// +kubebuilder:default="quay.io/podified-antelope-centos9/openstack-heat-api:current-podified"
	// ContainerImage - Heat API Container Image URL
	ContainerImage string `json:"containerImage"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=1
	// Replicas - Heat API Replicas
	Replicas int32 `json:"replicas"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=heat
	// DatabaseUser - optional username used for heat DB, defaults to heat.
	// TODO: -> implement needs work in mariadb-operator, right now only heat.
	DatabaseUser string `json:"databaseUser"`

	// +kubebuilder:validation:Optional
	// DatabaseHostname - Heat Database Hostname
	DatabaseHostname string `json:"databaseHostname,omitempty"`

	// +kubebuilder:validation:Required
	// Secret containing OpenStack password information for heat HeatDatabasePassword, AdminPassword
	Secret string `json:"secret"`

	// +kubebuilder:validation:Optional
	// PasswordSelectors - Selectors to identify the DB and AdminUser password from the Secret
	PasswordSelectors PasswordSelector `json:"passwordSelectors,omitempty"`

	// +kubebuilder:validation:Optional
	// Debug - enable debug for different deploy stages. If an init container is used, it runs and the
	// actual action pod gets started with sleep infinity
	Debug HeatDebug `json:"debug,omitempty"`

	// +kubebuilder:validation:Optional
	// NodeSelector to target subset of worker nodes for running the API service
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default="# add your customization here"
	// CustomServiceConfig - customize the service config using this parameter to change service defaults,
	// or overwrite rendered information using raw OpenStack config format. The content gets added to
	// to /etc/<service>/<service>.conf.d directory as custom.conf file.
	CustomServiceConfig string `json:"customServiceConfig,omitempty"`

	// +kubebuilder:validation:Optional
	// ConfigOverwrite - interface to overwrite default config files like e.g. policy.json.
	// But can also be used to add additional files. Those get added to the service config dir in /etc/<service> .
	// TODO: -> implement
	DefaultConfigOverwrite map[string]string `json:"defaultConfigOverwrite,omitempty"`

	// +kubebuilder:validation:Optional
	// Resources - Compute Resources required by this service (Limits/Requests).
	// https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// +kubebuilder:validation:Optional
	// TransportURLSecret - Secret containing RabbitMQ transportURL
	TransportURLSecret string `json:"transportURLSecret,omitempty"`
}

// HeatAPIStatus defines the observed state of Heat
type HeatAPIStatus struct {
	// Map of hashes to track e.g. job status
	Hash map[string]string `json:"hash,omitempty"`

	// API endpoint
	APIEndpoints map[string]map[string]string `json:"apiEndpoint,omitempty"`

	// Conditions
	Conditions condition.Conditions `json:"conditions,omitempty" optional:"true"`

	// ServiceID - the ID of the registered service in keystone
	ServiceIDs map[string]string `json:"serviceIDs,omitempty"`

	// ReadyCount of Heat instances
	ReadyCount int32 `json:"readyCount,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// HeatAPI ...
type HeatAPI struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HeatAPISpec   `json:"spec,omitempty"`
	Status HeatAPIStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// HeatAPIList contains a list of Heat
type HeatAPIList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HeatAPI `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HeatAPI{}, &HeatAPIList{})
}

// IsReady ...
func (instance HeatAPI) IsReady() bool {
	return instance.Status.ReadyCount >= 1
}
