package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/common"
	"github.com/Azure/azure-container-networking/cns/fakes"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/restserver"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/Azure/azure-container-networking/log"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	svc           *restserver.HTTPRestService
	errBadRequest = errors.New("bad request")
)

const (
	primaryIp           = "10.0.0.5"
	gatewayIp           = "10.0.0.1"
	subnetPrfixLength   = 24
	dockerContainerType = cns.Docker
	releasePercent      = 150
	requestPercent      = 50
	batchSize           = 10
	initPoolSize        = 10
)

var dnsservers = []string{"8.8.8.8", "8.8.4.4"}

type mockdo struct {
	errToReturn            error
	objToReturn            interface{}
	httpStatusCodeToReturn int
}

func (m *mockdo) Do(req *http.Request) (*http.Response, error) {
	byteArray, _ := json.Marshal(m.objToReturn)
	body := io.NopCloser(bytes.NewReader(byteArray))

	return &http.Response{
		StatusCode: m.httpStatusCodeToReturn,
		Body:       body,
	}, m.errToReturn
}

func addTestStateToRestServer(t *testing.T, secondaryIps []string) {
	var ipConfig cns.IPConfiguration
	ipConfig.DNSServers = dnsservers
	ipConfig.GatewayIPAddress = gatewayIp
	var ipSubnet cns.IPSubnet
	ipSubnet.IPAddress = primaryIp
	ipSubnet.PrefixLength = subnetPrfixLength
	ipConfig.IPSubnet = ipSubnet
	secondaryIPConfigs := make(map[string]cns.SecondaryIPConfig)

	for _, secIpAddress := range secondaryIps {
		secIpConfig := cns.SecondaryIPConfig{
			IPAddress: secIpAddress,
			NCVersion: -1,
		}
		ipId := uuid.New()
		secondaryIPConfigs[ipId.String()] = secIpConfig
	}

	req := &cns.CreateNetworkContainerRequest{
		NetworkContainerType: dockerContainerType,
		NetworkContainerid:   "testNcId1",
		IPConfiguration:      ipConfig,
		SecondaryIPConfigs:   secondaryIPConfigs,
		// Set it as -1 to be same as default host version.
		// It will allow secondary IPs status to be set as available.
		Version: "-1",
	}

	returnCode := svc.CreateOrUpdateNetworkContainerInternal(req)
	if returnCode != 0 {
		t.Fatalf("Failed to createNetworkContainerRequest, req: %+v, err: %d", req, returnCode)
	}

	_ = svc.IPAMPoolMonitor.Update(&v1alpha.NodeNetworkConfig{
		Spec: v1alpha.NodeNetworkConfigSpec{
			RequestedIPCount: 16,
			IPsNotInUse:      []string{"abc"},
		},
		Status: v1alpha.NodeNetworkConfigStatus{
			Scaler: v1alpha.Scaler{
				BatchSize:               batchSize,
				ReleaseThresholdPercent: releasePercent,
				RequestThresholdPercent: requestPercent,
				MaxIPCount:              250,
			},
			NetworkContainers: []v1alpha.NetworkContainer{
				{},
			},
		},
	})
}

func getIPNetFromResponse(resp *cns.IPConfigResponse) (net.IPNet, error) {
	var (
		resultIPnet net.IPNet
		err         error
	)

	// set result ipconfig from CNS Response Body
	prefix := strconv.Itoa(int(resp.PodIpInfo.PodIPConfig.PrefixLength))
	ip, ipnet, err := net.ParseCIDR(resp.PodIpInfo.PodIPConfig.IPAddress + "/" + prefix)
	if err != nil {
		return resultIPnet, err
	}

	// construct ipnet for result
	resultIPnet = net.IPNet{
		IP:   ip,
		Mask: ipnet.Mask,
	}
	return resultIPnet, err
}

func TestMain(m *testing.M) {
	var (
		info = &cns.SetOrchestratorTypeRequest{
			OrchestratorType: cns.KubernetesCRD,
		}
		body bytes.Buffer
		res  *http.Response
	)

	tmpFileState, err := os.CreateTemp(os.TempDir(), "cns-*.json")
	tmpLogDir, err := os.MkdirTemp("", "cns-")
	fmt.Printf("logdir: %+v", tmpLogDir)

	if err != nil {
		panic(err)
	}

	defer os.RemoveAll(tmpLogDir)
	defer os.Remove(tmpFileState.Name())

	logName := "azure-cns.log"
	fmt.Printf("Test logger file: %v", tmpLogDir+"/"+logName)
	fmt.Printf("Test state :%v", tmpFileState.Name())

	if err != nil {
		panic(err)
	}

	logger.InitLogger(logName, 0, 0, tmpLogDir+"/")
	config := common.ServiceConfig{}

	httpRestService, err := restserver.NewHTTPRestService(&config, &fakes.WireserverClientFake{}, &fakes.NMAgentClientFake{}, nil)
	svc = httpRestService.(*restserver.HTTPRestService)
	svc.Name = "cns-test-server"
	fakeNNC := v1alpha.NodeNetworkConfig{
		TypeMeta:   metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{},
		Spec: v1alpha.NodeNetworkConfigSpec{
			RequestedIPCount: 16,
			IPsNotInUse:      []string{"abc"},
		},
		Status: v1alpha.NodeNetworkConfigStatus{
			Scaler: v1alpha.Scaler{
				BatchSize:               10,
				ReleaseThresholdPercent: 150,
				RequestThresholdPercent: 50,
				MaxIPCount:              250,
			},
			NetworkContainers: []v1alpha.NetworkContainer{
				{
					ID:         "nc1",
					PrimaryIP:  "10.0.0.11",
					SubnetName: "sub1",
					IPAssignments: []v1alpha.IPAssignment{
						{
							Name: "ip1",
							IP:   "10.0.0.10",
						},
					},
					DefaultGateway:     "10.0.0.1",
					SubnetAddressSpace: "10.0.0.0/24",
					Version:            2,
				},
			},
		},
	}
	svc.IPAMPoolMonitor = &fakes.MonitorFake{IPsNotInUseCount: 13, NodeNetworkConfig: &fakeNNC}

	if err != nil {
		logger.Errorf("Failed to create CNS object, err:%v.\n", err)
		return
	}

	if httpRestService != nil {
		err = httpRestService.Init(&config)
		if err != nil {
			logger.Errorf("Failed to initialize HttpService, err:%v.\n", err)
			return
		}

		err = httpRestService.Start(&config)
		if err != nil {
			logger.Errorf("Failed to start HttpService, err:%v.\n", err)
			return
		}
	}

	if err := json.NewEncoder(&body).Encode(info); err != nil {
		log.Errorf("encoding json failed with %v", err)
		return
	}

	httpc := &http.Client{}
	url := defaultBaseURL + cns.SetOrchestratorType

	res, err = httpc.Post(url, "application/json", &body)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(res)

	exitCode := m.Run()
	os.Exit(exitCode)
}

func TestCNSClientRequestAndRelease(t *testing.T) {
	podName := "testpodname"
	podNamespace := "testpodnamespace"
	desiredIpAddress := "10.0.0.5"
	ip := net.ParseIP(desiredIpAddress)
	_, ipnet, _ := net.ParseCIDR("10.0.0.5/24")
	desired := net.IPNet{
		IP:   ip,
		Mask: ipnet.Mask,
	}

	secondaryIps := make([]string, 0)
	secondaryIps = append(secondaryIps, desiredIpAddress)
	cnsClient, _ := New("", 2*time.Hour)

	addTestStateToRestServer(t, secondaryIps)

	podInfo := cns.KubernetesPodInfo{PodName: podName, PodNamespace: podNamespace}
	orchestratorContext, err := json.Marshal(podInfo)
	assert.NoError(t, err)

	// no IP reservation found with that context, expect no failure.
	err = cnsClient.ReleaseIPAddress(context.TODO(), cns.IPConfigRequest{OrchestratorContext: orchestratorContext})
	assert.NoError(t, err, "Release ip idempotent call failed")

	// request IP address
	resp, err := cnsClient.RequestIPAddress(context.TODO(), cns.IPConfigRequest{OrchestratorContext: orchestratorContext})
	assert.NoError(t, err, "get IP from CNS failed")

	podIPInfo := resp.PodIpInfo
	assert.Equal(t, primaryIp, podIPInfo.NetworkContainerPrimaryIPConfig.IPSubnet.IPAddress, "PrimaryIP is not added as epected ipConfig")
	assert.EqualValues(t, podIPInfo.NetworkContainerPrimaryIPConfig.IPSubnet.PrefixLength, subnetPrfixLength, "Primary IP Prefix length is not added as expected ipConfig")

	// validate DnsServer and Gateway Ip as the same configured for Primary IP
	assert.Equal(t, dnsservers, podIPInfo.NetworkContainerPrimaryIPConfig.DNSServers, "DnsServer is not added as expected ipConfig")
	assert.Equal(t, gatewayIp, podIPInfo.NetworkContainerPrimaryIPConfig.GatewayIPAddress, "Gateway is not added as expected ipConfig")

	resultIPnet, err := getIPNetFromResponse(resp)

	assert.Equal(t, desired, resultIPnet, "Desired result not matching actual result")

	// checking for assigned IP address and pod context printing before ReleaseIPAddress is called
	ipaddresses, err := cnsClient.GetIPAddressesMatchingStates(context.TODO(), types.Assigned)
	assert.NoError(t, err, "Get assigned IP addresses failed")

	assert.Len(t, ipaddresses, 1, "Number of available IP addresses expected to be 1")
	assert.Equal(t, desiredIpAddress, ipaddresses[0].IPAddress, "Available IP address does not match expected, address state")
	assert.Equal(t, types.Assigned, ipaddresses[0].GetState(), "Available IP address does not match expected, address state")

	t.Log(ipaddresses)

	// release requested IP address, expect success
	err = cnsClient.ReleaseIPAddress(context.TODO(), cns.IPConfigRequest{DesiredIPAddress: ipaddresses[0].IPAddress, OrchestratorContext: orchestratorContext})
	assert.NoError(t, err, "Expected to not fail when releasing IP reservation found with context")
}

func TestCNSClientPodContextApi(t *testing.T) {
	podName := "testpodname"
	podNamespace := "testpodnamespace"
	desiredIpAddress := "10.0.0.5"

	secondaryIps := []string{desiredIpAddress}
	cnsClient, _ := New("", 2*time.Second)

	addTestStateToRestServer(t, secondaryIps)

	podInfo := cns.NewPodInfo("", "", podName, podNamespace)
	orchestratorContext, err := json.Marshal(podInfo)
	assert.NoError(t, err)

	// request IP address
	_, err = cnsClient.RequestIPAddress(context.TODO(), cns.IPConfigRequest{OrchestratorContext: orchestratorContext})
	assert.NoError(t, err, "get IP from CNS failed")

	// test for pod ip by orch context map
	podcontext, err := cnsClient.GetPodOrchestratorContext(context.TODO())
	assert.NoError(t, err, "Get pod ip by orchestrator context failed")
	assert.GreaterOrEqual(t, len(podcontext), 1, "Expected at least 1 entry in map for podcontext")

	t.Log(podcontext)

	// release requested IP address, expect success
	err = cnsClient.ReleaseIPAddress(context.TODO(), cns.IPConfigRequest{OrchestratorContext: orchestratorContext})
	assert.NoError(t, err, "Expected to not fail when releasing IP reservation found with context")
}

func TestCNSClientDebugAPI(t *testing.T) {
	podName := "testpodname"
	podNamespace := "testpodnamespace"
	desiredIpAddress := "10.0.0.5"

	secondaryIps := []string{desiredIpAddress}
	cnsClient, _ := New("", 2*time.Hour)

	addTestStateToRestServer(t, secondaryIps)

	podInfo := cns.NewPodInfo("", "", podName, podNamespace)
	orchestratorContext, err := json.Marshal(podInfo)
	assert.NoError(t, err)

	// request IP address
	_, err1 := cnsClient.RequestIPAddress(context.TODO(), cns.IPConfigRequest{OrchestratorContext: orchestratorContext})
	assert.NoError(t, err1, "get IP from CNS failed")

	// test for debug api/cmd to get inmemory data from HTTPRestService
	inmemory, err := cnsClient.GetHTTPServiceData(context.TODO())
	assert.NoError(t, err, "Get in-memory http REST Struct failed")

	assert.GreaterOrEqual(t, len(inmemory.HTTPRestServiceData.PodIPIDByPodInterfaceKey), 1, "OrchestratorContext map is expected but not returned")

	// testing Pod IP Configuration Status values set for test
	podConfig := inmemory.HTTPRestServiceData.PodIPConfigState
	for _, v := range podConfig {
		assert.Equal(t, "10.0.0.5", v.IPAddress, "Not the expected set values for testing IPConfigurationStatus, %+v", podConfig)
		assert.Equal(t, types.Assigned, v.GetState(), "Not the expected set values for testing IPConfigurationStatus, %+v", podConfig)
		assert.Equal(t, "testNcId1", v.NCID, "Not the expected set values for testing IPConfigurationStatus, %+v", podConfig)
	}
	assert.GreaterOrEqual(t, len(inmemory.HTTPRestServiceData.PodIPConfigState), 1, "PodIpConfigState with at least 1 entry expected")

	testIpamPoolMonitor := inmemory.HTTPRestServiceData.IPAMPoolMonitor
	assert.EqualValues(t, 5, testIpamPoolMonitor.MinimumFreeIps, "IPAMPoolMonitor state is not reflecting the initial set values")
	assert.EqualValues(t, 15, testIpamPoolMonitor.MaximumFreeIps, "IPAMPoolMonitor state is not reflecting the initial set values")
	assert.EqualValues(t, 13, testIpamPoolMonitor.UpdatingIpsNotInUseCount, "IPAMPoolMonitor state is not reflecting the initial set values")

	// check for cached NNC Spec struct values
	assert.EqualValues(t, 16, testIpamPoolMonitor.CachedNNC.Spec.RequestedIPCount, "IPAMPoolMonitor cached NNC Spec is not reflecting the initial set values")
	assert.Len(t, testIpamPoolMonitor.CachedNNC.Spec.IPsNotInUse, 1, "IPAMPoolMonitor cached NNC Spec is not reflecting the initial set values")

	// check for cached NNC Status struct values
	assert.EqualValues(t, 10, testIpamPoolMonitor.CachedNNC.Status.Scaler.BatchSize, "IPAMPoolMonitor cached NNC Status is not reflecting the initial set values")
	assert.EqualValues(t, 150, testIpamPoolMonitor.CachedNNC.Status.Scaler.ReleaseThresholdPercent, "IPAMPoolMonitor cached NNC Status is not reflecting the initial set values")
	assert.EqualValues(t, 50, testIpamPoolMonitor.CachedNNC.Status.Scaler.RequestThresholdPercent, "IPAMPoolMonitor cached NNC Status is not reflecting the initial set values")
	assert.Len(t, testIpamPoolMonitor.CachedNNC.Status.NetworkContainers, 1, "Expected only one Network Container in the list")

	t.Logf("In-memory Data: ")
	t.Logf("PodIPIDByOrchestratorContext: %+v", inmemory.HTTPRestServiceData.PodIPIDByPodInterfaceKey)
	t.Logf("PodIPConfigState: %+v", inmemory.HTTPRestServiceData.PodIPConfigState)
	t.Logf("IPAMPoolMonitor: %+v", inmemory.HTTPRestServiceData.IPAMPoolMonitor)
}

func TestNew(t *testing.T) {
	fqdnBaseURL := "http://testinstance.centraluseuap.cloudapp.azure.com"
	fqdnWithPortBaseURL := fqdnBaseURL + ":10090"
	emptyRoutes, _ := buildRoutes(defaultBaseURL, clientPaths)
	fqdnRoutes, _ := buildRoutes(fqdnBaseURL, clientPaths)
	fqdnWithPortRoutes, _ := buildRoutes(fqdnWithPortBaseURL, clientPaths)
	tests := []struct {
		name    string
		url     string
		timeout time.Duration
		want    *Client
		wantErr bool
	}{
		{
			name: "empty url",
			url:  "",
			want: &Client{
				routes: emptyRoutes,
				client: &http.Client{
					Timeout: 0,
				},
			},
			wantErr: false,
		},
		{
			name: "FQDN",
			url:  fqdnBaseURL,
			want: &Client{
				routes: fqdnRoutes,
				client: &http.Client{
					Timeout: 0,
				},
			},
			wantErr: false,
		},
		{
			name: "FQDN with port",
			url:  fqdnWithPortBaseURL,
			want: &Client{
				routes: fqdnWithPortRoutes,
				client: &http.Client{
					Timeout: 0,
				},
			},
			wantErr: false,
		},
		{
			name:    "bad path",
			url:     "postgres://user:abc{DEf1=ghi@example.com:5432/db?sslmode=require",
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got, err := New(tt.url, tt.timeout)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildRoutes(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		paths   []string
		want    map[string]url.URL
		wantErr bool
	}{
		{
			name:    "default base url",
			baseURL: "http://localhost:10090",
			paths: []string{
				"/test/path",
			},
			want: map[string]url.URL{
				"/test/path": {
					Scheme: "http",
					Host:   "localhost:10090",
					Path:   "/test/path",
				},
			},
			wantErr: false,
		},
		{
			name:    "empty base url",
			baseURL: "",
			paths: []string{
				"/test/path",
			},
			want: map[string]url.URL{
				"/test/path": {
					Path: "/test/path",
				},
			},
			wantErr: false,
		},
		{
			name:    "bad base url",
			baseURL: "postgres://user:abc{DEf1=ghi@example.com:5432/db?sslmode=require",
			paths: []string{
				"/test/path",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:    "bad path",
			baseURL: "http://localhost:10090",
			paths: []string{
				"postgres://user:abc{DEf1=ghi@example.com:5432/db?sslmode=require",
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildRoutes(tt.baseURL, tt.paths)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetNetworkConfiguration(t *testing.T) {
	emptyRoutes, _ := buildRoutes(defaultBaseURL, clientPaths)
	tests := []struct {
		name    string
		ctx     context.Context
		podInfo cns.KubernetesPodInfo
		mockdo  *mockdo
		routes  map[string]url.URL
		want    *cns.GetNetworkContainerResponse
		wantErr bool
	}{
		{
			name: "existing pod info",
			ctx:  context.TODO(),
			podInfo: cns.KubernetesPodInfo{
				PodName:      "testpodname",
				PodNamespace: "podNamespace",
			},
			mockdo: &mockdo{
				errToReturn:            nil,
				objToReturn:            &cns.GetNetworkContainerResponse{},
				httpStatusCodeToReturn: http.StatusOK,
			},
			routes:  emptyRoutes,
			want:    &cns.GetNetworkContainerResponse{},
			wantErr: false,
		},
		{
			name: "bad request",
			ctx:  context.TODO(),
			podInfo: cns.KubernetesPodInfo{
				PodName:      "testpodname",
				PodNamespace: "podNamespace",
			},
			mockdo: &mockdo{
				errToReturn:            errBadRequest,
				objToReturn:            nil,
				httpStatusCodeToReturn: http.StatusBadRequest,
			},
			routes:  emptyRoutes,
			want:    nil,
			wantErr: true,
		},
		{
			name: "bad decoding",
			ctx:  context.TODO(),
			podInfo: cns.KubernetesPodInfo{
				PodName:      "testpodname",
				PodNamespace: "podNamespace",
			},
			mockdo: &mockdo{
				errToReturn:            nil,
				objToReturn:            []cns.GetNetworkContainerResponse{},
				httpStatusCodeToReturn: http.StatusOK,
			},
			routes:  emptyRoutes,
			want:    nil,
			wantErr: true,
		},
		{
			name: "http status not ok",
			ctx:  context.TODO(),
			podInfo: cns.KubernetesPodInfo{
				PodName:      "testpodname",
				PodNamespace: "podNamespace",
			},
			mockdo: &mockdo{
				errToReturn:            nil,
				objToReturn:            nil,
				httpStatusCodeToReturn: http.StatusInternalServerError,
			},
			routes:  emptyRoutes,
			want:    nil,
			wantErr: true,
		},
		{
			name: "cns return code not zero",
			ctx:  context.TODO(),
			podInfo: cns.KubernetesPodInfo{
				PodName:      "testpodname",
				PodNamespace: "podNamespace",
			},
			mockdo: &mockdo{
				errToReturn: nil,
				objToReturn: &cns.GetNetworkContainerResponse{
					Response: cns.Response{
						ReturnCode: types.UnsupportedNetworkType,
					},
				},
				httpStatusCodeToReturn: http.StatusOK,
			},
			routes:  emptyRoutes,
			want:    nil,
			wantErr: true,
		},
		{
			name: "nil context",
			ctx:  nil,
			podInfo: cns.KubernetesPodInfo{
				PodName:      "testpodname",
				PodNamespace: "podNamespace",
			},
			mockdo:  &mockdo{},
			routes:  emptyRoutes,
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			client := Client{
				client: tt.mockdo,
				routes: tt.routes,
			}

			orchestratorContext, err := json.Marshal(tt.podInfo)
			assert.NoError(t, err, "marshaling orchestrator context failed")

			got, err := client.GetNetworkConfiguration(tt.ctx, orchestratorContext)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCreateHostNCApipaEndpoint(t *testing.T) {
	emptyRoutes, _ := buildRoutes(defaultBaseURL, clientPaths)
	tests := []struct {
		name               string
		ctx                context.Context
		networkContainerID string
		mockdo             *mockdo
		routes             map[string]url.URL
		want               string
		wantErr            bool
	}{
		{
			name:               "happy case",
			ctx:                context.TODO(),
			networkContainerID: "testncid",
			mockdo: &mockdo{
				errToReturn:            nil,
				objToReturn:            &cns.CreateHostNCApipaEndpointResponse{},
				httpStatusCodeToReturn: http.StatusOK,
			},
			routes:  emptyRoutes,
			want:    "",
			wantErr: false,
		},
		{
			name:               "bad request",
			ctx:                context.TODO(),
			networkContainerID: "testncid",
			mockdo: &mockdo{
				errToReturn:            errBadRequest,
				objToReturn:            nil,
				httpStatusCodeToReturn: http.StatusBadRequest,
			},
			routes:  emptyRoutes,
			want:    "",
			wantErr: true,
		},
		{
			name:               "bad decoding",
			ctx:                context.TODO(),
			networkContainerID: "testncid",
			mockdo: &mockdo{
				errToReturn:            nil,
				objToReturn:            []cns.CreateHostNCApipaEndpointResponse{},
				httpStatusCodeToReturn: http.StatusOK,
			},
			routes:  emptyRoutes,
			want:    "",
			wantErr: true,
		},
		{
			name:               "http status not ok",
			ctx:                context.TODO(),
			networkContainerID: "testncid",
			mockdo: &mockdo{
				errToReturn:            nil,
				objToReturn:            nil,
				httpStatusCodeToReturn: http.StatusInternalServerError,
			},
			routes:  emptyRoutes,
			want:    "",
			wantErr: true,
		},
		{
			name:               "cns return code not zero",
			ctx:                context.TODO(),
			networkContainerID: "testncid",
			mockdo: &mockdo{
				errToReturn: nil,
				objToReturn: &cns.CreateHostNCApipaEndpointResponse{
					Response: cns.Response{
						ReturnCode: types.UnsupportedNetworkType,
					},
				},
				httpStatusCodeToReturn: http.StatusOK,
			},
			routes:  emptyRoutes,
			want:    "",
			wantErr: true,
		},
		{
			name:               "nil context",
			ctx:                nil,
			networkContainerID: "testncid",
			mockdo:             &mockdo{},
			routes:             emptyRoutes,
			want:               "",
			wantErr:            true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			client := Client{
				client: tt.mockdo,
				routes: tt.routes,
			}
			got, err := client.CreateHostNCApipaEndpoint(tt.ctx, tt.networkContainerID)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

type RequestCapture struct {
	Request *http.Request
	Next    interface {
		Do(*http.Request) (*http.Response, error)
	}
}

// Do captures the outgoing HTTP request for later examination within a test.
func (r *RequestCapture) Do(req *http.Request) (*http.Response, error) {
	// clone the request to ensure that any downstream handlers can't modify what
	// we've captured. Clone requires a non-nil context argument, so use a
	// throwaway value.
	r.Request = req.Clone(context.Background())

	// invoke the next handler in the chain and transparently return its results
	//nolint:wrapcheck // we don't need error wrapping for tests
	return r.Next.Do(req)
}

func TestDeleteNetworkContainer(t *testing.T) {
	// the CNS client has to be provided with routes going somewhere, so create a
	// bunch of routes mapped to the localhost
	emptyRoutes, _ := buildRoutes(defaultBaseURL, clientPaths)

	// define our test cases
	deleteNCTests := []struct {
		name      string
		ncID      string
		response  *RequestCapture
		expReq    *cns.DeleteNetworkContainerRequest
		shouldErr bool
	}{
		{
			"empty",
			"",
			&RequestCapture{
				Next: &mockdo{},
			},
			nil,
			true,
		},
		{
			"with id",
			"foo",
			&RequestCapture{
				Next: &mockdo{
					httpStatusCodeToReturn: http.StatusOK,
				},
			},
			&cns.DeleteNetworkContainerRequest{
				NetworkContainerid: "foo",
			},
			false,
		},
		{
			"unspecified error",
			"foo",
			&RequestCapture{
				Next: &mockdo{
					errToReturn: nil,
					objToReturn: cns.DeleteNetworkContainerResponse{
						Response: cns.Response{
							ReturnCode: types.MalformedSubnet,
						},
					},
					httpStatusCodeToReturn: http.StatusBadRequest,
				},
			},
			&cns.DeleteNetworkContainerRequest{
				NetworkContainerid: "foo",
			},
			true,
		},
	}

	for _, test := range deleteNCTests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// create a new client with the mock routes and the mock doer
			client := Client{
				client: test.response,
				routes: emptyRoutes,
			}

			// execute the method under test
			err := client.DeleteNetworkContainer(context.TODO(), test.ncID)
			if err != nil && !test.shouldErr {
				t.Fatal("unexpected error: err:", err)
			}

			if err == nil && test.shouldErr {
				t.Fatal("expected test to error, but no error was produced")
			}

			// make sure a request was actually sent
			if test.expReq != nil && test.response.Request == nil {
				t.Fatal("expected a request to be sent, but none was")
			}

			// if a request was expected to be sent, decode it and ensure that it
			// matches expectations
			if test.expReq != nil {
				var gotReq cns.DeleteNetworkContainerRequest
				err = json.NewDecoder(test.response.Request.Body).Decode(&gotReq)
				if err != nil {
					t.Fatal("error decoding the received request: err:", err)
				}

				// a nil expReq is semantically meaningful (i.e. "no request"), but in
				// order for cmp to work properly, the outer types should be identical.
				// Thus we have to dereference it explicitly:
				expReq := *test.expReq

				// ensure that the received request is what was expected
				if !cmp.Equal(gotReq, expReq) {
					t.Error("received request differs from expectation: diff", cmp.Diff(gotReq, expReq))
				}
			}
		})
	}
}

func TestCreateOrUpdateNetworkContainer(t *testing.T) {
	// create the routes necessary for a test client
	emptyRoutes, _ := buildRoutes(defaultBaseURL, clientPaths)

	// define test cases
	createNCTests := []struct {
		name      string
		client    *RequestCapture
		req       cns.CreateNetworkContainerRequest
		expReq    *cns.CreateNetworkContainerRequest
		shouldErr bool
	}{
		{
			"empty request",
			&RequestCapture{
				Next: &mockdo{},
			},
			cns.CreateNetworkContainerRequest{},
			nil,
			true,
		},
		{
			"valid",
			&RequestCapture{
				Next: &mockdo{},
			},
			cns.CreateNetworkContainerRequest{
				Version:              "12345",
				NetworkContainerType: "blah",
				NetworkContainerid:   "4815162342",
				// to get a proper zero value for this informational field, we have to
				// do this json.RawMessage trick:
				OrchestratorContext: json.RawMessage("null"),
			},
			&cns.CreateNetworkContainerRequest{
				Version:              "12345",
				NetworkContainerType: "blah",
				NetworkContainerid:   "4815162342",
				OrchestratorContext:  json.RawMessage("null"),
			},
			false,
		},
		{
			"unspecified error",
			&RequestCapture{
				Next: &mockdo{
					errToReturn: nil,
					objToReturn: cns.Response{
						ReturnCode: types.MalformedSubnet,
					},
					httpStatusCodeToReturn: http.StatusBadRequest,
				},
			},
			cns.CreateNetworkContainerRequest{
				Version:              "12345",
				NetworkContainerType: "blah",
				NetworkContainerid:   "4815162342",
				// to get a proper zero value for this informational field, we have to
				// do this json.RawMessage trick:
				OrchestratorContext: json.RawMessage("null"),
			},
			&cns.CreateNetworkContainerRequest{
				Version:              "12345",
				NetworkContainerType: "blah",
				NetworkContainerid:   "4815162342",
				OrchestratorContext:  json.RawMessage("null"),
			},
			true,
		},
	}

	for _, test := range createNCTests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// create a new client
			client := &Client{
				client: test.client,
				routes: emptyRoutes,
			}

			// execute
			err := client.CreateNetworkContainer(context.TODO(), test.req)
			if err != nil && !test.shouldErr {
				t.Fatal("unexpected error: err:", err)
			}

			if err == nil && test.shouldErr {
				t.Fatal("expected an error but received none")
			}

			// make sure that if we expected a request, that the correct one was
			// received
			if test.expReq != nil {
				// first make sure a request was actually received
				if test.client.Request == nil {
					t.Fatal("expected to receive a request, but none received")
				}

				// decode the received request for later comparison
				var gotReq cns.CreateNetworkContainerRequest
				err = json.NewDecoder(test.client.Request.Body).Decode(&gotReq)
				if err != nil {
					t.Fatal("error decoding received request: err:", err)
				}

				// we know a non-nil request is present (i.e. we expect a request), so
				// we dereference it so that cmp can properly compare the types
				expReq := *test.expReq

				if !cmp.Equal(gotReq, expReq) {
					t.Error("received request differs from expectation: diff:", cmp.Diff(gotReq, expReq))
				}
			}
		})
	}
}

func TestPublishNC(t *testing.T) {
	// create the routes necessary for a test client
	emptyRoutes, _ := buildRoutes(defaultBaseURL, clientPaths)

	// define the test cases
	publishNCTests := []struct {
		name      string
		client    *RequestCapture
		req       cns.PublishNetworkContainerRequest
		expReq    *cns.PublishNetworkContainerRequest
		shouldErr bool
	}{
		{
			"empty",
			&RequestCapture{
				Next: &mockdo{},
			},
			cns.PublishNetworkContainerRequest{},
			nil,
			true,
		},
		{
			"complete",
			&RequestCapture{
				Next: &mockdo{},
			},
			cns.PublishNetworkContainerRequest{
				NetworkID:                         "foo",
				NetworkContainerID:                "frob",
				JoinNetworkURL:                    "http://example.com",
				CreateNetworkContainerURL:         "http://example.com",
				CreateNetworkContainerRequestBody: []byte{},
			},
			&cns.PublishNetworkContainerRequest{
				NetworkID:                         "foo",
				NetworkContainerID:                "frob",
				JoinNetworkURL:                    "http://example.com",
				CreateNetworkContainerURL:         "http://example.com",
				CreateNetworkContainerRequestBody: []byte{},
			},
			false,
		},
		{
			"unspecified error",
			&RequestCapture{
				Next: &mockdo{
					errToReturn: nil,
					objToReturn: cns.PublishNetworkContainerResponse{
						Response: cns.Response{
							ReturnCode: types.MalformedSubnet,
						},
					},
					httpStatusCodeToReturn: http.StatusBadRequest,
				},
			},
			cns.PublishNetworkContainerRequest{
				NetworkID:                         "foo",
				NetworkContainerID:                "frob",
				JoinNetworkURL:                    "http://example.com",
				CreateNetworkContainerURL:         "http://example.com",
				CreateNetworkContainerRequestBody: []byte{},
			},
			&cns.PublishNetworkContainerRequest{
				NetworkID:                         "foo",
				NetworkContainerID:                "frob",
				JoinNetworkURL:                    "http://example.com",
				CreateNetworkContainerURL:         "http://example.com",
				CreateNetworkContainerRequestBody: []byte{},
			},
			true,
		},
	}

	for _, test := range publishNCTests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// create a client
			client := &Client{
				client: test.client,
				routes: emptyRoutes,
			}

			// invoke the endpoint
			err := client.PublishNetworkContainer(context.TODO(), test.req)
			if err != nil && !test.shouldErr {
				t.Fatal("unexpected error: err:", err)
			}

			if err == nil && test.shouldErr {
				t.Fatal("expected an error but received none")
			}

			// if we expected to receive a request, make sure it's identical to the
			// one we received
			if test.expReq != nil {
				// first ensure that we actually got a request
				if test.client.Request == nil {
					t.Fatal("expected to receive a request, but received none")
				}
			}
		})
	}
}

func TestUnpublishNC(t *testing.T) {
	// create the routes necessary for a test client
	emptyRoutes, _ := buildRoutes(defaultBaseURL, clientPaths)

	// define test cases
	unpublishTests := []struct {
		name      string
		client    *RequestCapture
		req       cns.UnpublishNetworkContainerRequest
		expReq    *cns.UnpublishNetworkContainerRequest
		shouldErr bool
	}{
		{
			"empty",
			&RequestCapture{
				Next: &mockdo{},
			},
			cns.UnpublishNetworkContainerRequest{},
			nil,
			true,
		},
		{
			"complete",
			&RequestCapture{
				Next: &mockdo{},
			},
			cns.UnpublishNetworkContainerRequest{
				NetworkID:                 "foo",
				NetworkContainerID:        "bar",
				JoinNetworkURL:            "http://example.com",
				DeleteNetworkContainerURL: "http://example.com",
			},
			&cns.UnpublishNetworkContainerRequest{
				NetworkID:                 "foo",
				NetworkContainerID:        "bar",
				JoinNetworkURL:            "http://example.com",
				DeleteNetworkContainerURL: "http://example.com",
			},
			false,
		},
		{
			"unexpected error",
			&RequestCapture{
				Next: &mockdo{
					errToReturn: nil,
					objToReturn: cns.UnpublishNetworkContainerResponse{
						Response: cns.Response{
							ReturnCode: types.MalformedSubnet,
						},
					},
					httpStatusCodeToReturn: http.StatusBadRequest,
				},
			},
			cns.UnpublishNetworkContainerRequest{
				NetworkID:                 "foo",
				NetworkContainerID:        "bar",
				JoinNetworkURL:            "http://example.com",
				DeleteNetworkContainerURL: "http://example.com",
			},
			&cns.UnpublishNetworkContainerRequest{
				NetworkID:                 "foo",
				NetworkContainerID:        "bar",
				JoinNetworkURL:            "http://example.com",
				DeleteNetworkContainerURL: "http://example.com",
			},
			true,
		},
	}

	for _, test := range unpublishTests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// create a client
			client := &Client{
				client: test.client,
				routes: emptyRoutes,
			}

			// invoke the endpoint
			err := client.UnpublishNC(context.TODO(), test.req)
			if err != nil && !test.shouldErr {
				t.Fatal("unexpected error: err:", err)
			}

			if err == nil && test.shouldErr {
				t.Fatal("expected an error but received none")
			}

			// ensure the received request matches expectations if a request was
			// expected to be received
			if test.expReq != nil {
				// make sure that a request was sent
				if test.client.Request == nil {
					t.Fatal("expected a request to be sent but none was received")
				}
			}
		})
	}
}

func TestSetOrchestrator(t *testing.T) {
	// define the required routes for the CNS client
	emptyRoutes, _ := buildRoutes(defaultBaseURL, clientPaths)

	// define test cases
	setOrchestratorTests := []struct {
		name      string
		req       cns.SetOrchestratorTypeRequest
		response  *RequestCapture
		expReq    *cns.SetOrchestratorTypeRequest
		shouldErr bool
	}{
		{
			"empty",
			cns.SetOrchestratorTypeRequest{},
			&RequestCapture{
				Next: &mockdo{},
			},
			nil,
			true,
		},
		{
			"missing dnc partition key",
			cns.SetOrchestratorTypeRequest{
				OrchestratorType: "Kubernetes",
				NodeID:           "12345",
			},
			&RequestCapture{
				Next: &mockdo{},
			},
			nil,
			true,
		},
		{
			"missing node id key",
			cns.SetOrchestratorTypeRequest{
				OrchestratorType: "Kubernetes",
				DncPartitionKey:  "foo",
			},
			&RequestCapture{
				Next: &mockdo{},
			},
			nil,
			true,
		},
		{
			"full request",
			cns.SetOrchestratorTypeRequest{
				OrchestratorType: "Kubernetes",
				DncPartitionKey:  "foo",
				NodeID:           "12345",
			},
			&RequestCapture{
				Next: &mockdo{},
			},
			&cns.SetOrchestratorTypeRequest{
				OrchestratorType: "Kubernetes",
				DncPartitionKey:  "foo",
				NodeID:           "12345",
			},
			false,
		},
		{
			"unspecified error",
			cns.SetOrchestratorTypeRequest{
				OrchestratorType: "Kubernetes",
				DncPartitionKey:  "foo",
				NodeID:           "12345",
			},
			&RequestCapture{
				Next: &mockdo{
					errToReturn: nil,
					objToReturn: cns.Response{
						ReturnCode: types.MalformedSubnet,
					},
					httpStatusCodeToReturn: http.StatusBadRequest,
				},
			},
			&cns.SetOrchestratorTypeRequest{
				OrchestratorType: "Kubernetes",
				DncPartitionKey:  "foo",
				NodeID:           "12345",
			},
			true,
		},
	}

	for _, test := range setOrchestratorTests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// set up a client with the mocked routes
			client := Client{
				client: test.response,
				routes: emptyRoutes,
			}

			// execute
			err := client.SetOrchestratorType(context.TODO(), test.req)
			if err != nil && !test.shouldErr {
				t.Fatal("request produced an error where none was expected: err:", err)
			}

			if err == nil && test.shouldErr {
				t.Fatal("expected an error from the request, but none received")
			}

			// check to see if we expected a request to be sent. If so,
			// compare it to the request we actually received
			if test.expReq != nil {
				// first make sure any request at all was received
				if test.response.Request == nil {
					t.Fatal("expected a request to be sent, but none was")
				}

				var gotReq cns.SetOrchestratorTypeRequest
				err := json.NewDecoder(test.response.Request.Body).Decode(&gotReq)
				if err != nil {
					t.Fatal("decoding received request body")
				}

				// because a nil pointer in the expected request means "no
				// request expected", we have to dereference it here to make
				// sure that the type aligns with the gotReq
				expReq := *test.expReq

				if !cmp.Equal(gotReq, expReq) {
					t.Error("received request differs from expectation: diff:", cmp.Diff(gotReq, expReq))
				}
			}
		})
	}
}

func TestDeleteHostNCApipaEndpoint(t *testing.T) {
	emptyRoutes, _ := buildRoutes(defaultBaseURL, clientPaths)
	tests := []struct {
		name               string
		ctx                context.Context
		networkContainerID string
		mockdo             *mockdo
		routes             map[string]url.URL
		wantErr            bool
	}{
		{
			name:               "happy case",
			ctx:                context.TODO(),
			networkContainerID: "testncid",
			mockdo: &mockdo{
				errToReturn:            nil,
				objToReturn:            &cns.DeleteHostNCApipaEndpointResponse{},
				httpStatusCodeToReturn: http.StatusOK,
			},
			routes:  emptyRoutes,
			wantErr: false,
		},
		{
			name:               "bad request",
			ctx:                context.TODO(),
			networkContainerID: "testncid",
			mockdo: &mockdo{
				errToReturn:            errBadRequest,
				objToReturn:            nil,
				httpStatusCodeToReturn: http.StatusBadRequest,
			},
			routes:  emptyRoutes,
			wantErr: true,
		},
		{
			name:               "bad decoding",
			ctx:                context.TODO(),
			networkContainerID: "testncid",
			mockdo: &mockdo{
				errToReturn:            nil,
				objToReturn:            []cns.DeleteHostNCApipaEndpointResponse{},
				httpStatusCodeToReturn: http.StatusOK,
			},
			routes:  emptyRoutes,
			wantErr: true,
		},
		{
			name:               "http status not ok",
			ctx:                context.TODO(),
			networkContainerID: "testncid",
			mockdo: &mockdo{
				errToReturn:            nil,
				objToReturn:            nil,
				httpStatusCodeToReturn: http.StatusInternalServerError,
			},
			routes:  emptyRoutes,
			wantErr: true,
		},
		{
			name:               "cns return code not zero",
			ctx:                context.TODO(),
			networkContainerID: "testncid",
			mockdo: &mockdo{
				errToReturn: nil,
				objToReturn: &cns.DeleteHostNCApipaEndpointResponse{
					Response: cns.Response{
						ReturnCode: types.UnsupportedNetworkType,
					},
				},
				httpStatusCodeToReturn: http.StatusOK,
			},
			routes:  emptyRoutes,
			wantErr: true,
		},
		{
			name:               "nil context",
			ctx:                nil,
			networkContainerID: "testncid",
			mockdo:             &mockdo{},
			routes:             emptyRoutes,
			wantErr:            true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{
				client: tt.mockdo,
				routes: tt.routes,
			}
			err := client.DeleteHostNCApipaEndpoint(tt.ctx, tt.networkContainerID)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRequestIPAddress(t *testing.T) {
	emptyRoutes, _ := buildRoutes(defaultBaseURL, clientPaths)
	tests := []struct {
		name     string
		ctx      context.Context
		ipconfig cns.IPConfigRequest
		mockdo   *mockdo
		routes   map[string]url.URL
		want     *cns.IPConfigResponse
		wantErr  bool
	}{
		{
			name: "happy case",
			ctx:  context.TODO(),
			ipconfig: cns.IPConfigRequest{
				DesiredIPAddress: "testipaddress",
				PodInterfaceID:   "testpodinterfaceid",
				InfraContainerID: "testcontainerid",
			},
			mockdo: &mockdo{
				errToReturn:            nil,
				objToReturn:            &cns.IPConfigResponse{},
				httpStatusCodeToReturn: http.StatusOK,
			},
			routes:  emptyRoutes,
			want:    &cns.IPConfigResponse{},
			wantErr: false,
		},
		{
			name: "bad request",
			ctx:  context.TODO(),
			ipconfig: cns.IPConfigRequest{
				DesiredIPAddress: "testipaddress",
				PodInterfaceID:   "testpodinterfaceid",
				InfraContainerID: "testcontainerid",
			},
			mockdo: &mockdo{
				errToReturn:            errBadRequest,
				objToReturn:            nil,
				httpStatusCodeToReturn: http.StatusBadRequest,
			},
			routes:  emptyRoutes,
			want:    nil,
			wantErr: true,
		},
		{
			name: "bad decoding",
			ctx:  context.TODO(),
			ipconfig: cns.IPConfigRequest{
				DesiredIPAddress: "testipaddress",
				PodInterfaceID:   "testpodinterfaceid",
				InfraContainerID: "testcontainerid",
			},
			mockdo: &mockdo{
				errToReturn:            nil,
				objToReturn:            []cns.IPConfigResponse{},
				httpStatusCodeToReturn: http.StatusOK,
			},
			routes:  emptyRoutes,
			want:    nil,
			wantErr: true,
		},
		{
			name:     "http status not ok",
			ctx:      context.TODO(),
			ipconfig: cns.IPConfigRequest{},
			mockdo: &mockdo{
				errToReturn:            nil,
				objToReturn:            nil,
				httpStatusCodeToReturn: http.StatusInternalServerError,
			},
			routes:  emptyRoutes,
			want:    nil,
			wantErr: true,
		},
		{
			name: "cns return code not zero",
			ctx:  context.TODO(),
			ipconfig: cns.IPConfigRequest{
				DesiredIPAddress: "testipaddress",
				PodInterfaceID:   "testpodinterfaceid",
				InfraContainerID: "testcontainerid",
			},
			mockdo: &mockdo{
				errToReturn: nil,
				objToReturn: &cns.IPConfigResponse{
					Response: cns.Response{
						ReturnCode: types.UnsupportedNetworkType,
					},
				},
				httpStatusCodeToReturn: http.StatusOK,
			},
			routes:  emptyRoutes,
			want:    nil,
			wantErr: true,
		},
		{
			name: "nil context",
			ctx:  nil,
			ipconfig: cns.IPConfigRequest{
				DesiredIPAddress: "testipaddress",
				PodInterfaceID:   "testpodinterfaceid",
				InfraContainerID: "testcontainerid",
			},
			mockdo:  &mockdo{},
			routes:  emptyRoutes,
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{
				client: tt.mockdo,
				routes: tt.routes,
			}
			got, err := client.RequestIPAddress(tt.ctx, tt.ipconfig)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestReleaseIPAddress(t *testing.T) {
	emptyRoutes, _ := buildRoutes(defaultBaseURL, clientPaths)
	tests := []struct {
		name     string
		ctx      context.Context
		ipconfig cns.IPConfigRequest
		mockdo   *mockdo
		routes   map[string]url.URL
		wantErr  bool
	}{
		{
			name: "happy case",
			ctx:  context.TODO(),
			ipconfig: cns.IPConfigRequest{
				DesiredIPAddress: "testipaddress",
				PodInterfaceID:   "testpodinterfaceid",
				InfraContainerID: "testcontainerid",
			},
			mockdo: &mockdo{
				errToReturn:            nil,
				objToReturn:            &cns.Response{},
				httpStatusCodeToReturn: http.StatusOK,
			},
			routes:  emptyRoutes,
			wantErr: false,
		},
		{
			name: "bad request",
			ctx:  context.TODO(),
			ipconfig: cns.IPConfigRequest{
				DesiredIPAddress: "testipaddress",
				PodInterfaceID:   "testpodinterfaceid",
				InfraContainerID: "testcontainerid",
			},
			mockdo: &mockdo{
				errToReturn:            errBadRequest,
				objToReturn:            nil,
				httpStatusCodeToReturn: http.StatusBadRequest,
			},
			routes:  emptyRoutes,
			wantErr: true,
		},
		{
			name: "bad decoding",
			ctx:  context.TODO(),
			ipconfig: cns.IPConfigRequest{
				DesiredIPAddress: "testipaddress",
				PodInterfaceID:   "testpodinterfaceid",
				InfraContainerID: "testcontainerid",
			},
			mockdo: &mockdo{
				errToReturn:            nil,
				objToReturn:            []cns.Response{},
				httpStatusCodeToReturn: http.StatusOK,
			},
			routes:  emptyRoutes,
			wantErr: true,
		},
		{
			name:     "http status not ok",
			ctx:      context.TODO(),
			ipconfig: cns.IPConfigRequest{},
			mockdo: &mockdo{
				errToReturn:            nil,
				objToReturn:            nil,
				httpStatusCodeToReturn: http.StatusInternalServerError,
			},
			routes:  emptyRoutes,
			wantErr: true,
		},
		{
			name: "cns return code not zero",
			ctx:  context.TODO(),
			ipconfig: cns.IPConfigRequest{
				DesiredIPAddress: "testipaddress",
				PodInterfaceID:   "testpodinterfaceid",
				InfraContainerID: "testcontainerid",
			},
			mockdo: &mockdo{
				errToReturn: nil,
				objToReturn: &cns.Response{
					ReturnCode: types.UnsupportedNetworkType,
				},
				httpStatusCodeToReturn: http.StatusOK,
			},
			routes:  emptyRoutes,
			wantErr: true,
		},
		{
			name: "nil context",
			ctx:  nil,
			ipconfig: cns.IPConfigRequest{
				DesiredIPAddress: "testipaddress",
				PodInterfaceID:   "testpodinterfaceid",
				InfraContainerID: "testcontainerid",
			},
			mockdo:  &mockdo{},
			routes:  emptyRoutes,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{
				client: tt.mockdo,
				routes: tt.routes,
			}
			err := client.ReleaseIPAddress(tt.ctx, tt.ipconfig)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetIPAddressesMatchingStates(t *testing.T) {
	emptyRoutes, _ := buildRoutes(defaultBaseURL, clientPaths)
	tests := []struct {
		name        string
		ctx         context.Context
		stateFilter []types.IPState
		mockdo      *mockdo
		routes      map[string]url.URL
		want        []cns.IPConfigurationStatus
		wantErr     bool
	}{
		{
			name:        "happy case",
			ctx:         context.TODO(),
			stateFilter: []types.IPState{types.Available},
			mockdo: &mockdo{
				errToReturn: nil,
				objToReturn: &cns.GetIPAddressStatusResponse{
					IPConfigurationStatus: []cns.IPConfigurationStatus{},
				},
				httpStatusCodeToReturn: http.StatusOK,
			},
			routes:  emptyRoutes,
			want:    []cns.IPConfigurationStatus{},
			wantErr: false,
		},
		{
			name:        "length of zero",
			ctx:         context.TODO(),
			stateFilter: []types.IPState{},
			mockdo:      &mockdo{},
			routes:      emptyRoutes,
			want:        nil,
			wantErr:     false,
		},
		{
			name:        "bad request",
			ctx:         context.TODO(),
			stateFilter: []types.IPState{"garbage"},
			mockdo: &mockdo{
				errToReturn:            errBadRequest,
				objToReturn:            nil,
				httpStatusCodeToReturn: http.StatusBadRequest,
			},
			routes:  emptyRoutes,
			want:    nil,
			wantErr: true,
		},
		{
			name:        "bad decoding",
			ctx:         context.TODO(),
			stateFilter: []types.IPState{types.Available},
			mockdo: &mockdo{
				errToReturn:            nil,
				objToReturn:            []cns.GetIPAddressStatusResponse{},
				httpStatusCodeToReturn: http.StatusOK,
			},
			routes:  emptyRoutes,
			want:    nil,
			wantErr: true,
		},
		{
			name:        "http status not ok",
			ctx:         context.TODO(),
			stateFilter: []types.IPState{types.Available},
			mockdo: &mockdo{
				errToReturn:            nil,
				objToReturn:            nil,
				httpStatusCodeToReturn: http.StatusInternalServerError,
			},
			routes:  emptyRoutes,
			want:    nil,
			wantErr: true,
		},
		{
			name:        "cns return code not zero",
			ctx:         context.TODO(),
			stateFilter: []types.IPState{types.Available},
			mockdo: &mockdo{
				errToReturn: nil,
				objToReturn: &cns.GetIPAddressStatusResponse{
					Response: cns.Response{
						ReturnCode: types.UnsupportedNetworkType,
					},
				},
				httpStatusCodeToReturn: http.StatusOK,
			},
			routes:  emptyRoutes,
			want:    nil,
			wantErr: true,
		},
		{
			name:        "nil context",
			ctx:         nil,
			stateFilter: []types.IPState{types.Available},
			mockdo:      &mockdo{},
			routes:      emptyRoutes,
			wantErr:     true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{
				client: tt.mockdo,
				routes: tt.routes,
			}
			got, err := client.GetIPAddressesMatchingStates(tt.ctx, tt.stateFilter...)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetPodOrchestratorContext(t *testing.T) {
	emptyRoutes, _ := buildRoutes(defaultBaseURL, clientPaths)
	tests := []struct {
		name    string
		ctx     context.Context
		mockdo  *mockdo
		routes  map[string]url.URL
		want    map[string]string
		wantErr bool
	}{
		{
			name: "happy case",
			ctx:  context.TODO(),
			mockdo: &mockdo{
				errToReturn: nil,
				objToReturn: &cns.GetPodContextResponse{
					PodContext: map[string]string{},
				},
				httpStatusCodeToReturn: http.StatusOK,
			},
			routes:  emptyRoutes,
			want:    map[string]string{},
			wantErr: false,
		},
		{
			name: "bad request",
			ctx:  context.TODO(),
			mockdo: &mockdo{
				errToReturn:            errBadRequest,
				objToReturn:            nil,
				httpStatusCodeToReturn: http.StatusBadRequest,
			},
			routes:  emptyRoutes,
			want:    nil,
			wantErr: true,
		},
		{
			name: "bad decoding",
			ctx:  context.TODO(),
			mockdo: &mockdo{
				errToReturn:            nil,
				objToReturn:            []cns.GetPodContextResponse{},
				httpStatusCodeToReturn: http.StatusOK,
			},
			routes:  emptyRoutes,
			want:    nil,
			wantErr: true,
		},
		{
			name: "http status not ok",
			ctx:  context.TODO(),
			mockdo: &mockdo{
				errToReturn:            nil,
				objToReturn:            nil,
				httpStatusCodeToReturn: http.StatusInternalServerError,
			},
			routes:  emptyRoutes,
			want:    nil,
			wantErr: true,
		},
		{
			name: "cns return code not zero",
			ctx:  context.TODO(),
			mockdo: &mockdo{
				errToReturn: nil,
				objToReturn: &cns.GetPodContextResponse{
					Response: cns.Response{
						ReturnCode: types.UnsupportedNetworkType,
					},
				},
				httpStatusCodeToReturn: http.StatusOK,
			},
			routes:  emptyRoutes,
			want:    nil,
			wantErr: true,
		},
		{
			name:    "nil context",
			ctx:     nil,
			mockdo:  &mockdo{},
			routes:  emptyRoutes,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{
				client: tt.mockdo,
				routes: tt.routes,
			}
			got, err := client.GetPodOrchestratorContext(tt.ctx)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetHTTPServiceData(t *testing.T) {
	emptyRoutes, _ := buildRoutes(defaultBaseURL, clientPaths)
	tests := []struct {
		name    string
		ctx     context.Context
		mockdo  *mockdo
		routes  map[string]url.URL
		want    *restserver.GetHTTPServiceDataResponse
		wantErr bool
	}{
		{
			name: "happy case",
			ctx:  context.TODO(),
			mockdo: &mockdo{
				errToReturn:            nil,
				objToReturn:            &restserver.GetHTTPServiceDataResponse{},
				httpStatusCodeToReturn: http.StatusOK,
			},
			routes:  emptyRoutes,
			want:    &restserver.GetHTTPServiceDataResponse{},
			wantErr: false,
		},
		{
			name: "bad request",
			ctx:  context.TODO(),
			mockdo: &mockdo{
				errToReturn:            errBadRequest,
				objToReturn:            nil,
				httpStatusCodeToReturn: http.StatusBadRequest,
			},
			routes:  emptyRoutes,
			want:    nil,
			wantErr: true,
		},
		{
			name: "bad decoding",
			ctx:  context.TODO(),
			mockdo: &mockdo{
				errToReturn:            nil,
				objToReturn:            []restserver.GetHTTPServiceDataResponse{},
				httpStatusCodeToReturn: http.StatusOK,
			},
			routes:  emptyRoutes,
			want:    nil,
			wantErr: true,
		},
		{
			name: "http status not ok",
			ctx:  context.TODO(),
			mockdo: &mockdo{
				errToReturn:            nil,
				objToReturn:            nil,
				httpStatusCodeToReturn: http.StatusInternalServerError,
			},
			routes:  emptyRoutes,
			want:    nil,
			wantErr: true,
		},
		{
			name: "cns return code not zero",
			ctx:  context.TODO(),
			mockdo: &mockdo{
				errToReturn: nil,
				objToReturn: &restserver.GetHTTPServiceDataResponse{
					Response: restserver.Response{
						ReturnCode: types.UnsupportedNetworkType,
					},
				},
				httpStatusCodeToReturn: http.StatusOK,
			},
			routes:  emptyRoutes,
			want:    nil,
			wantErr: true,
		},
		{
			name:    "nil context",
			ctx:     nil,
			mockdo:  &mockdo{},
			routes:  emptyRoutes,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{
				client: tt.mockdo,
				routes: tt.routes,
			}
			got, err := client.GetHTTPServiceData(tt.ctx)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
