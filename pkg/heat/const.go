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
	// ServiceAccount -
	ServiceAccount = "heat-operator"
	// DatabaseName -
	DatabaseName = "heat"
	// HeatPublicPort -
	HeatPublicPort int32 = 8004
	// HeatPublicPort -
	HeatAdminPort int32 = 8004
	// HeatPublicPort -
	HeatInternalPort int32 = 8004
	// KollaConfigDbSync -
	KollaConfigDbSync = "/var/lib/config-data/merged/db-sync-config.json"
	// ComponentSelector - used by operators to specify pod labels
	ComponentSelector = "component"
	// ApiComponent -
	ApiComponent = "api"
	// EngineComponent -
	EngineComponent = "engine"
)
