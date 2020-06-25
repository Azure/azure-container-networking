package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"testing"

	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	mockClient MockClient
)

type MockClient struct {
	mockStore map[string]*nnc.NodeNetworkConfig //Mock store of namespace/name -> nodeNetworkConfig
}

// Mock implementation of the K8sClientInterface Get method
func (mc *MockClient) Get(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
	nodeNetConfig, ok := mc.mockStore[key.String()]
	if !ok {
		return errors.New("CRD not found")
	}
	obj = nodeNetConfig
	return nil
}

func initMockClient() {
	//Make a mock store
	mockStore := make(map[string]*nnc.NodeNetworkConfig)

	//Fill it with an example namespace/name -> nodeNetConfig obj
	mockStore["namespace/name"] = &nnc.NodeNetworkConfig{}

	// Make mock client initialized with mock store
	mockClient = MockClient{mockStore: mockStore}
}

func TestMain(m *testing.M) {
	//Setup mock client
	initMockClient()
	//run test
	m.Run()
}

func TestNewK8sRequestController(t *testing.T) {

	// Attempt to retrieve mock obj
	key := client.ObjectKey{
		Namespace: "namespace",
		Name:      "name",
	}
	toFill := &nnc.NodeNetworkConfig{}
	if err := mockClient.Get(context.Background(), key, toFill); err != nil {
		fmt.Printf("Some error ocured: %v \n", err)
	}

	return
}

// TODO: Finish tests, make interface for reconciler
