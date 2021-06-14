package nns

import (
	"context"
	"fmt"
	"github.com/Azure/azure-container-networking/test/nnsmockserver"
	"os"
	"strings"
	"testing"
	"time"
)

var mockserver *nnsmockserver.NnsMockServer

func TestMain(m *testing.M) {
	setup()
	code := m.Run()
	teardown()
	os.Exit(code)
}

func setup() {
	mockserver = nnsmockserver.NewNnsMockServer()
	go mockserver.StartGrpcServer(nnsPort)
	fmt.Println("mock nns server started")
}

func teardown() {
	mockserver.StopGrpcServer()
	fmt.Println("mock nns server stopped")
}

// CNI ADD to add container to network
func TestAddContainerNetworking(t *testing.T) {

	client := &GrpcClient{}
	if err, _ := client.AddContainerNetworking(
		context.Background(),
		"sf_8e9961f4-5b4f-4b3c-a9ae-c3294b0d9681",
		"testnwspace"); err != nil {
		t.Fatalf("TestAddContainerNetworking failed: %v", err)
	}
}

// CNI DEL to delete container from network
func TestDeleteContainerNetworking(t *testing.T) {

	client := &GrpcClient{}
	if err, _ := client.DeleteContainerNetworking(
		context.Background(),
		"sf_8e9961f4-5b4f-4b3c-a9ae-c3294b0d9681",
		"testnwspace"); err != nil {
		t.Fatalf("TestSetupContainer: %v", err)
	}
}

// CNI ADD to add container to network - failure case
func TestAddContainerNetworkingFailure(t *testing.T) {

	client := &GrpcClient{}

	var err error
	if err, _ = client.AddContainerNetworking(context.Background(), "testpod", "testnwspace"); err == nil {
		t.Fatalf("TestAddContainerNetworkingFailure failed. Expected error but none returned")
	}

	if !strings.Contains(err.Error(), "Setup") {
		t.Fatalf("TestAddContainerNetworkingFailure failed. Error should have contained Setup. %v ", err)
	}
}

// CNI DEL to add container to network - failure case
func TestDeleteContainerNetworkingFailure(t *testing.T) {

	client := &GrpcClient{}

	var err error
	if err, _ = client.DeleteContainerNetworking(context.Background(), "testpod", "testnwspace"); err == nil {
		t.Fatalf("TestDeleteContainerNetworkingFailure failed. Expected error but none returned")
	}

	if !strings.Contains(err.Error(), "Teardown") {
		t.Fatalf("TestDeleteContainerNetworkingFailure failed. Error should have contained Teardown. %v", err)
	}
}

// CNI ADD to add container to network - grpc server was down temporarily and client reconnects well
func TestAddContainerNetworkingGrpcServerDown(t *testing.T) {

	// shutdown server to simulate connection error
	teardown()

	// bring up the server again after 25 seconds
	go func() {
		time.Sleep(25 * time.Second)
		setup()
	}()

	client := &GrpcClient{}

	var err error
	if err, _ = client.AddContainerNetworking(
		context.Background(), "sf_8e9961f4-5b4f-4b3c-a9ae-c3294b0d9681", "testnwspace"); err != nil {

		t.Fatalf("TestAddContainerNetworkingGrpcServerDown failed. %s", err)
	}
}
