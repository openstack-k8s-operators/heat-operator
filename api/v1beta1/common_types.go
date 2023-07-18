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
	corev1 "k8s.io/api/core/v1"
)

// HeatTemplate -
type HeatTemplate struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=heat
	// ServiceUser - optional username used for this service to register in heat
	ServiceUser string `json:"serviceUser"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=heat
	// DatabaseUser - optional username used for heat DB, defaults to heat.
	// TODO: -> implement needs work in mariadb-operator, right now only heat.
	DatabaseUser string `json:"databaseUser"`

	// +kubebuilder:validation:Required
	// Secret containing OpenStack password information for heat HeatDatabasePassword, HeatPassword
	// and HeatAuthEncryptionKey
	Secret string `json:"secret"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default={database: HeatDatabasePassword, service: HeatPassword, authEncryptionKey: HeatAuthEncryptionKey}
	// PasswordSelectors - Selectors to identify the DB and ServiceUser password from the Secret
	PasswordSelectors PasswordSelector `json:"passwordSelectors"`
}

// HeatServiceTemplate -
type HeatServiceTemplate struct {
	// +kubebuilder:validation:Required
	// ContainerImage - Container Image URL
	ContainerImage string `json:"containerImage"`

	// +kubebuilder:validation:Optional
	// Debug - enable debug for different deploy stages. If an init container is used, it runs and the
	// actual action pod gets started with sleep infinity
	Debug HeatServiceDebug `json:"debug,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Maximum=32
	// +kubebuilder:validation:Minimum=0
	// Replicas -
	Replicas *int32 `json:"replicas"`

	// +kubebuilder:validation:Optional
	// NodeSelector to target subset of worker nodes for running the service
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// +kubebuilder:validation:Optional
	// Resources - Compute Resources required by this service (Limits/Requests).
	// https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

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
}

// PasswordSelector ..
type PasswordSelector struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="HeatDatabasePassword"
	// Database - Selector to get the heat Database user password from the Secret
	// TODO: not used, need change in mariadb-operator
	Database string `json:"database"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="HeatPassword"
	// Service - Selector to get the heat service password from the Secret
	Service string `json:"service"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="HeatAuthEncryptionKey"
	// AuthEncryptionKey - Selector to get the heat auth encryption key from the Secret
	AuthEncryptionKey string `json:"authEncryptionKey"`
}

// HeatDebug ...
type HeatDebug struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	// DBSync enable debug
	DBSync bool `json:"dbSync"`
}

// HeatServiceDebug ...
type HeatServiceDebug struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	// Service enable debug
	Service bool `json:"service"`
}
