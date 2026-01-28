package restserver

import "time"

const (
	// Key against which CNS state is persisted.
	storeKey         = "ContainerNetworkService"
	StoreKey         = storeKey
	EndpointStoreKey = "Endpoints"
	IPAMStoreKey     = "IPAMState"
	attach           = "Attach"
	detach           = "Detach"
	// Rest service state identifier for named lock
	stateJoinedNetworks = "JoinedNetworks"
	dncApiVersion       = "?api-version=2018-03-01"
	nmaAPICallTimeout   = 2 * time.Second
)
