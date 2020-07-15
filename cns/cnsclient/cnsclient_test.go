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
)

var (
	testNCID = "06867cf3-332d-409d-8819-ed70d2c116b0"

	testIP1      = "10.0.0.1"
	testPod1GUID = "898fb8f1-f93e-4c96-9c31-6b89098949a3"
	testPod1Info = cns.KubernetesPodInfo{
		PodName:      "testpod1",
		PodNamespace: "testpod1namespace",
	}
)

func addTestStateToRestServer(svc *restserver.HTTPRestService) {
	// set state as already allocated
	state1, _ := restserver.NewPodStateWithOrchestratorContext(testIP1, 24, testPod1GUID, testNCID, cns.Available, testPod1Info)
	ipconfigs := map[string]cns.ContainerIPConfigState{
		state1.ID: state1,
	}
	nc := cns.CreateNetworkContainerRequest{
		SecondaryIPConfigs: ipconfigs,
	}

	svc.CreateOrUpdateNetworkContainerWithSecondaryIPConfigs(nc)
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
			OrchestratorType: cns.Kubernetes}
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
	svc := httpRestService.(*restserver.HTTPRestService)
	svc.Name = "cns-test-server"
	if err != nil {
		logger.Errorf("Failed to create CNS object, err:%v.\n", err)
		return
	}

	addTestStateToRestServer(svc)

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

	m.Run()
}

func TestCNSClientRequestAndRelease(t *testing.T) {
	podName := "testpodname"
	podNamespace := "testpodnamespace"
	ip := net.ParseIP("10.0.0.1")
	_, ipnet, _ := net.ParseCIDR("10.0.0.1/24")
	desired := net.IPNet{
		IP:   ip,
		Mask: ipnet.Mask,
	}

	cnsClient, _ := InitCnsClient("")

	podInfo := cns.KubernetesPodInfo{PodName: podName, PodNamespace: podNamespace}
	orchestratorContext, err := json.Marshal(podInfo)
	if err != nil {
		t.Fatal(err)
	}

	// no IP reservation found with that context, expect fail
	err = cnsClient.ReleaseIPAddress(orchestratorContext)
	if err == nil {
		t.Fatalf("Expected failure to release when no IP reservation found with context: %+v", err)
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
