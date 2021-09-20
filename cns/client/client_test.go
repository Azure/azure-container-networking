package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
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
	releasePercent      = 50
	requestPercent      = 100
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
	body := ioutil.NopCloser(bytes.NewReader(byteArray))

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

	req := cns.CreateNetworkContainerRequest{
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

	svc.IPAMPoolMonitor.Update(
		fakes.NewFakeScalar(releasePercent, requestPercent, batchSize),
		fakes.NewFakeNodeNetworkConfigSpec(initPoolSize))
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

	tmpFileState, err := ioutil.TempFile(os.TempDir(), "cns-*.json")
	tmpLogDir, err := ioutil.TempDir("", "cns-")
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

	httpRestService, err := restserver.NewHTTPRestService(&config, fakes.NewFakeImdsClient(), fakes.NewFakeNMAgentClient())
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
				ReleaseThresholdPercent: 50,
				RequestThresholdPercent: 40,
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
	svc.IPAMPoolMonitor = &fakes.IPAMPoolMonitorFake{FakeMinimumIps: 10, FakeMaximumIps: 20, FakeIpsNotInUseCount: 13, FakecachedNNC: fakeNNC}

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
	cnsClient, _ := New("", 2*time.Second)

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

	// checking for allocated IP address and pod context printing before ReleaseIPAddress is called
	ipaddresses, err := cnsClient.GetIPAddressesMatchingStates(context.TODO(), cns.Allocated)
	assert.NoError(t, err, "Get allocated IP addresses failed")

	assert.Len(t, ipaddresses, 1, "Number of available IP addresses expected to be 1")
	assert.Equal(t, desiredIpAddress, ipaddresses[0].IPAddress, "Available IP address does not match expected, address state")
	assert.Equal(t, cns.Allocated, ipaddresses[0].State, "Available IP address does not match expected, address state")

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
	cnsClient, _ := New("", 2*time.Second)

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
		assert.Equal(t, cns.Allocated, v.State, "Not the expected set values for testing IPConfigurationStatus, %+v", podConfig)
		assert.Equal(t, "testNcId1", v.NCID, "Not the expected set values for testing IPConfigurationStatus, %+v", podConfig)
	}
	assert.GreaterOrEqual(t, len(inmemory.HTTPRestServiceData.PodIPConfigState), 1, "PodIpConfigState with at least 1 entry expected")

	testIpamPoolMonitor := inmemory.HTTPRestServiceData.IPAMPoolMonitor
	assert.EqualValues(t, 10, testIpamPoolMonitor.MinimumFreeIps, "IPAMPoolMonitor state is not reflecting the initial set values")
	assert.EqualValues(t, 20, testIpamPoolMonitor.MaximumFreeIps, "IPAMPoolMonitor state is not reflecting the initial set values")
	assert.Equal(t, 13, testIpamPoolMonitor.UpdatingIpsNotInUseCount, "IPAMPoolMonitor state is not reflecting the initial set values")

	// check for cached NNC Spec struct values
	assert.EqualValues(t, 16, testIpamPoolMonitor.CachedNNC.Spec.RequestedIPCount, "IPAMPoolMonitor cached NNC Spec is not reflecting the initial set values")
	assert.Len(t, testIpamPoolMonitor.CachedNNC.Spec.IPsNotInUse, 1, "IPAMPoolMonitor cached NNC Spec is not reflecting the initial set values")

	// check for cached NNC Status struct values
	assert.EqualValues(t, 10, testIpamPoolMonitor.CachedNNC.Status.Scaler.BatchSize, "IPAMPoolMonitor cached NNC Status is not reflecting the initial set values")
	assert.EqualValues(t, 50, testIpamPoolMonitor.CachedNNC.Status.Scaler.ReleaseThresholdPercent, "IPAMPoolMonitor cached NNC Status is not reflecting the initial set values")
	assert.EqualValues(t, 40, testIpamPoolMonitor.CachedNNC.Status.Scaler.RequestThresholdPercent, "IPAMPoolMonitor cached NNC Status is not reflecting the initial set values")
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
		stateFilter []cns.IPConfigState
		mockdo      *mockdo
		routes      map[string]url.URL
		want        []cns.IPConfigurationStatus
		wantErr     bool
	}{
		{
			name:        "happy case",
			ctx:         context.TODO(),
			stateFilter: []cns.IPConfigState{cns.Available},
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
			stateFilter: []cns.IPConfigState{},
			mockdo:      &mockdo{},
			routes:      emptyRoutes,
			want:        nil,
			wantErr:     false,
		},
		{
			name:        "bad request",
			ctx:         context.TODO(),
			stateFilter: []cns.IPConfigState{"garbage"},
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
			stateFilter: []cns.IPConfigState{cns.Available},
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
			stateFilter: []cns.IPConfigState{cns.Available},
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
			stateFilter: []cns.IPConfigState{cns.Available},
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
			stateFilter: []cns.IPConfigState{cns.Available},
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