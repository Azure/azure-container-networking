package kubecontroller

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const existingNNCName = "nodenetconfig_1"
const existingPodName = "pod_1"
const allocatedPodIP = "10.0.0.1"
const allocatedUUID = "539970a2-c2dd-11ea-b3de-0242ac130004"
const unallocatedPodIP = "10.0.0.2"
const unallocatedUUID = "deb9de3b-f15f-403f-8a2c-fbe0b8a6af48"
const networkContainerID = "24fcd232-0364-41b0-8027-6e6ef9aeabc6"
const existingNamespace = k8sNamespace
const nonexistingNNCName = "nodenetconfig_nonexisting"
const nonexistingPodName = "pod_nonexisting"
const nonexistingNamespace = "namespace_nonexisting"

var (
	mockAPI        *MockAPI
	mockCNSUpdated bool
	mockCNSReady   bool
)

// MockAPI is a mock of kubernete's API server
type MockAPI struct {
	nodeNetConfigs map[MockKey]*nnc.NodeNetworkConfig
	pods           map[MockKey]*corev1.Pod
}

//MockKey is the key to the mockAPI, namespace+"/"+name like in API server
type MockKey struct {
	Namespace string
	Name      string
}

// MockKubeClient implements KubeClient interface
type MockKubeClient struct {
	mockAPI *MockAPI
}

// Mock implementation of the KubeClient interface Get method
// Mimics that of controller-runtime's client.Client
func (mc MockKubeClient) Get(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
	mockKey := MockKey{
		Namespace: key.Namespace,
		Name:      key.Name,
	}

	nodeNetConfig, ok := mc.mockAPI.nodeNetConfigs[mockKey]
	if !ok {
		return errors.New("Node Net Config not found in mock store")
	}
	nodeNetConfig.DeepCopyInto(obj.(*nnc.NodeNetworkConfig))

	return nil
}

//Mock implementation of the KubeClient interface Update method
//Mimics that of controller-runtime's client.Client
func (mc MockKubeClient) Update(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
	nodeNetConfig := obj.(*nnc.NodeNetworkConfig)

	mockKey := MockKey{
		Namespace: nodeNetConfig.ObjectMeta.Namespace,
		Name:      nodeNetConfig.ObjectMeta.Name,
	}

	_, ok := mockAPI.nodeNetConfigs[mockKey]

	if !ok {
		return errors.New("Node Net Config not found in mock store")
	}

	nodeNetConfig.DeepCopyInto(mockAPI.nodeNetConfigs[mockKey])
	return nil
}

// Mock implementatino of KubeClient List method
func (mc MockKubeClient) List(ctx context.Context, list runtime.Object, opts ...client.ListOption) error {
	podList := &corev1.PodList{}
	for _, pod := range mc.mockAPI.pods {
		podList.Items = append(podList.Items, *pod)
	}

	pods := list.(*corev1.PodList)

	podList.DeepCopyInto(pods)
	return nil
}

// MockCNSClient implements API client interface
type MockCNSClient struct{}

// we're just testing that reconciler interacts with CNS on Reconcile().
func (mc *MockCNSClient) UpdateCNSState(ipConfigs []*cns.ContainerIPConfigState) error {
	mockCNSUpdated = true
	return nil
}

func (mc *MockCNSClient) InitCNSState(ipConfigs []*cns.ContainerIPConfigState) error {
	for _, ipConfig := range ipConfigs {
		if ipConfig.ID == allocatedUUID {
			if ipConfig.State != cns.Allocated {
				return errors.New("Expected allocated ip to be marked as such")
			}
		}
	}
	mockCNSReady = true
	return nil
}

func (mc *MockCNSClient) ReadyToIPAM() bool {
	return mockCNSReady
}

func ResetCNSInteractionFlag() {
	mockCNSUpdated = false
}

func ResetCNSReadyFlag() {
	mockCNSReady = false
}

func ResetReconcileFlags() {
	ResetCNSInteractionFlag()
	ResetCNSReadyFlag()
}

func TestNewCrdRequestController(t *testing.T) {
	//Test making request controller without logger initialized, should fail
	_, err := NewCrdRequestController(nil, nil)
	if err == nil {
		t.Fatalf("Expected error when making NewCrdRequestController without initializing logger, got nil error")
	} else if !strings.Contains(err.Error(), "logger") {
		t.Fatalf("Expected logger error when making NewCrdRequestController without initializing logger, got: %+v", err)
	}

	//Initialize logger
	logger.InitLogger("Azure CRD Request Controller", 3, 3, "")

	//Test making request controller without NODENAME env var set, should fail
	//Save old value though
	nodeName, found := os.LookupEnv(nodeNameEnvVar)
	os.Unsetenv(nodeNameEnvVar)
	defer func() {
		if found {
			os.Setenv(nodeNameEnvVar, nodeName)
		}
	}()

	_, err = NewCrdRequestController(nil, nil)
	if err == nil {
		t.Fatalf("Expected error when making NewCrdRequestController without setting " + nodeNameEnvVar + " env var, got nil error")
	} else if !strings.Contains(err.Error(), nodeNameEnvVar) {
		t.Fatalf("Expected error when making NewCrdRequestController without setting "+nodeNameEnvVar+" env var, got: %+v", err)
	}

	//TODO: Create integration tests with minikube
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

	if !reflect.DeepEqual(nodeNetConfig, mockAPI.nodeNetConfigs[mockKey]) {
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
	nodeNetConfigUpdated := mockAPI.nodeNetConfigs[mockKey].DeepCopy()
	nodeNetConfigUpdated.ObjectMeta.ClusterName = "New cluster name"

	err := rc.updateNodeNetConfig(context.Background(), nodeNetConfigUpdated)
	if err != nil {
		t.Fatalf("Expected no error when updating existing NodeNetworkConfig, got :%v", err)
	}

	//See that NodeNetworkConfig in mock store was updated
	if !reflect.DeepEqual(nodeNetConfigUpdated, mockAPI.nodeNetConfigs[mockKey]) {
		t.Fatal("Update of existing NodeNetworkConfig did not get passed along")
	}
}

func TestUpdateSpecOnNonExistingNodeNetConfig(t *testing.T) {
	rc := createMockRequestController()
	rc.nodeName = nonexistingNNCName

	uuids := make([]string, 3)
	uuids[0] = "uuid0"
	uuids[1] = "uuid1"
	uuids[2] = "uuid2"
	newCount := int64(5)

	spec := &nnc.NodeNetworkConfigSpec{
		RequestedIPCount: newCount,
		IPsNotInUse:      uuids,
	}

	//Test updating spec for existing NodeNetworkConfig
	err := rc.UpdateCRDSpec(context.Background(), spec)

	if err == nil {
		t.Fatalf("Expected error when updating spec on non-existing crd")
	}
}

func TestUpdateSpecOnExistingNodeNetConfig(t *testing.T) {
	rc := createMockRequestController()

	uuids := make([]string, 3)
	uuids[0] = "uuid0"
	uuids[1] = "uuid1"
	uuids[2] = "uuid2"
	newCount := int64(5)

	spec := &nnc.NodeNetworkConfigSpec{
		RequestedIPCount: newCount,
		IPsNotInUse:      uuids,
	}

	//Test releasing ips by uuid for existing NodeNetworkConfig
	err := rc.UpdateCRDSpec(context.Background(), spec)

	if err != nil {
		t.Fatalf("Expected no error when updating spec on existing crd, got :%v", err)
	}

	mockKey := MockKey{
		Namespace: existingNamespace,
		Name:      existingNNCName,
	}

	if !reflect.DeepEqual(mockAPI.nodeNetConfigs[mockKey].Spec.IPsNotInUse, uuids) {
		t.Fatalf("Expected IpsNotInUse to equal requested ips to release")
	}

	if mockAPI.nodeNetConfigs[mockKey].Spec.RequestedIPCount != int64(newCount) {
		t.Fatalf("Expected requested ip count to equal count passed into requested ip count")
	}
}

func TestReconcileNonExistingNNCWhenCNSReady(t *testing.T) {
	rc := createMockRequestController()
	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: nonexistingNamespace,
			Name:      nonexistingNNCName,
		},
	}
	mockCNSReady = true

	_, err := rc.Reconciler.Reconcile(request)

	//Want to reset flags to false for next test
	defer ResetReconcileFlags()

	if err == nil {
		t.Fatalf("Expected error when calling Reconcile for non existing NodeNetworkConfig")
	}
}

func TestReconcileExistingNNCWhenCNSReady(t *testing.T) {
	rc := createMockRequestController()
	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: existingNamespace,
			Name:      existingNNCName,
		},
	}
	mockCNSReady = true

	_, err := rc.Reconciler.Reconcile(request)

	//Want to reset flags to false for next test
	defer ResetReconcileFlags()

	if err != nil {
		t.Fatalf("Expected no error reconciling existing NodeNetworkConfig, got :%v", err)
	}

	if !mockCNSUpdated {
		t.Fatalf("Expected MockCNSInteractor's UpdateCNSState() method to be called on Reconcile of existing NodeNetworkConfig")
	}
}

func TestReconcileExistingNNCWhenCNSNotReady(t *testing.T) {
	//Should init cns state
	rc := createMockRequestController()
	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: existingNamespace,
			Name:      existingNNCName,
		},
	}

	_, err := rc.Reconciler.Reconcile(request)

	//Want to reset flags to false for next test
	defer ResetReconcileFlags()

	if err != nil {
		t.Fatalf("Expected no error reconciling existing NodeNetworkConfig, got :%v", err)
	}

	if mockCNSReady == false {
		t.Fatalf("Expected reconciler to init cns state")
	}
}

func TestReconcileNonExistingNNCWhenCNSNotReady(t *testing.T) {
	//will fail on get
	rc := createMockRequestController()
	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: nonexistingNamespace,
			Name:      nonexistingNNCName,
		},
	}

	_, err := rc.Reconciler.Reconcile(request)

	//Want to reset flags to false for next test
	defer ResetReconcileFlags()

	if err == nil {
		t.Fatalf("Expected error when calling Reconcile for non existing NodeNetworkConfig")
	}

	//Assert that update and ready flags are still false
	if mockCNSUpdated != false {
		t.Fatalf("Expected no update with a non-existing nnc")
	}
	if mockCNSReady != false {
		t.Fatalf("Expected no initialization with a non-existing nnc")
	}
}

func createMockAPI() *MockAPI {
	//Create the mock API
	nodeNetConfigs := make(map[MockKey]*nnc.NodeNetworkConfig)
	pods := make(map[MockKey]*corev1.Pod)
	mockAPI = &MockAPI{
		nodeNetConfigs: nodeNetConfigs,
		pods:           pods,
	}

	mockKey := MockKey{
		Namespace: existingNamespace,
		Name:      existingNNCName,
	}

	//Fill the mock API with one valid nodenetconfig obj
	nnc := &nnc.NodeNetworkConfig{
		ObjectMeta: v1.ObjectMeta{
			Name:      existingNNCName,
			Namespace: existingNamespace,
		},
		Status: nnc.NodeNetworkConfigStatus{
			NetworkContainers: []nnc.NetworkContainer{
				{
					ID: networkContainerID,
					IPAssignments: []nnc.IPAssignment{
						{
							Name: allocatedUUID,
							IP:   allocatedPodIP,
						},
						{
							Name: unallocatedUUID,
							IP:   unallocatedPodIP,
						},
					},
				},
			},
		},
	}
	mockAPI.nodeNetConfigs[mockKey] = nnc

	//Fill the mock API with one valid pod
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      existingPodName,
			Namespace: existingNamespace,
		},
		Status: corev1.PodStatus{
			PodIP: allocatedPodIP,
		},
	}
	mockKey.Name = existingPodName
	mockAPI.pods[mockKey] = pod

	return mockAPI
}

func createMockKubeClient() MockKubeClient {
	mockAPI := createMockAPI()
	// Make mock client initialized with mock API
	MockKubeClient := MockKubeClient{mockAPI: mockAPI}

	return MockKubeClient
}

func createMockCNSClient() *MockCNSClient {
	return &MockCNSClient{}
}

func createMockRequestController() *crdRequestController {
	MockKubeClient := createMockKubeClient()
	MockCNSClient := createMockCNSClient()

	rc := &crdRequestController{}
	rc.nodeName = existingNNCName
	rc.KubeClient = MockKubeClient
	rc.Reconciler = &CrdReconciler{}
	rc.Reconciler.KubeClient = MockKubeClient
	rc.Reconciler.CNSClient = MockCNSClient

	//Initialize logger
	logger.InitLogger("Azure CNS Request Controller", 0, 0, "")

	return rc
}
