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
	dockerContainerType = cns.Docker
)

func addTestStateToRestServer(t *testing.T, secondaryIps []string) {
	var ipConfig cns.IPConfiguration
	ipConfig.DNSServers = []string{"8.8.8.8", "8.8.4.4"}
	ipConfig.GatewayIPAddress = gatewayIp
	var ipSubnet cns.IPSubnet
	ipSubnet.IPAddress = primaryIp
	ipSubnet.PrefixLength = 32
	ipConfig.IPSubnet = ipSubnet
	secondaryIPConfigs := make(map[string]cns.SecondaryIPConfig)

	for _, secIpAddress := range secondaryIps {
		secIpConfig := cns.SecondaryIPConfig{
			IPSubnet: cns.IPSubnet{
				IPAddress:    secIpAddress,
				PrefixLength: 32,
			},
		}
		ipId := uuid.New()
		secondaryIPConfigs[ipId.String()] = secIpConfig
	}

	req := cns.CreateNetworkContainerRequest{
		NetworkContainerType: dockerContainerType,
		NetworkContainerid:   "testNcId1",
		IPConfiguration:      ipConfig,
		SecondaryIPConfigs:   secondaryIPConfigs,
	}

	returnCode := svc.CreateOrUpdateNetworkContainerInternal(req)
	if returnCode != 0 {
		t.Fatalf("Failed to createNetworkContainerRequest, req: %+v, err: %d", req, returnCode)
	}
}

func getIPConfigFromGetNetworkContainerResponse(resp *cns.GetIPConfigResponse) (net.IPNet, error) {
	var (
		resultIPnet net.IPNet
		err         error
	)

	// set result ipconfig from CNS Response Body
	prefix := strconv.Itoa(int(resp.IPConfiguration.IPSubnet.PrefixLength))
	ip, ipnet, err := net.ParseCIDR(resp.IPConfiguration.IPSubnet.IPAddress + "/" + prefix)
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

	if err != nil {
		panic(err)
	}

	logger.InitLogger("azure-cns.log", 0, 0, tmpLogDir)
	config := common.ServiceConfig{}

	httpRestService, err := restserver.NewHTTPRestService(&config)
	svc = httpRestService.(*restserver.HTTPRestService)
	svc.Name = "cns-test-server"
	if err != nil {
		logger.Errorf("Failed to create CNS object, err:%v.\n", err)
		return
	}

	if httpRestService != nil {
		err = httpRestService.Start(&config)
		if err != nil {
			logger.Errorf("Failed to start CNS, err:%v.\n", err)
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
	_, ipnet, _ := net.ParseCIDR("10.0.0.5/32")
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

	resultIPnet, err := getIPConfigFromGetNetworkContainerResponse(resp)

	if reflect.DeepEqual(desired, resultIPnet) != true {
		t.Fatalf("Desired result not matching actual result, expected: %+v, actual: %+v", desired, resultIPnet)
	}

	// release requested IP address, expect success
	err = cnsClient.ReleaseIPAddress(orchestratorContext)
	if err != nil {
		t.Fatalf("Expected to not fail when releasing IP reservation found with context: %+v", err)
	}
}
