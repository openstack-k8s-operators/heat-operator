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
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// DbSyncHash hash
	DbSyncHash = "dbsync"

	// DeploymentHash hash used to detect changes
	DeploymentHash = "deployment"

	// HeatAPIContainerImage - default fall-back container image for HeatAPI if associated env var not provided
	HeatAPIContainerImage = "quay.io/podified-antelope-centos9/openstack-heat-api:current-podified"
	// HeatCfnAPIContainerImage - default fall-back container image for HeatCfnAPI if associated env var not provided
	HeatCfnAPIContainerImage = "quay.io/podified-antelope-centos9/openstack-heat-api-cfn:current-podified"
	// HeatEngineContainerImage - default fall-back container image for HeatEngine if associated env var not provided
	HeatEngineContainerImage = "quay.io/podified-antelope-centos9/openstack-heat-engine:current-podified"
)

// HeatSpec defines the desired state of Heat
type HeatSpec struct {
	// Common input parameters for all Heat services
	HeatTemplate `json:",inline"`

	// +kubebuilder:validation:Required
	// MariaDB instance name.
	// Right now required by the maridb-operator to get the credentials from the instance to create the DB.
	// Might not be required in future.
	DatabaseInstance string `json:"databaseInstance"`

	// +kubebuilder:validation:Required
	// +kubebuilder:default=memcached
	// Memcached instance name.
	MemcachedInstance string `json:"memcachedInstance"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	// PreserveJobs - do not delete jobs after they finished e.g. to check logs
	PreserveJobs bool `json:"preserveJobs"`

	// +kubebuilder:validation:Optional
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
	// NodeSelector to target subset of worker nodes for running the Heat services
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// +kubebuilder:validation:Required
	// HeatAPI - Spec definition for the API service of this Heat deployment
	HeatAPI HeatAPITemplate `json:"heatAPI"`

	// +kubebuilder:validation:Required
	// HeatCfnAPI - Spec definition for the CfnAPI service of this Heat deployment
	HeatCfnAPI HeatCfnAPITemplate `json:"heatCfnAPI"`

	// +kubebuilder:validation:Required
	// HeatEngine - Spec definition for the Engine service of this Heat deployment
	HeatEngine HeatEngineTemplate `json:"heatEngine"`

	// +kubebuilder:validation:Required
	// +kubebuilder:default=rabbitmq
	// RabbitMQ instance name
	// Needed to request a transportURL that is created and used in Heat
	RabbitMqClusterName string `json:"rabbitMqClusterName"`
}

// HeatStatus defines the observed state of Heat
type HeatStatus struct {
	// Map of hashes to track e.g. job status
	Hash map[string]string `json:"hash,omitempty"`

	// Conditions
	Conditions condition.Conditions `json:"conditions,omitempty" optional:"true"`

	// Heat Database Hostname
	DatabaseHostname string `json:"databaseHostname,omitempty"`

	// TransportURLSecret - Secret containing RabbitMQ transportURL
	TransportURLSecret string `json:"transportURLSecret,omitempty"`

	// ReadyCount of Heat API instance
	HeatAPIReadyCount int32 `json:"heatApiReadyCount,omitempty"`

	// ReadyCount of Heat CfnAPI instance
	HeatCfnAPIReadyCount int32 `json:"heatCfnApiReadyCount,omitempty"`

	// ReadyCount of Heat Engine instance
	HeatEngineReadyCount int32 `json:"heatEngineReadyCount,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[0].status",description="Status"
//+kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.conditions[0].message",description="Message"

// Heat is the Schema for the heats API
type Heat struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HeatSpec   `json:"spec,omitempty"`
	Status HeatStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// HeatList contains a list of Heat
type HeatList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Heat `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Heat{}, &HeatList{})
}

// IsReady - returns true if Heat is reconciled successfully
func (instance Heat) IsReady() bool {
	return instance.Status.Conditions.IsTrue(condition.ReadyCondition)
}

// RbacConditionsSet - set the conditions for the rbac object
func (instance Heat) RbacConditionsSet(c *condition.Condition) {
	instance.Status.Conditions.Set(c)
}

// RbacNamespace - return the namespace
func (instance Heat) RbacNamespace() string {
	return instance.Namespace
}

// RbacResourceName - return the name to be used for rbac objects (serviceaccount, role, rolebinding)
func (instance Heat) RbacResourceName() string {
	return "heat-" + instance.Name
}

// SetupDefaults - initializes any CRD field defaults based on environment variables (the defaulting mechanism itself is implemented via webhooks)
func SetupDefaults() {
	// Acquire environmental defaults and initialize Heat defaults with them
	heatDefaults := HeatDefaults{
		APIContainerImageURL:    util.GetEnvVar("RELATED_IMAGE_HEAT_API_IMAGE_URL_DEFAULT", HeatAPIContainerImage),
		CfnAPIContainerImageURL: util.GetEnvVar("RELATED_IMAGE_HEAT_CFNAPI_IMAGE_URL_DEFAULT", HeatCfnAPIContainerImage),
		EngineContainerImageURL: util.GetEnvVar("RELATED_IMAGE_HEAT_ENGINE_IMAGE_URL_DEFAULT", HeatEngineContainerImage),
	}

	SetupHeatDefaults(heatDefaults)
}
