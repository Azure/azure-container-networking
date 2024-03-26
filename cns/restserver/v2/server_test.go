package v2

import (
	"fmt"
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/cns/common"
	"github.com/Azure/azure-container-networking/cns/fakes"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/restserver"
)

var (
	defaultAPIServerURLTest = "localhost:9100"
)

// Wraps the test run with service setup and teardown.
func TestStartEchoServer(t *testing.T) {
	var (
		err error
	)

	logger.InitLogger("testlogs", 0, 0, "./")

	// Create the service.
	if err = startService(); err != nil {
		fmt.Printf("Failed to start CNS Service. Error: %v", err)
		os.Exit(1)
	}
}

func startService() error {
	var service *restserver.HTTPRestService
	// Create the service.
	config := common.ServiceConfig{}

	nmagentClient := &fakes.NMAgentClientFake{}
	service, err := restserver.NewHTTPRestService(&config, &fakes.WireserverClientFake{}, &fakes.WireserverProxyFake{}, nmagentClient, nil, nil, nil)
	if err != nil {
		return err
	}

	svc := New(service)
	svc.Name = "cns-test-echo-server"

	if service != nil {
		err = service.Init(&config)
		if err != nil {
			logger.Errorf("Failed to Init CNS, err:%v.\n", err)
			return err
		}

		service := New(service)

		err = service.Start(defaultAPIServerURLTest)
		if err != nil {
			logger.Errorf("Failed to start CNS, err:%v.\n", err)
			return err
		}
	}

	return nil
}
