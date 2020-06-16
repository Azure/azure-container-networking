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

	cnsChannel := make(chan channels.CNSChannel)

	rc, err := requestcontroller.NewRequestController(cnsChannel)
	if err != nil {
		logger.Errorf("Error making new RequestController: %v", err)
	}

	//Spawn off a goroutine running the reconcile loop

	if err = rc.StartRequestController(); err != nil {
		logger.Errorf("Error starting requestController: %v", err)
	}

}
