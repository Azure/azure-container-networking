package requestcontroller

// interface for cns to interact with the request controller
type RequestController interface {
	StartRequestController() error
	ReleaseIPsByUUIDs(listOfIPUUIDS []string) error
	UpdateRequestedIPCount(newCount int64) error
}
