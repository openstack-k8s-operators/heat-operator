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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HeatEngineTemplate defines the input parameters for the Heat Engine service
type HeatEngineTemplate struct {
	// Common input parameters for all Heat services
	HeatServiceTemplate `json:",inline"`
}

// HeatEngineSpec defines the desired state of HeatEngine
type HeatEngineSpec struct {
	// Common input parameters for all Heat services
	HeatTemplate `json:",inline"`

	// Input parameters for the Heat Engine service
	HeatEngineTemplate `json:",inline"`

	// +kubebuilder:validation:Required
	// DatabaseHostname - Heat Database Hostname
	DatabaseHostname string `json:"databaseHostname"`

	// +kubebuilder:validation:Required
	// TransportURLSecret - Secret containing RabbitMQ transportURL
	TransportURLSecret string `json:"transportURLSecret"`

	// +kubebuilder:validation:Optional
	// NotificationURLSecret - Secret containing RabbitMQ transportURL for notifications
	NotificationURLSecret string `json:"notificationURLSecret,omitempty"`

	// +kubebuilder:validation:Required
	// ServiceAccount - service account name used internally to provide Heat services the default SA name
	ServiceAccount string `json:"serviceAccount"`
}

// HeatEngineStatus defines the observed state of HeatEngine
type HeatEngineStatus struct {
	// Map of hashes to track e.g. job status
	Hash map[string]string `json:"hash,omitempty"`

	// Conditions
	Conditions condition.Conditions `json:"conditions,omitempty" optional:"true"`

	// ReadyCount of HeatEngine instances
	ReadyCount int32 `json:"readyCount,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[0].status",description="Status"
//+kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.conditions[0].message",description="Message"

// HeatEngine defined.
type HeatEngine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HeatEngineSpec   `json:"spec,omitempty"`
	Status HeatEngineStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// HeatEngineList contains a list of HeatEngine
type HeatEngineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HeatEngine `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HeatEngine{}, &HeatEngineList{})
}

// IsReady - returns true if HeatEngine is reconciled successfully
func (instance HeatEngine) IsReady() bool {
	return instance.Status.Conditions.IsTrue(condition.ReadyCondition)
}
