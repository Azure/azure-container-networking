package main

import (
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/requestcontroller"
	"github.com/Azure/azure-container-networking/cns/requestcontroller/kubecontroller"
	"github.com/Azure/azure-container-networking/cns/restserver"
	"golang.org/x/net/context"
)

func goRequestController(rc requestcontroller.RequestController) {
	//Before we start the reconcile loop, we want to initialize the CNS state
	if err := rc.InitCNS(); err != nil {
		logger.Errorf("Error initializing cns state: %v", err)
		return
	}

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

	// Example translation from cns secondaryIPs to crdSpec
	ipConfig1 := cns.SecondaryIPConfig{
		IPConfig: cns.IPSubnet{
			IPAddress:    "10.0.0.1",
			PrefixLength: 24,
		},
	}

	ipConfig2 := cns.SecondaryIPConfig{
		IPConfig: cns.IPSubnet{
			IPAddress:    "10.0.0.2",
			PrefixLength: 24,
		},
	}

	secondaryIPConfigs := []cns.SecondaryIPConfig{ipConfig1, ipConfig2}
	oldCount := 20
	newRequestedIPCount := oldCount - len(secondaryIPConfigs)

	spec, _ := kubecontroller.CNSToCRDSpec(secondaryIPConfigs, newRequestedIPCount)

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
