package requestcontroller

// interface for cns to interact with the request controller
type RequestController interface {
	StartRequestController() error
	ReleaseIpsByUUID(listOfIPUUIDS []string, newCount int64) error
}
