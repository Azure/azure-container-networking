package restserver

import "time"

const (
	// Key against which CNS state is persisted.
	storeKey         = "ContainerNetworkService"
	EndpointStoreKey = "Endpoints"
	attach           = "Attach"
	detach           = "Detach"
	// Rest service state identifier for named lock
	dncAPIVersion     = "?api-version=2018-03-01"
	nmaAPICallTimeout = 2 * time.Second
)
