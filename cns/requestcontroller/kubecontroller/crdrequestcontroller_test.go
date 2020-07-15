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

const (
	existingNNCName      = "nodenetconfig_1"
	existingPodName      = "pod_1"
	allocatedPodIP       = "10.0.0.1"
	allocatedUUID        = "539970a2-c2dd-11ea-b3de-0242ac130004"
	unallocatedPodIP     = "10.0.0.2"
	unallocatedUUID      = "deb9de3b-f15f-403f-8a2c-fbe0b8a6af48"
	networkContainerID   = "24fcd232-0364-41b0-8027-6e6ef9aeabc6"
	existingNamespace    = k8sNamespace
	nonexistingNNCName   = "nodenetconfig_nonexisting"
	nonexistingPodName   = "pod_nonexisting"
	nonexistingNamespace = "namespace_nonexisting"
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

	_, ok := mc.mockAPI.nodeNetConfigs[mockKey]

	if !ok {
		return errors.New("Node Net Config not found in mock store")
	}

	nodeNetConfig.DeepCopyInto(mc.mockAPI.nodeNetConfigs[mockKey])

	return nil
}

// Mock implementation of KubeClient List method
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
type MockCNSClient struct {
	MockCNSUpdated bool
	MockCNSReady   bool
}

// we're just testing that reconciler interacts with CNS on Reconcile().
func (mc *MockCNSClient) UpdateCNSState(ipConfigs []*cns.ContainerIPConfigState) error {
	mc.MockCNSUpdated = true

	return nil
}

func (mc *MockCNSClient) InitCNSState(ipConfigs []*cns.ContainerIPConfigState) error {
	for _, ipConfig := range ipConfigs {
		if ipConfig.ID == allocatedUUID {
			if ipConfig.State != cns.Allocated {
				return errors.New("Expected allocated ip to be marked allocated")
			}
		} else if ipConfig.ID == unallocatedUUID {
			if ipConfig.State != cns.Available {
				return errors.New("Expected unallocated ip to be marked available")
			}
		}
	}
	mc.MockCNSReady = true

	return nil
}

func (mc *MockCNSClient) ReadyToIPAM() bool {
	return mc.MockCNSReady
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
	nodeNetConfig := &nnc.NodeNetworkConfig{
		ObjectMeta: v1.ObjectMeta{
			Name:      existingNNCName,
			Namespace: existingNamespace,
		},
	}
	mockNNCKey := MockKey{
		Namespace: existingNamespace,
		Name:      existingNNCName,
	}
	mockAPI := &MockAPI{
		nodeNetConfigs: map[MockKey]*nnc.NodeNetworkConfig{
			mockNNCKey: nodeNetConfig,
		},
	}
	mockKubeClient := MockKubeClient{
		mockAPI: mockAPI,
	}
	rc := &crdRequestController{
		KubeClient: mockKubeClient,
	}
	logger.InitLogger("Azure CNS RequestController", 0, 0, "")

	//Test getting nonexisting NodeNetconfig obj
	_, err := rc.getNodeNetConfig(context.Background(), nonexistingNNCName, nonexistingNamespace)
	if err == nil {
		t.Fatalf("Expected error when getting nonexisting nodenetconfig obj. Got nil error.")
	}

}

func TestGetExistingNodeNetConfig(t *testing.T) {
	nodeNetConfig := &nnc.NodeNetworkConfig{
		ObjectMeta: v1.ObjectMeta{
			Name:      existingNNCName,
			Namespace: existingNamespace,
		},
	}
	mockNNCKey := MockKey{
		Namespace: existingNamespace,
		Name:      existingNNCName,
	}
	mockAPI := &MockAPI{
		nodeNetConfigs: map[MockKey]*nnc.NodeNetworkConfig{
			mockNNCKey: nodeNetConfig,
		},
	}
	mockKubeClient := MockKubeClient{
		mockAPI: mockAPI,
	}
	rc := &crdRequestController{
		KubeClient: mockKubeClient,
	}
	logger.InitLogger("Azure CNS RequestController", 0, 0, "")

	//Test getting existing NodeNetConfig obj
	nodeNetConfig, err := rc.getNodeNetConfig(context.Background(), existingNNCName, existingNamespace)
	if err != nil {
		t.Fatalf("Expected no error when getting existing NodeNetworkConfig: %+v", err)
	}

	if !reflect.DeepEqual(nodeNetConfig, mockAPI.nodeNetConfigs[mockNNCKey]) {
		t.Fatalf("Expected fetched node net config to equal one in mock store")
	}
}

func TestUpdateNonExistingNodeNetConfig(t *testing.T) {
	nodeNetConfig := &nnc.NodeNetworkConfig{
		ObjectMeta: v1.ObjectMeta{
			Name:      existingNNCName,
			Namespace: existingNamespace,
		},
	}
	mockNNCKey := MockKey{
		Namespace: existingNamespace,
		Name:      existingNNCName,
	}
	mockAPI := &MockAPI{
		nodeNetConfigs: map[MockKey]*nnc.NodeNetworkConfig{
			mockNNCKey: nodeNetConfig,
		},
	}
	mockKubeClient := MockKubeClient{
		mockAPI: mockAPI,
	}
	rc := &crdRequestController{
		KubeClient: mockKubeClient,
	}
	logger.InitLogger("Azure CNS RequestController", 0, 0, "")

	//Test updating non existing NodeNetworkConfig obj
	nodeNetConfigNonExisting := &nnc.NodeNetworkConfig{ObjectMeta: metav1.ObjectMeta{
		Name:      nonexistingNNCName,
		Namespace: nonexistingNamespace,
	}}

	err := rc.updateNodeNetConfig(context.Background(), nodeNetConfigNonExisting)

	if err == nil {
		t.Fatalf("Expected error when updating non existing NodeNetworkConfig. Got nil error")
	}
}

func TestUpdateExistingNodeNetConfig(t *testing.T) {
	nodeNetConfig := &nnc.NodeNetworkConfig{
		ObjectMeta: v1.ObjectMeta{
			Name:      existingNNCName,
			Namespace: existingNamespace,
		},
	}
	mockNNCKey := MockKey{
		Namespace: existingNamespace,
		Name:      existingNNCName,
	}
	mockAPI := &MockAPI{
		nodeNetConfigs: map[MockKey]*nnc.NodeNetworkConfig{
			mockNNCKey: nodeNetConfig,
		},
	}
	mockKubeClient := MockKubeClient{
		mockAPI: mockAPI,
	}
	rc := &crdRequestController{
		nodeName:   existingNNCName,
		KubeClient: mockKubeClient,
	}
	logger.InitLogger("Azure CNS RequestController", 0, 0, "")

	//Update an existing NodeNetworkConfig obj from the mock API
	nodeNetConfigUpdated := mockAPI.nodeNetConfigs[mockNNCKey].DeepCopy()
	nodeNetConfigUpdated.ObjectMeta.ClusterName = "New cluster name"

	err := rc.updateNodeNetConfig(context.Background(), nodeNetConfigUpdated)
	if err != nil {
		t.Fatalf("Expected no error when updating existing NodeNetworkConfig, got :%v", err)
	}

	//See that NodeNetworkConfig in mock store was updated
	if !reflect.DeepEqual(nodeNetConfigUpdated, mockAPI.nodeNetConfigs[mockNNCKey]) {
		t.Fatal("Update of existing NodeNetworkConfig did not get passed along")
	}
}

func TestUpdateSpecOnNonExistingNodeNetConfig(t *testing.T) {
	nodeNetConfig := &nnc.NodeNetworkConfig{
		ObjectMeta: v1.ObjectMeta{
			Name:      existingNNCName,
			Namespace: existingNamespace,
		},
	}
	mockNNCKey := MockKey{
		Namespace: existingNamespace,
		Name:      existingNNCName,
	}
	mockAPI := &MockAPI{
		nodeNetConfigs: map[MockKey]*nnc.NodeNetworkConfig{
			mockNNCKey: nodeNetConfig,
		},
	}
	mockKubeClient := MockKubeClient{
		mockAPI: mockAPI,
	}
	rc := &crdRequestController{
		nodeName:   nonexistingNNCName,
		KubeClient: mockKubeClient,
	}
	logger.InitLogger("Azure CNS RequestController", 0, 0, "")

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
	nodeNetConfig := &nnc.NodeNetworkConfig{
		ObjectMeta: v1.ObjectMeta{
			Name:      existingNNCName,
			Namespace: existingNamespace,
		},
	}
	mockNNCKey := MockKey{
		Namespace: existingNamespace,
		Name:      existingNNCName,
	}
	mockAPI := &MockAPI{
		nodeNetConfigs: map[MockKey]*nnc.NodeNetworkConfig{
			mockNNCKey: nodeNetConfig,
		},
	}
	mockKubeClient := MockKubeClient{
		mockAPI: mockAPI,
	}
	rc := &crdRequestController{
		nodeName:   existingNNCName,
		KubeClient: mockKubeClient,
	}
	logger.InitLogger("Azure CNS RequestController", 0, 0, "")

	uuids := make([]string, 3)
	uuids[0] = "uuid0"
	uuids[1] = "uuid1"
	uuids[2] = "uuid2"
	newCount := int64(5)

	spec := &nnc.NodeNetworkConfigSpec{
		RequestedIPCount: newCount,
		IPsNotInUse:      uuids,
	}

	//Test update spec for existing NodeNetworkConfig
	err := rc.UpdateCRDSpec(context.Background(), spec)

	if err != nil {
		t.Fatalf("Expected no error when updating spec on existing crd, got :%v", err)
	}

	if !reflect.DeepEqual(mockAPI.nodeNetConfigs[mockNNCKey].Spec, *spec) {
		t.Fatalf("Expected Spec to equal requested spec update")
	}
}

func TestReconcileNonExistingNNCWhenCNSReady(t *testing.T) {
	nodeNetConfig := &nnc.NodeNetworkConfig{
		ObjectMeta: v1.ObjectMeta{
			Name:      existingNNCName,
			Namespace: existingNamespace,
		},
	}
	mockNNCKey := MockKey{
		Namespace: existingNamespace,
		Name:      existingNNCName,
	}
	mockAPI := &MockAPI{
		nodeNetConfigs: map[MockKey]*nnc.NodeNetworkConfig{
			mockNNCKey: nodeNetConfig,
		},
	}
	mockKubeClient := MockKubeClient{
		mockAPI: mockAPI,
	}
	//Set mockCNSState to ready
	mockCNSClient := &MockCNSClient{
		MockCNSReady: true,
	}
	rc := &crdRequestController{
		nodeName:   nonexistingNNCName,
		KubeClient: mockKubeClient,
		Reconciler: &CrdReconciler{
			KubeClient: mockKubeClient,
			NodeName:   nonexistingNNCName,
			Namespace:  nonexistingNamespace,
			CNSClient:  mockCNSClient,
		},
	}
	logger.InitLogger("Azure CNS RequestController", 0, 0, "")

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

func TestReconcileExistingNNCWhenCNSReady(t *testing.T) {
	nodeNetConfig := &nnc.NodeNetworkConfig{
		ObjectMeta: v1.ObjectMeta{
			Name:      existingNNCName,
			Namespace: existingNamespace,
		},
	}
	mockNNCKey := MockKey{
		Namespace: existingNamespace,
		Name:      existingNNCName,
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      existingPodName,
			Namespace: existingNamespace,
		},
		Status: corev1.PodStatus{
			PodIP: allocatedPodIP,
		},
	}
	mockPodKey := MockKey{
		Namespace: existingNamespace,
		Name:      existingPodName,
	}
	mockAPI := &MockAPI{
		nodeNetConfigs: map[MockKey]*nnc.NodeNetworkConfig{
			mockNNCKey: nodeNetConfig,
		},
		pods: map[MockKey]*corev1.Pod{
			mockPodKey: pod,
		},
	}
	mockKubeClient := MockKubeClient{
		mockAPI: mockAPI,
	}
	//Set mockCNSState to ready
	mockCNSClient := &MockCNSClient{
		MockCNSReady: true,
	}
	rc := &crdRequestController{
		nodeName:   nonexistingNNCName,
		KubeClient: mockKubeClient,
		Reconciler: &CrdReconciler{
			KubeClient: mockKubeClient,
			NodeName:   existingNNCName,
			Namespace:  existingNamespace,
			CNSClient:  mockCNSClient,
		},
	}
	logger.InitLogger("Azure CNS RequestController", 0, 0, "")

	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: existingNamespace,
			Name:      existingNNCName,
		},
	}

	_, err := rc.Reconciler.Reconcile(request)

	if err != nil {
		t.Fatalf("Expected no error reconciling existing NodeNetworkConfig, got :%v", err)
	}

	if !mockCNSClient.MockCNSUpdated {
		t.Fatalf("Expected MockCNSInteractor's UpdateCNSState() method to be called on Reconcile of existing NodeNetworkConfig")
	}
}

func TestReconcileExistingNNCWhenCNSNotReady(t *testing.T) {
	//Should init cns state
	nodeNetConfig := &nnc.NodeNetworkConfig{
		ObjectMeta: v1.ObjectMeta{
			Name:      existingNNCName,
			Namespace: existingNamespace,
		},
	}
	mockNNCKey := MockKey{
		Namespace: existingNamespace,
		Name:      existingNNCName,
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      existingPodName,
			Namespace: existingNamespace,
		},
		Status: corev1.PodStatus{
			PodIP: allocatedPodIP,
		},
	}
	mockPodKey := MockKey{
		Namespace: existingNamespace,
		Name:      existingPodName,
	}
	mockAPI := &MockAPI{
		nodeNetConfigs: map[MockKey]*nnc.NodeNetworkConfig{
			mockNNCKey: nodeNetConfig,
		},
		pods: map[MockKey]*corev1.Pod{
			mockPodKey: pod,
		},
	}
	mockKubeClient := MockKubeClient{
		mockAPI: mockAPI,
	}
	mockCNSClient := &MockCNSClient{
		MockCNSReady: false,
	}
	rc := &crdRequestController{
		nodeName:   nonexistingNNCName,
		KubeClient: mockKubeClient,
		Reconciler: &CrdReconciler{
			KubeClient: mockKubeClient,
			NodeName:   existingNNCName,
			Namespace:  existingNamespace,
			CNSClient:  mockCNSClient,
		},
	}
	logger.InitLogger("Azure CNS RequestController", 0, 0, "")

	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: existingNamespace,
			Name:      existingNNCName,
		},
	}

	_, err := rc.Reconciler.Reconcile(request)

	if err != nil {
		t.Fatalf("Expected no error reconciling existing NodeNetworkConfig, got :%v", err)
	}

	if mockCNSClient.MockCNSReady == false {
		t.Fatalf("Expected reconciler to init cns state")
	}
}

func TestReconcileNonExistingNNCWhenCNSNotReady(t *testing.T) {
	//will fail on get
	nodeNetConfig := &nnc.NodeNetworkConfig{
		ObjectMeta: v1.ObjectMeta{
			Name:      existingNNCName,
			Namespace: existingNamespace,
		},
	}
	mockNNCKey := MockKey{
		Namespace: existingNamespace,
		Name:      existingNNCName,
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      existingPodName,
			Namespace: existingNamespace,
		},
		Status: corev1.PodStatus{
			PodIP: allocatedPodIP,
		},
	}
	mockPodKey := MockKey{
		Namespace: existingNamespace,
		Name:      existingPodName,
	}
	mockAPI := &MockAPI{
		nodeNetConfigs: map[MockKey]*nnc.NodeNetworkConfig{
			mockNNCKey: nodeNetConfig,
		},
		pods: map[MockKey]*corev1.Pod{
			mockPodKey: pod,
		},
	}
	mockKubeClient := MockKubeClient{
		mockAPI: mockAPI,
	}
	mockCNSClient := &MockCNSClient{
		MockCNSUpdated: false,
		MockCNSReady:   false,
	}
	rc := &crdRequestController{
		nodeName:   nonexistingNNCName,
		KubeClient: mockKubeClient,
		Reconciler: &CrdReconciler{
			KubeClient: mockKubeClient,
			NodeName:   nonexistingNNCName,
			Namespace:  nonexistingNamespace,
			CNSClient:  mockCNSClient,
		},
	}
	logger.InitLogger("Azure CNS RequestController", 0, 0, "")

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

	//Assert that update and ready flags are still false
	if mockCNSClient.MockCNSUpdated != false {
		t.Fatalf("Expected no update with a non-existing nnc")
	}
	if mockCNSClient.MockCNSReady != false {
		t.Fatalf("Expected no initialization with a non-existing nnc")
	}
}
