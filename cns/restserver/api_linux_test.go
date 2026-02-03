// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package restserver

import (
	"fmt"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
)

func TestCreateNetworkContainer(t *testing.T) {
	// requires more than 30 seconds to run
	fmt.Println("Test: TestCreateNetworkContainer")

	setEnv(t)
	setOrchestratorType(t, cns.ServiceFabric)

	// Test create network container of type JobObject
	fmt.Println("TestCreateNetworkContainer: JobObject")

	params := createOrUpdateNetworkContainerParams{
		ncID:         "f47ac10b-58cc-0372-8567-0e02b2c3d476",
		ncIP:         "10.1.0.5",
		ncType:       "JobObject",
		ncVersion:    "0",
		podName:      "testpod",
		podNamespace: "testpodnamespace",
	}

	err := createOrUpdateNetworkContainerWithParams(params)
	if err != nil {
		t.Errorf("Failed to save the goal state for network container of type JobObject "+
			" due to error: %+v", err)
		t.Fatal(err)
	}

	fmt.Println("Deleting the saved goal state for network container of type JobObject")
	err = deleteNetworkContainerWithParams(params)
	if err != nil {
		t.Errorf("Failed to delete the saved goal state due to error: %+v", err)
		t.Fatal(err)
	}

	// Test create network container of type WebApps
	fmt.Println("TestCreateNetworkContainer: WebApps")
	params = createOrUpdateNetworkContainerParams{
		ncID:         "f47ac10b-58cc-0372-8567-0e02b2c3d475",
		ncIP:         "192.0.0.5",
		ncType:       "WebApps",
		ncVersion:    "0",
		podName:      "testpod",
		podNamespace: "testpodnamespace",
	}

	err = createOrUpdateNetworkContainerWithParams(params)
	if err != nil {
		t.Errorf("creatOrUpdateWebAppContainerWithName failed Err:%+v", err)
		t.Fatal(err)
	}

	params = createOrUpdateNetworkContainerParams{
		ncID:         "f47ac10b-58cc-0372-8567-0e02b2c3d475",
		ncIP:         "192.0.0.6",
		ncType:       "WebApps",
		ncVersion:    "0",
		podName:      "testpod",
		podNamespace: "testpodnamespace",
	}

	err = createOrUpdateNetworkContainerWithParams(params)
	if err != nil {
		t.Errorf("Updating interface failed Err:%+v", err)
		t.Fatal(err)
	}

	fmt.Println("Now calling DeleteNetworkContainer")

	err = deleteNetworkContainerWithParams(params)
	if err != nil {
		t.Errorf("Deleting interface failed Err:%+v", err)
		t.Fatal(err)
	}

	// Test create network container of type COW
	params = createOrUpdateNetworkContainerParams{
		ncID:         "f47ac10b-58cc-0372-8567-0e02b2c3d474",
		ncIP:         "10.0.0.5",
		ncType:       "COW",
		ncVersion:    "0",
		podName:      "testpod",
		podNamespace: "testpodnamespace",
	}

	err = createOrUpdateNetworkContainerWithParams(params)
	if err != nil {
		t.Errorf("Failed to save the goal state for network container of type COW"+
			" due to error: %+v", err)
		t.Fatal(err)
	}

	fmt.Println("Deleting the saved goal state for network container of type COW")
	err = deleteNetworkContainerWithParams(params)
	if err != nil {
		t.Errorf("Failed to delete the saved goal state due to error: %+v", err)
		t.Fatal(err)
	}
}

func TestGetInterfaceForNetworkContainer(t *testing.T) {
	// requires more than 30 seconds to run
	fmt.Println("Test: TestCreateNetworkContainer")

	setEnv(t)
	setOrchestratorType(t, cns.Kubernetes)

	params := createOrUpdateNetworkContainerParams{
		ncID:         "f47ac10b-58cc-0372-8567-0e02b2c3d479",
		ncIP:         "11.0.0.5",
		ncType:       "WebApps",
		ncVersion:    "0",
		podName:      "testpod",
		podNamespace: "testpodnamespace",
	}

	err := createOrUpdateNetworkContainerWithParams(params)
	if err != nil {
		t.Errorf("creatOrUpdateWebAppContainerWithName failed Err:%+v", err)
		t.Fatal(err)
	}

	fmt.Println("Now calling getInterfaceForContainer")
	err = getInterfaceForContainer(params)
	if err != nil {
		t.Errorf("getInterfaceForContainer failed Err:%+v", err)
		t.Fatal(err)
	}

	fmt.Println("Now calling DeleteNetworkContainer")

	err = deleteNetworkContainerWithParams(params)
	if err != nil {
		t.Errorf("Deleting interface failed Err:%+v", err)
		t.Fatal(err)
	}
}
