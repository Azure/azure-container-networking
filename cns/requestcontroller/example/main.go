package main

import (
	"time"

	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/requestcontroller"
	"github.com/Azure/azure-container-networking/cns/restserver"
)

func goRequestController(rc requestcontroller.RequestController) {
	//Start the RequestController which starts the reconcile loop
	if err := rc.StartRequestController(); err != nil {
		logger.Errorf("Error starting requestController: %v", err)
	}
	// After calling StartRequestController, there needs to be some pause or, the calling program
	// must keep running because the reconcile loop is spawned off on a different go-routine inside
	// rc.StartRequestController()
	time.Sleep(5 * time.Second)

	// Example of releasing ips, give the requestController list of uuids which correspond to the ips you want to release
	// Create some dummy uuids
	uuids := make([]string, 5)
	uuids[0] = "uuid0"
	uuids[1] = "uuid1"
	uuids[2] = "uuid2"
	uuids[3] = "uuid3"
	uuids[4] = "uuid4"
	rc.ReleaseIPsByUUIDs(uuids)

	// This method is not synchronous, all it does is send the new count to the API server through the CRD spec.
	// Dnc would see this new ip count, and in turn, send the requested IPS in the CRD status, which would
	// trigger the reconcile loop, and in that reconcile loop is where the ips would be passed to cns.
	// So this method only relays the message, it does nothing else
	// Update to dummy count
	rc.UpdateRequestedIPCount(int64(10))
}

//Example of using the requestcontroller package
func main() {
	var requestController requestcontroller.RequestController

	//Assuming logger is already setup and stuff
	logger.InitLogger("Azure CNS", 3, 3, "")

	restService := &restserver.HTTPRestService{}

	requestController, err := requestcontroller.NewK8sRequestController(restService)
	if err != nil {
		logger.Errorf("Error making new RequestController: %v", err)
	}

	//Rely on the interface
	goRequestController(requestController)
}
