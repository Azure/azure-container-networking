package restserver

import "time"

const (
	// Key against which CNS state is persisted.
	storeKey         = "ContainerNetworkService"
	EndpointStoreKey = "Endpoints"
	attach           = "Attach"
	detach           = "Detach"
	// Rest service state identifier for named lock
	stateJoinedNetworks = "JoinedNetworks"
	dncApiVersion       = "?api-version=2018-03-01"
	nmaAPICallTimeout   = 2 * time.Second
)

type IPFamily string

const (
	IPv4Family IPFamily = "ipv4"
	IPv6Family IPFamily = "ipv6"
)
