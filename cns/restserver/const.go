package restserver

import "time"

const (
	// Key against which CNS state is persisted.
	storeKey                     = "ContainerNetworkService"
	EndpointStoreKey             = "Endpoints"
	EndpointDeleteIntentStoreKey = "EndpointDeleteIntents"
	attach                       = "Attach"
	detach                       = "Detach"
	// endpointDeleteIntentTTL bounds how long a delete intent can reject a late
	// ADD for the same infra container. It only needs to outlive a CNI request
	// that was already in flight when the DELETE arrived, plus a CNS restart, so
	// it is deliberately far shorter than a pod lifetime: an intent that outlives
	// the racing request can only reject work that should have succeeded.
	endpointDeleteIntentTTL = 10 * time.Minute
	// Rest service state identifier for named lock
	stateJoinedNetworks = "JoinedNetworks"
	dncApiVersion       = "?api-version=2018-03-01"
	nmaAPICallTimeout   = 2 * time.Second
)
