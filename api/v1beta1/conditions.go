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

	// HeatEngineReadyCondition ...
	HeatEngineReadyCondition condition.Type = "HeatEngineReady"

	// HeatEngineReadyErrorMessage ...
	HeatEngineReadyErrorMessage condition.Type = "HeatEngineError"
)

const (
	// HeatAPIReadyInitMessage ...
	//
	// HeatAPIReady condition messages
	//
	HeatAPIReadyInitMessage = "HeatAPI not started"

	// HeatAPIReadyErrorMessage ...
	HeatAPIReadyErrorMessage = "HeatAPI error occured %s"

	// HeatConductorReadyInitMessage ...
	//
	// HeatConductorReady condition messages
	//
	HeatConductorReadyInitMessage = "HeatConductor not started"

	// HeatConductorReadyErrorMessage ...
	HeatConductorReadyErrorMessage = "HeatConductor error occured %s"
)

const (
	// HeatRabbitMqTransportURLReadyCondition Status=True condition which indicates if the RabbitMQ TransportURLUrl is ready
	HeatRabbitMqTransportURLReadyCondition condition.Type = "HeatRabbitMqTransportURLReady"
)

// Common Messages used by API objects.
const (
	//
	// HeatRabbitMqTransportURLReady condition messages
	//
	// HeatRabbitMqTransportURLReadyInitMessage
	HeatRabbitMqTransportURLReadyInitMessage = "HeatRabbitMqTransportURL not started"

	// HeatRabbitMqTransportURLReadyRunningMessage
	HeatRabbitMqTransportURLReadyRunningMessage = "HeatRabbitMqTransportURL creation in progress"

	// HeatRabbitMqTransportURLReadyMessage
	HeatRabbitMqTransportURLReadyMessage = "HeatRabbitMqTransportURL successfully created"

	// HeatRabbitMqTransportURLReadyErrorMessage
	HeatRabbitMqTransportURLReadyErrorMessage = "HeatRabbitMqTransportURL error occured %s"
)
