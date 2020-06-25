package requestcontroller

import "context"

// interface for cns to interact with the request controller
type RequestController interface {
	StartRequestController(exitChan chan bool) error
	ReleaseIPsByUUIDs(cntxt context.Context, listOfIPUUIDS []string, newRequestedIPCount int) error
}
