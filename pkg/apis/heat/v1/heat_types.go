package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// HeatSpec defines the desired state of Heat
// +k8s:openapi-gen=true
type HeatSpec struct {
	HeatDatabasePassword         string `json:"heatDatabasePassword,omitempty"`
	HeatDatabaseUsername         string `json:"heatDatabaseUsername,omitempty"`
	DatabaseAdminPassword        string `json:"databaseAdminPassword,omitempty"`
	DatabaseAdminUsername        string `json:"databaseAdminUsername,omitempty"`
	HeatMessagingPassword        string `json:"heatMessagingPassword,omitempty"`
	HeatMessagingUsername        string `json:"heatMessagingUsername,omitempty"`
	HeatAPIContainerImage        string `json:"heatAPIContainerImage,omitempty"`
	HeatEngineContainerImage     string `json:"heatEngineContainerImage,omitempty"`
	HeatAPIReplicas              int32  `json:"heatAPIReplicas"`
	HeatEngineReplicas           int32  `json:"heatEngineReplicas"`
	HeatServicePassword          string `json:"heatServicePassword"`
	HeatStackDomainAdminPassword string `json:"heatStackDomainAdminPassword"`
	MysqlContainerImage          string `json:"mysqlContainerImage,omitempty"`
}

// HeatStatus defines the observed state of Heat
// +k8s:openapi-gen=true
type HeatStatus struct {
	// DbSync hash
	DbSyncHash string `json:"dbSyncHash"`
	// Deployment hash used to detect changes
	DeploymentHash       string `json:"deploymentHash"`
	EngineDeploymentHash string `json:"engineDeploymentHash"`
	// bootstrap completed
	BootstrapHash string `json:"bootstrapHash"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Heat is the Schema for the heats API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=heats,scope=Namespaced
type Heat struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HeatSpec   `json:"spec,omitempty"`
	Status HeatStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// HeatList contains a list of Heat
type HeatList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Heat `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Heat{}, &HeatList{})
}
