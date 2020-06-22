package main

import (
	"time"

	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/requestcontroller"
	"github.com/Azure/azure-container-networking/cns/requestcontroller/channels"
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

	// Create some dummy uuids
	uuids := make([]string, 5)
	uuids[0] = "uuid0"
	uuids[1] = "uuid1"
	uuids[2] = "uuid2"
	uuids[3] = "uuid3"
	uuids[4] = "uuid100"
	rc.ReleaseIPsByUUIDs(uuids)

	// Update to dummy count
	rc.UpdateRequestedIPCount(int64(10))
}

//Example of using the requestcontroller package
func main() {
	var requestController requestcontroller.RequestController

	//Assuming logger is already setup and stuff
	logger.InitLogger("Azure CNS", 3, 3, "")

	cnsChannel := make(chan channels.CNSChannel, 1)

	requestController, err := requestcontroller.NewK8sRequestController(cnsChannel)
	if err != nil {
		logger.Errorf("Error making new RequestController: %v", err)
	}

	//Rely on the interface
	goRequestController(requestController)
}
