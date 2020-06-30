package kubernetes

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/cns/logger"
	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const existingNNCName = "nodenetconfig_1"
const existingNamespace = k8sNamespace
const nonexistingNNCName = "nodenetconfig_nonexisting"
const nonexistingNamespace = "namespace_nonexisting"

var (
	mockStore      map[MockKey]*nnc.NodeNetworkConfig
	mockCNSUpdated bool
)

//MockKey is the key to the mockStore, namespace+"/"+name like in API server
type MockKey struct {
	Namespace string
	Name      string
}

func (m MockKey) String() string {
	return m.Namespace + "/" + m.Name
}

// MockClient implements K8SClient interface
type MockClient struct {
	mockStore map[MockKey]*nnc.NodeNetworkConfig
}

// Mock implementation of the K8sClientInterface Get method
//Mimics that of controller-runtime's client.Client
func (mc MockClient) Get(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
	mockKey := MockKey{
		Namespace: key.Namespace,
		Name:      key.Name,
	}

	nodeNetConfig, ok := mc.mockStore[mockKey]
	if !ok {
		return errors.New("Node Net Config not found in mock store")
	}
	nodeNetConfig.DeepCopyInto(obj.(*nnc.NodeNetworkConfig))

	return nil
}

//Mock implementation of the K8sClientInterface Update method
//Mimics that of controller-runtime's client.Client
func (mc MockClient) Update(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
	nodeNetConfig := obj.(*nnc.NodeNetworkConfig)

	mockKey := MockKey{
		Namespace: nodeNetConfig.ObjectMeta.Namespace,
		Name:      nodeNetConfig.ObjectMeta.Name,
	}

	_, ok := mockStore[mockKey]

	if !ok {
		return errors.New("Node Net Config not found in mock store")
	}

	nodeNetConfig.DeepCopyInto(mockStore[mockKey])
	return nil
}

// MockCNSInteractor implements CNSInteractor interface
type MockCNSInteractor struct{}

// we're just testing that reconciler interacts with CNS on Reconcile().
func (mi MockCNSInteractor) UpdateCNSState(nnc.NodeNetworkConfigStatus) error {
	mockCNSUpdated = true
	return nil
}

func ResetCNSInteractionFlag() {
	mockCNSUpdated = false
}

func TestNewK8sRequestController(t *testing.T) {
	//Test making request controller without logger initialized, should fail
	_, err := NewK8sRequestController(nil, nil)
	if err == nil {
		t.Fatalf("Expected error when making NewK8sRequestController without initializing logger, got nil error")
	} else if !strings.Contains(err.Error(), "logger") {
		t.Fatalf("Expected logger error when making NewK8sRequestController without initializing logger, got: %+v", err)
	}

	//Initialize logger
	logger.InitLogger("Azure CNS Request Controller", 3, 3, "")

	//Test making request controller without HOSTNAME env var set, should fail
	//Save old value though
	hostName, found := os.LookupEnv("HOSTNAME")
	os.Unsetenv("HOSTNAME")
	defer func() {
		if found {
			os.Setenv("HOSTNAME", hostName)
		}
	}()

	_, err = NewK8sRequestController(nil, nil)
	if err == nil {
		t.Fatalf("Expected error when making NewK8sRequestController without setting HOSTNAME env var, got nil error")
	} else if !strings.Contains(err.Error(), "HOSTNAME") {
		t.Fatalf("Expected error when making NewK8sRequestController without setting HOSTNAME env var, got: %+v", err)
	}

	//Successful creation is tested in integrationt tests because it requires standing up a minikube cluster
}

func TestGetNonExistingNodeNetConfig(t *testing.T) {
	rc := createMockRequestController()

	//Test getting nonexisting NodeNetconfig obj
	_, err := rc.getNodeNetConfig(context.Background(), nonexistingNNCName, nonexistingNamespace)
	if err == nil {
		t.Fatalf("Expected error when getting nonexisting nodenetconfig obj. Got nil error.")
	}

}

func TestGetExistingNodeNetConfig(t *testing.T) {
	rc := createMockRequestController()

	//Test getting existing NodeNetConfig obj
	nodeNetConfig, err := rc.getNodeNetConfig(context.Background(), existingNNCName, existingNamespace)
	if err != nil {
		t.Fatalf("Expected no error when getting existing NodeNetworkConfig: %+v", err)
	}

	mockKey := MockKey{
		Namespace: existingNamespace,
		Name:      existingNNCName,
	}

	if !reflect.DeepEqual(nodeNetConfig, mockStore[mockKey]) {
		t.Fatalf("Expected fetched node net config to equal one in mock store")
	}
}

func TestUpdateNonExistingNodeNetConfig(t *testing.T) {
	rc := createMockRequestController()

	//Test updating non existing NodeNetworkConfig obj
	nodeNetConfig := &nnc.NodeNetworkConfig{ObjectMeta: metav1.ObjectMeta{
		Name:      nonexistingNNCName,
		Namespace: nonexistingNamespace,
	}}

	err := rc.updateNodeNetConfig(context.Background(), nodeNetConfig)

	if err == nil {
		t.Fatalf("Expected error when updating non existing NodeNetworkConfig. Got nil error")
	}
}

func TestUpdateExistingNodeNetConfig(t *testing.T) {
	rc := createMockRequestController()

	mockKey := MockKey{
		Namespace: existingNamespace,
		Name:      existingNNCName,
	}

	//Update an existing NodeNetworkConfig obj from the mock store
	nodeNetConfigUpdated := mockStore[mockKey].DeepCopy()
	nodeNetConfigUpdated.ObjectMeta.ClusterName = "New cluster name"

	err := rc.updateNodeNetConfig(context.Background(), nodeNetConfigUpdated)
	if err != nil {
		t.Fatalf("Expected no error when updating existing NodeNetworkConfig, got :%v", err)
	}

	//See that NodeNetworkConfig in mock store was updated
	if !reflect.DeepEqual(nodeNetConfigUpdated, mockStore[mockKey]) {
		t.Fatal("Update of existing NodeNetworkConfig did not get passed along")
	}
}

func TestReleaseIPsByUUIDsOnNonExistingNodeNetConfig(t *testing.T) {
	rc := createMockRequestController()
	rc.hostName = nonexistingNNCName

	uuids := make([]string, 3)
	uuids[0] = "uuid0"
	uuids[1] = "uuid1"
	uuids[2] = "uuid2"
	newCount := 5

	//Test releasing ips by uuid for existing NodeNetworkConfig (hostname)
	err := rc.ReleaseIPsByUUIDs(context.Background(), uuids, newCount)

	if err == nil {
		t.Fatalf("Expected error when releasing ips by uuids")
	}
}

func TestReleaseIPsByUUIDsOnExistingNodeNetConfig(t *testing.T) {
	rc := createMockRequestController()

	uuids := make([]string, 3)
	uuids[0] = "uuid0"
	uuids[1] = "uuid1"
	uuids[2] = "uuid2"
	newCount := 5

	//Test releasing ips by uuid for existing NodeNetworkConfig (hostname)
	err := rc.ReleaseIPsByUUIDs(context.Background(), uuids, newCount)

	if err != nil {
		t.Fatalf("Expected no error when releasing ips by uuids, got :%v", err)
	}

	mockKey := MockKey{
		Namespace: existingNamespace,
		Name:      existingNNCName,
	}

	if !reflect.DeepEqual(mockStore[mockKey].Spec.IPsNotInUse, uuids) {
		t.Fatalf("Expected IpsNotInUse to equal requested ReleaseIpsByUUIDs")
	}

	if mockStore[mockKey].Spec.RequestedIPCount != int64(newCount) {
		t.Fatalf("Expected requested ip count to equal count passed into ReleaseIPsByUUIds")
	}
}

func TestReconcileNonExistingNNC(t *testing.T) {
	rc := createMockRequestController()
	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: nonexistingNamespace,
			Name:      nonexistingNNCName,
		},
	}

	_, err := rc.Reconciler.Reconcile(request)

	if err == nil {
		t.Fatalf("Expected error when calling Reconcile for non existing NodeNetworkConfig")
	}
}

func TestReconcileExistingNNC(t *testing.T) {
	rc := createMockRequestController()
	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: existingNamespace,
			Name:      existingNNCName,
		},
	}

	_, err := rc.Reconciler.Reconcile(request)

	//Want to reset flag to false for next test
	defer ResetCNSInteractionFlag()

	if err != nil {
		t.Fatalf("Expected no error reconciling existing NodeNetworkConfig, got :%v", err)
	}

	if !mockCNSUpdated {
		t.Fatalf("Expected MockCNSInteractor's UpdateCNSState() method to be called on Reconcile of existing NodeNetworkConfig")
	}
}

func createMockStore() map[MockKey]*nnc.NodeNetworkConfig {
	//Create the mock store
	mockStore = make(map[MockKey]*nnc.NodeNetworkConfig)

	mockKey := MockKey{
		Namespace: existingNamespace,
		Name:      existingNNCName,
	}

	//Fill the mock store with one valid nodenetconfig obj
	mockStore[mockKey] = &nnc.NodeNetworkConfig{ObjectMeta: v1.ObjectMeta{
		Name:      existingNNCName,
		Namespace: existingNamespace,
	}}

	return mockStore
}

func createMockClient() MockClient {
	mockStore := createMockStore()
	// Make mock client initialized with mock store
	mockClient := MockClient{mockStore: mockStore}

	return mockClient
}

func createMockInteractor() MockCNSInteractor {
	return MockCNSInteractor{}
}

func createMockRequestController() *k8sRequestController {
	mockClient := createMockClient()
	mockInteractor := createMockInteractor()

	rc := &k8sRequestController{}
	rc.hostName = existingNNCName
	rc.K8sClient = mockClient
	rc.Reconciler = &NodeNetworkConfigReconciler{}
	rc.Reconciler.K8sClient = mockClient
	rc.Reconciler.CNSInteractor = mockInteractor

	//Initialize logger
	logger.InitLogger("Azure CNS Request Controller", 0, 0, "")

	return rc
}
