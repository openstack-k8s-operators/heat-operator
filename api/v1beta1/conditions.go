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

package v1beta1

import (
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
)

const (
	// HeatAPIReadyCondition ...
	HeatAPIReadyCondition condition.Type = "HeatAPIReady"

	// HeatCfnAPIReadyCondition ...
	HeatCfnAPIReadyCondition condition.Type = "HeatCfnAPIReady"

	// HeatEngineReadyCondition ...
	HeatEngineReadyCondition condition.Type = "HeatEngineReady"

	// HeatStackDomainReadyCondition ...
	HeatStackDomainReadyCondition condition.Type = "HeatStackDomainReady"

	// HeatRabbitMqNotificationURLReadyCondition Status=True condition which indicates if the RabbitMQ TransportURL is ready
	HeatRabbitMqNotificationURLReadyCondition condition.Type = "HeatRabbitMqNotificationURLReady"
)

// Common Messages used by API objects.
const (
	//
	// HeatAPIReady condition messages
	//
	// HeatAPIReadyInitMessage ...
	HeatAPIReadyInitMessage = "HeatAPI not started"

	// HeatAPIReadyErrorMessage ...
	HeatAPIReadyErrorMessage = "HeatAPI error occured %s"

	//
	// HeatCfnAPIReady condition messages
	//
	// HeatCfnAPIReadyInitMessage ...
	HeatCfnAPIReadyInitMessage = "HeatCfnAPI not started"

	// HeatCfnAPIReadyErrorMessage ...
	HeatCfnAPIReadyErrorMessage = "HeatCfnAPI error occured %s"

	//
	// HeatEngineReady condition messages
	//
	// HeatEngineReadyInitMessage ...
	HeatEngineReadyInitMessage = "HeatEngine not started"

	// HeatEngineReadyErrorMessage ...
	HeatEngineReadyErrorMessage = "HeatEngine error occured %s"

	//
	// HeatStackDomainReady condition messages
	//
	// HeatStackDomainReadyInitMessage
	HeatStackDomainReadyInitMessage = "HeatStackDomain not started"

	// HeatStackDomainReadyRunningMessage
	HeatStackDomainReadyRunningMessage = "HeatStackDomain creation in progress"

	// HeatStackDomainReadyMessage
	HeatStackDomainReadyMessage = "HeatStackDomain successfully created"

	// HeatStackDomainReadyErrorMessage
	HeatStackDomainReadyErrorMessage = "HeatStackDomain error occured %s"

	//
	// HeatRabbitMqNotificationURLReady condition messages
	//
	// HeatRabbitMqNotificationURLReadyInitMessage
	HeatRabbitMqNotificationURLReadyInitMessage = "HeatRabbitMqNotificationURL not started"

	// HeatRabbitMqNotificationURLReadyRunningMessage
	HeatRabbitMqNotificationURLReadyRunningMessage = "HeatRabbitMqNotificationURL creation in progress"

	// HeatRabbitMqNotificationURLReadyMessage
	HeatRabbitMqNotificationURLReadyMessage = "HeatRabbitMqNotificationURL successfully created"

	// HeatRabbitMqNotificationURLReadyErrorMessage
	HeatRabbitMqNotificationURLReadyErrorMessage = "HeatRabbitMqNotificationURL error occured %s"
)
