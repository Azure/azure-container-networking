package main

import (
	"time"

	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/requestcontroller"
	"github.com/Azure/azure-container-networking/cns/requestcontroller/kubecontroller"
	"github.com/Azure/azure-container-networking/cns/restserver"
	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
	"golang.org/x/net/context"
)

func goRequestController(rc requestcontroller.RequestController) {
	//Exit channel for requestController, this channel is notified when requestController receives
	//SIGINT or SIGTERM, requestControllerExitChan is sent 'true' and you can clean up anything then
	requestControllerExitChan := make(chan bool, 1)

	//Start the RequestController which starts the reconcile loop, blocks
	go func() {
		if err := rc.StartRequestController(requestControllerExitChan); err != nil {
			logger.Errorf("Error starting requestController: %v", err)
			return
		}
	}()

	// After calling StartRequestController, there needs to be some pause before updating CRD spec
	time.Sleep(5 * time.Second)

	// We provide a context when making operations on CRD in case we need to cancel operation
	cntxt := context.Background()

	// Create some dummy uuids
	uuids := make([]string, 5)
	uuids[0] = "uuid0"
	uuids[1] = "uuid1"
	uuids[2] = "uuid2"
	uuids[3] = "uuid3"
	uuids[4] = "uuid4"

	// newCount = oldCount - ips releasing
	// In this example, say we had 20 allocated to the node, we want to release 5, new count would be 15
	oldCount := 20
	newRequestedIPCount := int64(oldCount - len(uuids))

	//Create CRD spec
	spec := &nnc.NodeNetworkConfigSpec{
		RequestedIPCount: newRequestedIPCount,
		IPsNotInUse:      uuids,
	}

	//Update CRD spec
	rc.UpdateCRDSpec(cntxt, spec)

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
	kubeconfig, err := kubecontroller.GetKubeConfig()
	if err != nil {
		logger.Errorf("Error getting kubeconfig: %v", err)
	}

	requestController, err = kubecontroller.NewCrdRequestController(restService, kubeconfig)
	if err != nil {
		logger.Errorf("Error making new RequestController: %v", err)
		return
	}

	//Rely on the interface
	goRequestController(requestController)
}
