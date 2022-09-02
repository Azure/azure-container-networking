package restserver

const (
	// Key against which CNS state is persisted.
	storeKey         = "ContainerNetworkService"
	EndpointStoreKey = "Endpoints"
	attach           = "Attach"
	detach           = "Detach"
	// Rest service state identifier for named lock
	stateJoinedNetworks = "JoinedNetworks"
	dncApiVersion       = "?api-version=2018-03-01"
)
