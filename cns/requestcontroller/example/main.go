package main

import (
	"time"

	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/requestcontroller"
	"github.com/Azure/azure-container-networking/cns/requestcontroller/kubernetes"
	"github.com/Azure/azure-container-networking/cns/restserver"
	"golang.org/x/net/context"
)

func goRequestController(rc requestcontroller.RequestController) {
	//Exit channel for requestController, this channel is notified when requestController receives
	//SIGINT or SIGTERM, requestControllerExitChan is sent 'true' and you can clean up anything then
	requestControllerExitChan := make(chan bool, 1)

	//Start the RequestController which starts the reconcile loop
	if err := rc.StartRequestController(requestControllerExitChan); err != nil {
		logger.Errorf("Error starting requestController: %v", err)
		return
	}

	// After calling StartRequestController, there needs to be some pause or, the calling program
	// must keep running because the reconcile loop is spawned off on a different go-routine inside
	time.Sleep(5 * time.Second)
	logger.Printf("Done sleeping")

	//For all request controller interactions with CRD spec (ReleaseIPsByUUIDs and UpdateRequestedIPCount) provide a context
	// in order to be able to cancel if needed in the future
	cntxt := context.Background()

	// Example of releasing ips, give the requestController list of uuids which correspond to the ips you want to release
	// as well as the new requestedIpCount
	// Create some dummy uuids
	uuids := make([]string, 5)
	uuids[0] = "uuid0"
	uuids[1] = "uuid1"
	uuids[2] = "uuid2"
	uuids[3] = "uuid3"
	uuids[4] = "uuid4"

	// newCount = oldCount - #ips releasing
	// In this example, say we had 20 allocated to the node, we want to release 5, new count would be 15
	oldCount := 20
	newRequestedIPCount := oldCount - len(uuids)

	// This method is not synchronous, all it does is send the new count to the API server through the CRD spec.
	// Dnc would see this new ip count, and in turn, send the requested IPS in the CRD status, which would
	// trigger the reconcile loop, and in that reconcile loop is where the ips would be passed to cns.
	// So this method only relays the message, it does nothing else
	rc.ReleaseIPsByUUIDs(cntxt, uuids, newRequestedIPCount)
	time.Sleep(5 * time.Second)

	<-requestControllerExitChan
	logger.Printf("Request controller received sigint or sigterm, time to cleanup")
	// Clean clean...
}

//Example of using the requestcontroller package
func main() {
	var requestController requestcontroller.RequestController

	//Assuming logger is already setup and stuff
	logger.InitLogger("Azure CNS", 3, 3, "")

	restService := &restserver.HTTPRestService{}

	//Provide kubeconfig, this method was abstracted out for testing
	kubeconfig, err := kubernetes.GetKubeConfig()
	if err != nil {
		logger.Errorf("Error getting kubeconfig: %v", err)
	}

	requestController, err = kubernetes.NewK8sRequestController(restService, kubeconfig)
	if err != nil {
		logger.Errorf("Error making new RequestController: %v", err)
		return
	}

	//Rely on the interface
	goRequestController(requestController)
}
