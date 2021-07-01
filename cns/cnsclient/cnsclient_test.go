package cnsclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/common"
	"github.com/Azure/azure-container-networking/cns/fakes"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/restserver"
	"github.com/Azure/azure-container-networking/log"
	"github.com/google/uuid"
)

var (
	svc *restserver.HTTPRestService
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

var (
	dnsservers = []string{"8.8.8.8", "8.8.4.4"}
)

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

	returnCode = svc.UpdateIPAMPoolMonitorInternal(fakes.NewFakeScalar(releasePercent, requestPercent, batchSize), fakes.NewFakeNodeNetworkConfigSpec(initPoolSize))
	if returnCode != 0 {
		t.Fatalf("Failed to UpdateIPAMPoolMonitorInternal, err: %d", returnCode)
	}
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
			OrchestratorType: cns.KubernetesCRD}
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
	fakeNNC := nnc.NodeNetworkConfig{
		TypeMeta:   metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{},
		Spec: nnc.NodeNetworkConfigSpec{
			RequestedIPCount: 16,
			IPsNotInUse:      []string{"abc"},
		},
		Status: nnc.NodeNetworkConfigStatus{
			Scaler: nnc.Scaler{
				BatchSize:               10,
				ReleaseThresholdPercent: 50,
				RequestThresholdPercent: 40,
			},
			NetworkContainers: []nnc.NetworkContainer{
				{
					ID:         "nc1",
					PrimaryIP:  "10.0.0.11",
					SubnetName: "sub1",
					IPAssignments: []nnc.IPAssignment{
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
	url := defaultCnsURL + cns.SetOrchestratorType

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
	cnsClient, _ := InitCnsClient("", 2*time.Second)

	addTestStateToRestServer(t, secondaryIps)

	podInfo := cns.KubernetesPodInfo{PodName: podName, PodNamespace: podNamespace}
	orchestratorContext, err := json.Marshal(podInfo)
	if err != nil {
		t.Fatal(err)
	}

	// no IP reservation found with that context, expect no failure.
	err = cnsClient.ReleaseIPAddress(&cns.IPConfigRequest{OrchestratorContext: orchestratorContext})
	if err != nil {
		t.Fatalf("Release ip idempotent call failed: %+v", err)
	}

	// request IP address
	resp, err := cnsClient.RequestIPAddress(&cns.IPConfigRequest{OrchestratorContext: orchestratorContext})
	if err != nil {
		t.Fatalf("get IP from CNS failed with %+v", err)
	}

	podIPInfo := resp.PodIpInfo
	if reflect.DeepEqual(podIPInfo.NetworkContainerPrimaryIPConfig.IPSubnet.IPAddress, primaryIp) != true {
		t.Fatalf("PrimarIP is not added as expected ipConfig %+v, expected primaryIP: %+v", podIPInfo.NetworkContainerPrimaryIPConfig, primaryIp)
	}

	if podIPInfo.NetworkContainerPrimaryIPConfig.IPSubnet.PrefixLength != subnetPrfixLength {
		t.Fatalf("Primary IP Prefix length is not added as expected ipConfig %+v, expected: %+v", podIPInfo.NetworkContainerPrimaryIPConfig, subnetPrfixLength)
	}

	// validate DnsServer and Gateway Ip as the same configured for Primary IP
	if reflect.DeepEqual(podIPInfo.NetworkContainerPrimaryIPConfig.DNSServers, dnsservers) != true {
		t.Fatalf("DnsServer is not added as expected ipConfig %+v, expected dnsServers: %+v", podIPInfo.NetworkContainerPrimaryIPConfig, dnsservers)
	}

	if reflect.DeepEqual(podIPInfo.NetworkContainerPrimaryIPConfig.GatewayIPAddress, gatewayIp) != true {
		t.Fatalf("Gateway is not added as expected ipConfig %+v, expected GatewayIp: %+v", podIPInfo.NetworkContainerPrimaryIPConfig, gatewayIp)
	}

	resultIPnet, err := getIPNetFromResponse(resp)

	if reflect.DeepEqual(desired, resultIPnet) != true {
		t.Fatalf("Desired result not matching actual result, expected: %+v, actual: %+v", desired, resultIPnet)
	}
	//checking for allocated IP address and pod context printing before ReleaseIPAddress is called
	ipaddresses, err := cnsClient.GetIPAddressesMatchingStates(cns.Allocated)
	if err != nil {
		t.Fatalf("Get allocated IP addresses failed %+v", err)
	}

	if len(ipaddresses) != 1 {
		t.Fatalf("Number of available IP addresses expected to be 1, actual %+v", ipaddresses)
	}

	if ipaddresses[0].IPAddress != desiredIpAddress && ipaddresses[0].State != cns.Allocated {
		t.Fatalf("Available IP address does not match expected, address state: %+v", ipaddresses)
	}

	t.Log(ipaddresses)

	// release requested IP address, expect success
	err = cnsClient.ReleaseIPAddress(&cns.IPConfigRequest{DesiredIPAddress: ipaddresses[0].IPAddress, OrchestratorContext: orchestratorContext})
	if err != nil {
		t.Fatalf("Expected to not fail when releasing IP reservation found with context: %+v", err)
	}
}

func TestCNSClientPodContextApi(t *testing.T) {
	podName := "testpodname"
	podNamespace := "testpodnamespace"
	desiredIpAddress := "10.0.0.5"

	secondaryIps := []string{desiredIpAddress}
	cnsClient, _ := InitCnsClient("", 2*time.Second)

	addTestStateToRestServer(t, secondaryIps)

	podInfo := cns.NewPodInfo("", "", podName, podNamespace)
	orchestratorContext, err := json.Marshal(podInfo)
	if err != nil {
		t.Fatal(err)
	}

	// request IP address
	_, err = cnsClient.RequestIPAddress(&cns.IPConfigRequest{OrchestratorContext: orchestratorContext})
	if err != nil {
		t.Fatalf("get IP from CNS failed with %+v", err)
	}

	//test for pod ip by orch context map
	podcontext, err := cnsClient.GetPodOrchestratorContext()
	if err != nil {
		t.Errorf("Get pod ip by orchestrator context failed:  %+v", err)
	}
	if len(podcontext) < 1 {
		t.Errorf("Expected atleast 1 entry in map for podcontext:  %+v", podcontext)
	}

	t.Log(podcontext)

	// release requested IP address, expect success
	err = cnsClient.ReleaseIPAddress(&cns.IPConfigRequest{OrchestratorContext: orchestratorContext})
	if err != nil {
		t.Fatalf("Expected to not fail when releasing IP reservation found with context: %+v", err)
	}
}

func TestCNSClientDebugAPI(t *testing.T) {
	podName := "testpodname"
	podNamespace := "testpodnamespace"
	desiredIpAddress := "10.0.0.5"

	secondaryIps := []string{desiredIpAddress}
	cnsClient, _ := InitCnsClient("", 2*time.Second)

	addTestStateToRestServer(t, secondaryIps)

	podInfo := cns.NewPodInfo("", "", podName, podNamespace)
	orchestratorContext, err := json.Marshal(podInfo)
	if err != nil {
		t.Fatal(err)
	}

	// request IP address
	_, err1 := cnsClient.RequestIPAddress(&cns.IPConfigRequest{OrchestratorContext: orchestratorContext})
	if err1 != nil {
		t.Fatalf("get IP from CNS failed with %+v", err1)
	}

	//test for debug api/cmd to get inmemory data from HTTPRestService
	inmemory, err := cnsClient.GetHTTPServiceData()
	if err != nil {
		t.Errorf("Get in-memory http REST Struct failed %+v", err)
	}

	if len(inmemory.HttpRestServiceData.PodIPIDByPodInterfaceKey) < 1 {
		t.Errorf("OrchestratorContext map is expected but not returned")
	}

	//testing Pod IP Configuration Status values set for test
	podConfig := inmemory.HttpRestServiceData.PodIPConfigState
	for _, v := range podConfig {
		if v.IPAddress != "10.0.0.5" || v.State != "Allocated" || v.NCID != "testNcId1" {
			t.Errorf("Not the expected set values for testing IPConfigurationStatus, %+v", podConfig)
		}
	}
	if len(inmemory.HttpRestServiceData.PodIPConfigState) < 1 {
		t.Errorf("PodIpConfigState with atleast 1 entry expected but not returned.")
	}

	testIpamPoolMonitor := inmemory.HttpRestServiceData.IPAMPoolMonitor
	if testIpamPoolMonitor.MinimumFreeIps != 10 || testIpamPoolMonitor.MaximumFreeIps != 20 || testIpamPoolMonitor.UpdatingIpsNotInUseCount != 13 {
		t.Errorf("IPAMPoolMonitor state is not reflecting the initial set values, %+v", testIpamPoolMonitor)
	}

	//check for cached NNC Spec struct values
	if testIpamPoolMonitor.CachedNNC.Spec.RequestedIPCount != 16 || len(testIpamPoolMonitor.CachedNNC.Spec.IPsNotInUse) != 1 {
		t.Errorf("IPAMPoolMonitor cached NNC Spec is not reflecting the initial set values, %+v", testIpamPoolMonitor.CachedNNC.Spec)
	}

	//check for cached NNC Status struct values
	if testIpamPoolMonitor.CachedNNC.Status.Scaler.BatchSize != 10 || testIpamPoolMonitor.CachedNNC.Status.Scaler.ReleaseThresholdPercent != 50 || testIpamPoolMonitor.CachedNNC.Status.Scaler.RequestThresholdPercent != 40 {
		t.Errorf("IPAMPoolMonitor cached NNC Status is not reflecting the initial set values, %+v", testIpamPoolMonitor.CachedNNC.Status.Scaler)
	}

	if len(testIpamPoolMonitor.CachedNNC.Status.NetworkContainers) != 1 {
		t.Errorf("Expected only one Network Container in the list, %+v", testIpamPoolMonitor.CachedNNC.Status.NetworkContainers)
	}

	t.Logf("In-memory Data: ")
	t.Logf("PodIPIDByOrchestratorContext: %+v", inmemory.HttpRestServiceData.PodIPIDByPodInterfaceKey)
	t.Logf("PodIPConfigState: %+v", inmemory.HttpRestServiceData.PodIPConfigState)
	t.Logf("IPAMPoolMonitor: %+v", inmemory.HttpRestServiceData.IPAMPoolMonitor)

}
