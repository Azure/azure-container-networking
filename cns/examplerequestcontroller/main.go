package main

import (
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/requestcontroller"
	"github.com/Azure/azure-container-networking/cns/requestcontroller/channels"
)

//Example of using the requestcontroller package
func main() {
	//Assuming logger is already setup and stuff
	logger.InitLogger("Azure CNS", 3, 3, "")

	cnsChannel := make(chan channels.CNSChannel, 1)

	rc, err := requestcontroller.NewRequestController(cnsChannel)
	if err != nil {
		logger.Errorf("Error making new RequestController: %v", err)
	}

	//Start the RequestController which starts the reconcile loop
	if err := rc.StartRequestController(); err != nil {
		logger.Errorf("Error starting requestController: %v", err)
	}

	x := <-cnsChannel

	logger.Printf("done: %v", x)

}
