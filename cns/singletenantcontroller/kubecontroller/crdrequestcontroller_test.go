package kubecontroller

import (
	"context"
	"errors"
	"fmt"
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
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	existingNNCName      = "nodenetconfig_1"
	existingPodName      = "pod_1"
	hostNetworkPodName   = "pod_hostNet"
	allocatedPodIP       = "10.0.0.2"
	allocatedUUID        = "539970a2-c2dd-11ea-b3de-0242ac130004"
	allocatedUUID2       = "01a5dd00-cd5d-11ea-87d0-0242ac130003"
	networkContainerID   = "24fcd232-0364-41b0-8027-6e6ef9aeabc6"
	existingNamespace    = k8sNamespace
	nonexistingNNCName   = "nodenetconfig_nonexisting"
	nonexistingNamespace = "namespace_nonexisting"
	ncPrimaryIP          = "10.0.0.1"
	subnetRange          = "10.0.0.0/24"
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
func (mc MockKubeClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object) error {
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
func (mc MockKubeClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
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

// MockCNSClient implements API client interface
type MockCNSClient struct {
	MockCNSUpdated     bool
	MockCNSInitialized bool
	Pods               map[string]cns.PodInfo
	NCRequest          *cns.CreateNetworkContainerRequest
}

// we're just testing that reconciler interacts with CNS on Reconcile().
func (mi *MockCNSClient) CreateOrUpdateNC(ncRequest cns.CreateNetworkContainerRequest) error {
	mi.MockCNSUpdated = true
	return nil
}

func (mi *MockCNSClient) UpdateIPAMPoolMonitor(scalar nnc.Scaler, spec nnc.NodeNetworkConfigSpec) error {
	return nil
}

func (mi *MockCNSClient) DeleteNC(nc cns.DeleteNetworkContainerRequest) error {
	return nil
}

func (mi *MockCNSClient) GetNC(nc cns.GetNetworkContainerRequest) (cns.GetNetworkContainerResponse, error) {
	return cns.GetNetworkContainerResponse{NetworkContainerID: nc.NetworkContainerid}, nil
}

func (mi *MockCNSClient) ReconcileNCState(ncRequest *cns.CreateNetworkContainerRequest, podInfoByIP map[string]cns.PodInfo, scalar nnc.Scaler, spec nnc.NodeNetworkConfigSpec) error {
	mi.MockCNSInitialized = true
	mi.Pods = podInfoByIP
	mi.NCRequest = ncRequest
	return nil
}

// MockDirectCRDClient implements the DirectCRDClient interface
var _ DirectCRDClient = &MockDirectCRDClient{}

type MockDirectCRDClient struct {
	mockAPI *MockAPI
}

func (mc *MockDirectCRDClient) Get(ctx context.Context, name, namespace, typeName string) (*nnc.NodeNetworkConfig, error) {
	var (
		mockKey       MockKey
		nodeNetConfig *nnc.NodeNetworkConfig
		ok            bool
	)

	mockKey = MockKey{
		Namespace: namespace,
		Name:      name,
	}

	if nodeNetConfig, ok = mc.mockAPI.nodeNetConfigs[mockKey]; !ok {
		return nil, fmt.Errorf("No nnc by that name in mock client")
	}

	return nodeNetConfig, nil

}

// MockDirectAPIClient implements the DirectAPIClient interface
var _ DirectAPIClient = &MockDirectAPIClient{}

type MockDirectAPIClient struct {
	mockAPI *MockAPI
}

func (mc *MockDirectAPIClient) ListPods(ctx context.Context, namespace, node string) (*corev1.PodList, error) {
	var (
		pod  *corev1.Pod
		pods corev1.PodList
	)

	for _, pod = range mc.mockAPI.pods {
		if namespace == "" || namespace == pod.ObjectMeta.Namespace {
			if pod.Spec.NodeName == node {
				pods.Items = append(pods.Items, *pod)
			}
		}
	}

	if len(pods.Items) == 0 {
		return nil, errors.New("No pods found")
	}

	return &pods, nil
}
func TestNewCrdRequestController(t *testing.T) {
	//Test making request controller without logger initialized, should fail
	_, err := New(Config{})
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

	_, err = New(Config{})
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
	rc := &requestController{
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
	rc := &requestController{
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
	rc := &requestController{
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
	rc := &requestController{
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
	rc := &requestController{
		nodeName:   nonexistingNNCName,
		KubeClient: mockKubeClient,
	}
	logger.InitLogger("Azure CNS RequestController", 0, 0, "")

	spec := nnc.NodeNetworkConfigSpec{
		RequestedIPCount: int64(10),
		IPsNotInUse: []string{
			allocatedUUID,
			allocatedUUID2,
		},
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
	rc := &requestController{
		nodeName:   existingNNCName,
		KubeClient: mockKubeClient,
	}
	logger.InitLogger("Azure CNS RequestController", 0, 0, "")

	spec := nnc.NodeNetworkConfigSpec{
		RequestedIPCount: int64(10),
		IPsNotInUse: []string{
			allocatedUUID,
			allocatedUUID2,
		},
	}

	//Test update spec for existing NodeNetworkConfig
	err := rc.UpdateCRDSpec(context.Background(), spec)

	if err != nil {
		t.Fatalf("Expected no error when updating spec on existing crd, got :%v", err)
	}

	if !reflect.DeepEqual(mockAPI.nodeNetConfigs[mockNNCKey].Spec, spec) {
		t.Fatalf("Expected Spec to equal requested spec update")
	}
}

// test get nnc directly
func TestGetExistingNNCDirectClient(t *testing.T) {
	nodeNetConfigFill := &nnc.NodeNetworkConfig{
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
			mockNNCKey: nodeNetConfigFill,
		},
	}
	mockCRDDirectClient := &MockDirectCRDClient{
		mockAPI: mockAPI,
	}
	rc := &requestController{
		directCRDClient: mockCRDDirectClient,
	}

	nodeNetConfigFetched, err := rc.getNodeNetConfigDirect(context.Background(), existingNNCName, existingNamespace)

	if err != nil {
		t.Fatalf("Expected to be able to get existing nodenetconfig with directCRD client: %v", err)
	}

	if !reflect.DeepEqual(nodeNetConfigFill, nodeNetConfigFetched) {
		t.Fatalf("Expected fetched nodenetconfig to be equal to one we loaded into store")
	}

}

// test get nnc directly non existing
func TestGetNonExistingNNCDirectClient(t *testing.T) {
	nodeNetConfigFill := &nnc.NodeNetworkConfig{
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
			mockNNCKey: nodeNetConfigFill,
		},
	}
	mockCRDDirectClient := &MockDirectCRDClient{
		mockAPI: mockAPI,
	}
	rc := &requestController{
		directCRDClient: mockCRDDirectClient,
	}

	_, err := rc.getNodeNetConfigDirect(context.Background(), nonexistingNNCName, nonexistingNamespace)

	if err == nil {
		t.Fatalf("Expected error when getting non-existing nodenetconfig with direct crd client.")
	}
}

// test get all pods on node
func TestGetPodsExistingNodeDirectClient(t *testing.T) {
	mockPodKey := MockKey{
		Namespace: existingNamespace,
		Name:      existingPodName,
	}
	mockPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      existingPodName,
			Namespace: existingNamespace,
		},
		Status: corev1.PodStatus{
			PodIP: allocatedPodIP,
		},
		Spec: corev1.PodSpec{
			NodeName:    existingNNCName,
			HostNetwork: false,
		},
	}
	mockAPI := &MockAPI{
		pods: map[MockKey]*corev1.Pod{
			mockPodKey: mockPod,
		},
	}
	mockAPIDirectClient := &MockDirectAPIClient{
		mockAPI: mockAPI,
	}
	rc := &requestController{
		directAPIClient: mockAPIDirectClient,
	}

	pods, err := rc.getAllPods(context.Background(), existingNNCName)

	if err != nil {
		t.Fatalf("Expected to be able to get all pods given correct node name")
	}

	if !reflect.DeepEqual(pods.Items[0], *mockPod) {
		t.Fatalf("Expected pods to equal each other when getting all pods on node")
	}
}

func TestGetPodsNonExistingNodeDirectClient(t *testing.T) {
	mockPodKey := MockKey{
		Namespace: existingNamespace,
		Name:      existingPodName,
	}
	mockPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      existingPodName,
			Namespace: existingNamespace,
		},
		Status: corev1.PodStatus{
			PodIP: allocatedPodIP,
		},
		Spec: corev1.PodSpec{
			NodeName:    existingNNCName,
			HostNetwork: false,
		},
	}
	mockAPI := &MockAPI{
		pods: map[MockKey]*corev1.Pod{
			mockPodKey: mockPod,
		},
	}
	mockAPIDirectClient := &MockDirectAPIClient{
		mockAPI: mockAPI,
	}
	rc := &requestController{
		directAPIClient: mockAPIDirectClient,
	}

	_, err := rc.getAllPods(context.Background(), nonexistingNNCName)

	if err == nil {
		t.Fatalf("Expected failure when getting pods of non-existant node")
	}
}

// test that cns init gets called
func TestInitRequestController(t *testing.T) {
	nodeNetConfigFill := &nnc.NodeNetworkConfig{
		ObjectMeta: v1.ObjectMeta{
			Name:      existingNNCName,
			Namespace: existingNamespace,
		},
		Status: nnc.NodeNetworkConfigStatus{
			NetworkContainers: []nnc.NetworkContainer{
				{
					PrimaryIP: ncPrimaryIP,
					ID:        networkContainerID,
					IPAssignments: []nnc.IPAssignment{
						{
							Name: allocatedUUID,
							IP:   allocatedPodIP,
						},
					},
					SubnetAddressSpace: subnetRange,
					Version:            1,
				},
			},
		},
	}
	mockNNCKey := MockKey{
		Namespace: existingNamespace,
		Name:      existingNNCName,
	}
	mockPodKey := MockKey{
		Namespace: existingNamespace,
		Name:      existingPodName,
	}
	mockPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      existingPodName,
			Namespace: existingNamespace,
		},
		Status: corev1.PodStatus{
			PodIP: allocatedPodIP,
		},
		Spec: corev1.PodSpec{
			NodeName:    existingNNCName,
			HostNetwork: false,
		},
	}
	mockPodKeyHostNetwork := MockKey{
		Namespace: existingNamespace,
		Name:      hostNetworkPodName,
	}
	mockPodHostNetwork := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hostNetworkPodName,
			Namespace: existingNamespace,
		},
		Spec: corev1.PodSpec{
			NodeName:    existingNNCName,
			HostNetwork: true,
		},
	}
	mockAPI := &MockAPI{
		nodeNetConfigs: map[MockKey]*nnc.NodeNetworkConfig{
			mockNNCKey: nodeNetConfigFill,
		},
		pods: map[MockKey]*corev1.Pod{
			mockPodKey:            mockPod,
			mockPodKeyHostNetwork: mockPodHostNetwork,
		},
	}
	mockAPIDirectClient := &MockDirectAPIClient{
		mockAPI: mockAPI,
	}
	mockCRDDirectClient := &MockDirectCRDClient{
		mockAPI: mockAPI,
	}
	mockCNSClient := &MockCNSClient{}
	rc := &requestController{
		cfg:             Config{},
		directAPIClient: mockAPIDirectClient,
		directCRDClient: mockCRDDirectClient,
		CNSClient:       mockCNSClient,
		nodeName:        existingNNCName,
	}

	logger.InitLogger("Azure CNS RequestController", 0, 0, "")

	if err := rc.initCNS(context.Background()); err != nil {
		t.Fatalf("Expected no failure to init cns when given mock clients")
	}

	if !mockCNSClient.MockCNSInitialized {
		t.Fatalf("MockCNSClient should have been initialized on request controller init")
	}

	if _, ok := mockCNSClient.Pods[mockPodHostNetwork.Status.PodIP]; ok {
		t.Fatalf("Init shouldn't pass cns pods that are part of host network")
	}

	if _, ok := mockCNSClient.Pods[mockPod.Status.PodIP]; !ok {
		t.Fatalf("Init should pass cns pods that aren't part of host network")
	}

	if _, ok := mockCNSClient.NCRequest.SecondaryIPConfigs[allocatedUUID]; !ok {
		t.Fatalf("Expected secondary ip config to be in ncrequest")
	}

}
