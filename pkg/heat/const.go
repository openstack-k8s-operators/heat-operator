/*

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

package heat

const (
	// ServiceName -
	ServiceName = "heat"
	// ServiceType -
	ServiceType = "orchestration"
	// CfnServiceName -
	CfnServiceName = "heat-cfn"
	// CfnServiceType -
	CfnServiceType = "cloudformation"
	// StackDomainAdminUsername -
	StackDomainAdminUsername = "heat_stack_domain_admin"
	// StackDomainName -
	StackDomainName = "heat_stack"
	// DatabaseName -
	DatabaseName = "heat"
	// DefaultsConfigFileName -
	DefaultsConfigFileName = "00-config.conf"
	// CustomConfigFileName -
	CustomConfigFileName = "01-config.conf"
	// CustomServiceConfigFileName -
	CustomServiceConfigFileName = "02-config.conf"
	// HeatPublicPort -
	HeatPublicPort int32 = 8004
	// HeatInternalPort -
	HeatInternalPort int32 = 8004
	// HeatCfnPublicPort -
	HeatCfnPublicPort int32 = 8000
	// HeatCfnInternalPort -
	HeatCfnInternalPort int32 = 8000
	// KollaConfigDbSync -
	KollaConfigDbSync = "/var/lib/config-data/merged/db-sync-config.json"
	// APIComponent -
	APIComponent = "api"
	// CfnAPIComponent -
	CfnAPIComponent = "cfnapi"
	// EngineComponent -
	EngineComponent = "engine"
)
