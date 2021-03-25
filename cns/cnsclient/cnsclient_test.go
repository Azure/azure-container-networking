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

	returnCode := svc.CreateOrUpdateNetworkContainerInternal(req, fakes.NewFakeScalar(releasePercent, requestPercent, batchSize), fakes.NewFakeNodeNetworkConfigSpec(initPoolSize))
	if returnCode != 0 {
		t.Fatalf("Failed to createNetworkContainerRequest, req: %+v, err: %d", req, returnCode)
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
	svc.IPAMPoolMonitor = fakes.NewIPAMPoolMonitorFake()

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
	cnsClient, _ := InitCnsClient("")

	addTestStateToRestServer(t, secondaryIps)

	podInfo := cns.KubernetesPodInfo{PodName: podName, PodNamespace: podNamespace}
	orchestratorContext, err := json.Marshal(podInfo)
	if err != nil {
		t.Fatal(err)
	}

	// no IP reservation found with that context, expect no failure.
	err = cnsClient.ReleaseIPAddress(orchestratorContext)
	if err != nil {
		t.Fatalf("Release ip idempotent call failed: %+v", err)
	}

	// request IP address
	resp, err := cnsClient.RequestIPAddress(orchestratorContext)
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

	// release requested IP address, expect success
	err = cnsClient.ReleaseIPAddress(orchestratorContext)
	if err != nil {
		t.Fatalf("Expected to not fail when releasing IP reservation found with context: %+v", err)
	}

	ipaddresses, err := cnsClient.GetIPAddressesMatchingStates(cns.Available)
	if err != nil {
		t.Fatalf("Get allocated IP addresses failed %+v", err)
	}

	if len(ipaddresses) != 1 {
		t.Fatalf("Number of available IP addresses expected to be 1, actual %+v", ipaddresses)
	}

	if ipaddresses[0].IPAddress != desiredIpAddress && ipaddresses[0].State != cns.Available {
		t.Fatalf("Available IP address does not match expected, address state: %+v", ipaddresses)
	}
}
