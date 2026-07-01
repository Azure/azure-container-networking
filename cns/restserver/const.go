package restserver

import "time"

const (
	// Key against which CNS state is persisted.
	storeKey                     = "ContainerNetworkService"
	EndpointStoreKey             = "Endpoints"
	EndpointDeleteIntentStoreKey = "EndpointDeleteIntents"
	attach                       = "Attach"
	detach                       = "Detach"
	endpointDeleteIntentTTL      = 24 * time.Hour
	// Rest service state identifier for named lock
	stateJoinedNetworks = "JoinedNetworks"
	dncApiVersion       = "?api-version=2018-03-01"
	nmaAPICallTimeout   = 2 * time.Second
)
