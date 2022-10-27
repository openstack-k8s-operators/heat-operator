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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// DbSyncHash hash
	DbSyncHash = "dbsync"

	// DeploymentHash hash used to detect changes
	DeploymentHash = "deployment"
)

// HeatSpec defines the desired state of Heat
type HeatSpec struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=heat
	// ServiceUser - optional username used for this service to register in heat
	ServiceUser string `json:"serviceUser"`

	// +kubebuilder:validation:Required
	// MariaDB instance name.
	// Right now required by the maridb-operator to get the credentials from the instance to create the DB.
	// Might not be required in future.
	DatabaseInstance string `json:"databaseInstance,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=heat
	// DatabaseUser - optional username used for heat DB, defaults to heat.
	// TODO: -> implement needs work in mariadb-operator, right now only heat.
	DatabaseUser string `json:"databaseUser"`

	// +kubebuilder:validation:Required
	// Secret containing OpenStack password information for heat HeatDatabasePassword, AdminPassword
	Secret string `json:"secret,omitempty"`

	// +kubebuilder:validation:Optional
	// PasswordSelectors - Selectors to identify the DB and AdminUser password from the Secret
	PasswordSelectors PasswordSelector `json:"passwordSelectors,omitempty"`

	// +kubebuilder:validation:Optional
	// Debug - enable debug for different deploy stages. If an init container is used, it runs and the
	// actual action pod gets started with sleep infinity
	Debug HeatDebug `json:"debug,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	// PreserveJobs - do not delete jobs after they finished e.g. to check logs
	PreserveJobs bool `json:"preserveJobs,omitempty"`

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
	// HeatEngineCount - interface to overwrite default number of Heat engine pods. This will
	// scale the number of Heat engine pods. Note that this shouldn't be confused with the heat.conf option
	// for num_engine_workers: n. This option provides an alternative and allows the user to scale in a k8s native
	// manner, while preseving the num_engine_workers option as well.
	HeatEngineCount int32 `json:"heatEngineCount,omitempty"`

	// +kubebuilder:validation:Required
	// HeatAPI - Spec definition for the API service of this Heat deployment
	HeatAPI HeatAPISpec `json:"heatAPI"`

	// +kubebuilder:validation:Required
	// HeatEngine - Spec definition for the API service of this Heat deployment
	HeatEngine HeatEngineSpec `json:"heatEngine"`
}

type PasswordSelector struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="heatDatabasePassword"
	// Database - Selector to get the heat Database user password from the Secret
	// TODO: not used, need change in mariadb-operator
	Database string `json:"database,omitempty"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="heatPassword"
	// Database - Selector to get the heat service password from the Secret
	Service string `json:"admin,omitempty"`
}

type HeatDebug struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	// DBSync enable debug
	DBSync bool `json:"dbSync,omitempty"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	// ReadyCount enable debug
	Bootstrap bool `json:"bootstrap,omitempty"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	// Service enable debug
	Service bool `json:"service,omitempty"`
}

// HeatStatus defines the observed state of Heat
type HeatStatus struct {
	// Map of hashes to track e.g. job status
	Hash map[string]string `json:"hash,omitempty"`

	// API endpoint
	APIEndpoints map[string]map[string]string `json:"apiEndpoint,omitempty"`

	// Conditions
	Conditions condition.Conditions `json:"conditions,omitempty" optional:"true"`

	// Neutron Database Hostname
	DatabaseHostname string `json:"databaseHostname,omitempty"`

	// ServiceID - the ID of the registered service in keystone
	ServiceIDs map[string]string `json:"serviceIDs,omitempty"`

	// ReadyCount of Heat instances
	ReadyCount int32 `json:"readyCount,omitempty"`

	// ReadyCount of Heat API instance
	HeatAPIReadyCount int32 `json:"heatApiReadyCount,omitempty"`

	// ReadyCount of Heat API instance
	HeatEngineReadyCount int32 `json:"heatEngineReadyCount,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

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

// IsReady - returns true if service is ready to serve requests
func (instance Heat) IsReady() bool {
	ready := instance.Status.HeatAPIReadyCount > 0

	ready = ready && instance.Status.HeatEngineReadyCount > 0

	return ready
}
