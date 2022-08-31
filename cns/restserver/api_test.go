// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package restserver

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/common"
	"github.com/Azure/azure-container-networking/cns/fakes"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/nmagent"
	"github.com/Azure/azure-container-networking/cns/types"
	acncommon "github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/processlock"
	"github.com/Azure/azure-container-networking/store"
	"github.com/stretchr/testify/assert"
)

const (
	cnsJsonFileName = "azure-cns.json"
)

type IPAddress struct {
	XMLName   xml.Name `xml:"IPAddress"`
	Address   string   `xml:"Address,attr"`
	IsPrimary bool     `xml:"IsPrimary,attr"`
}

type IPSubnet struct {
	XMLName   xml.Name `xml:"IPSubnet"`
	Prefix    string   `xml:"Prefix,attr"`
	IPAddress []IPAddress
}

type Interface struct {
	XMLName    xml.Name `xml:"Interface"`
	MacAddress string   `xml:"MacAddress,attr"`
	IsPrimary  bool     `xml:"IsPrimary,attr"`
	IPSubnet   []IPSubnet
}

type xmlDocument struct {
	XMLName   xml.Name `xml:"Interfaces"`
	Interface []Interface
}

var (
	service           cns.HTTPService
	svc               *HTTPRestService
	mux               *http.ServeMux
	hostQueryResponse = xmlDocument{
		XMLName: xml.Name{Local: "Interfaces"},
		Interface: []Interface{{
			XMLName:    xml.Name{Local: "Interface"},
			MacAddress: "*",
			IsPrimary:  true,
			IPSubnet: []IPSubnet{
				{
					XMLName: xml.Name{Local: "IPSubnet"},
					Prefix:  "10.0.0.0/16",
					IPAddress: []IPAddress{
						{
							XMLName:   xml.Name{Local: "IPAddress"},
							Address:   "10.0.0.4",
							IsPrimary: true,
						},
					},
				},
			},
		}},
	}
)

const (
	nmagentEndpoint = "localhost:9000"
)

type createOrUpdateNetworkContainerParams struct {
	ncID         string
	ncIP         string
	ncType       string
	ncVersion    string
	vnetID       string
	podName      string
	podNamespace string
}

func getInterfaceInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(acncommon.ContentType, "application/xml")
	output, _ := xml.Marshal(hostQueryResponse)
	w.Write(output)
}

func nmagentHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(acncommon.ContentType, acncommon.JsonContent)

	if strings.Contains(r.RequestURI, "nc-nma-success") {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"httpStatusCode":"200","networkContainerId":"nc-nma-success","version":"0"}`))
	}

	if strings.Contains(r.RequestURI, "nc-nma-fail-version-mismatch") {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"httpStatusCode":"200","networkContainerId":"nc-nma-fail-version-mismatch","version":"0"}`))
	}

	if strings.Contains(r.RequestURI, "nc-nma-fail-500") {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"httpStatusCode":"200","networkContainerId":"nc-nma-fail-500","version":"0"}`))
	}

	if strings.Contains(r.RequestURI, "nc-nma-fail-unavailable") {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"httpStatusCode":"401","networkContainerId":"nc-nma-fail-unavailable","version":"0"}`))
	}
}

// Wraps the test run with service setup and teardown.
func TestMain(m *testing.M) {
	var err error
	logger.InitLogger("testlogs", 0, 0, "./")

	// Create the service.
	if err = startService(); err != nil {
		fmt.Printf("Failed to start CNS Service. Error: %v", err)
		os.Exit(1)
	}

	// Setup mock nmagent server
	u, err := url.Parse("tcp://" + nmagentEndpoint)
	if err != nil {
		fmt.Println(err.Error())
	}

	nmAgentServer, err := acncommon.NewListener(u)
	if err != nil {
		fmt.Println(err.Error())
	}

	nmAgentServer.AddHandler("/getInterface", getInterfaceInfo)
	nmAgentServer.AddHandler("/", nmagentHandler)
	nmagent.WireserverIP = nmagentEndpoint

	err = nmAgentServer.Start(make(chan error, 1))
	if err != nil {
		fmt.Printf("Failed to start agent, err:%v.\n", err)
		return
	}

	// Run tests.
	exitCode := m.Run()

	// Cleanup.
	service.Stop()
	nmAgentServer.Stop()

	os.Exit(exitCode)
}

func TestSetEnvironment(t *testing.T) {
	fmt.Println("Test: SetEnvironment")

	var resp cns.Response
	w := setEnv(t)

	err := decodeResponse(w, &resp)
	if err != nil || resp.ReturnCode != 0 {
		t.Errorf("SetEnvironment failed with response %+v", resp)
	} else {
		fmt.Printf("SetEnvironment Responded with %+v\n", resp)
	}
}

func TestSetOrchestratorType(t *testing.T) {
	fmt.Println("Test: TestSetOrchestratorType")

	setEnv(t)

	err := setOrchestratorType(t, cns.Kubernetes)
	if err != nil {
		t.Errorf("setOrchestratorType failed Err:%+v", err)
		t.Fatal(err)
	}
}

func FirstByte(b []byte, err error) []byte {
	if err != nil {
		panic(err)
	}
	return b
}

func FirstRequest(req *http.Request, err error) *http.Request {
	if err != nil {
		panic(err)
	}
	return req
}

func TestSetOrchestratorType_NCsPresent(t *testing.T) {
	tests := []struct {
		name          string
		service       *HTTPRestService
		writer        *httptest.ResponseRecorder
		request       *http.Request
		response      cns.Response
		wanthttperror bool
	}{
		{
			name: "Node already set, and has NCs, so registration should fail",
			service: &HTTPRestService{
				state: &httpRestServiceState{
					NodeID: "node1",
					ContainerStatus: map[string]containerstatus{
						"nc1": {},
					},
					ContainerIDByOrchestratorContext: map[string]string{
						"nc1": "present",
					},
				},
			},
			writer: httptest.NewRecorder(),
			request: FirstRequest(http.NewRequestWithContext(
				context.TODO(), http.MethodPost, cns.SetOrchestratorType, bytes.NewReader(
					FirstByte(json.Marshal( //nolint:errchkjson //inline map, only using returned bytes
						cns.SetOrchestratorTypeRequest{
							OrchestratorType: "Kubernetes",
							DncPartitionKey:  "partition1",
							NodeID:           "node2",
						}))))),
			response: cns.Response{
				ReturnCode: types.InvalidRequest,
				Message:    "Invalid request since this node has already been registered as node1",
			},
			wanthttperror: false,
		},
		{
			name: "Node already set, with no NCs, so registration should succeed",
			service: &HTTPRestService{
				state: &httpRestServiceState{
					NodeID: "node1",
				},
			},
			writer: httptest.NewRecorder(),
			request: FirstRequest(http.NewRequestWithContext(
				context.TODO(), http.MethodPost, cns.SetOrchestratorType, bytes.NewReader(
					FirstByte(json.Marshal( //nolint:errchkjson //inline map, only using returned bytes
						cns.SetOrchestratorTypeRequest{
							OrchestratorType: "Kubernetes",
							DncPartitionKey:  "partition1",
							NodeID:           "node2",
						}))))),
			response: cns.Response{
				ReturnCode: types.Success,
				Message:    "",
			},
			wanthttperror: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			var resp cns.Response
			// Since this is global, we have to replace the state
			oldstate := svc.state
			svc.state = tt.service.state
			mux.ServeHTTP(tt.writer, tt.request)
			// Replace back old state
			svc.state = oldstate

			err := decodeResponse(tt.writer, &resp)
			if tt.wanthttperror {
				assert.NotNil(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.response, resp)
		})
	}
}

func TestCreateNetworkContainer(t *testing.T) {
	// requires more than 30 seconds to run
	fmt.Println("Test: TestCreateNetworkContainer")

	setEnv(t)
	setOrchestratorType(t, cns.ServiceFabric)

	// Test create network container of type JobObject
	fmt.Println("TestCreateNetworkContainer: JobObject")

	params := createOrUpdateNetworkContainerParams{
		ncID:         "testJobObject",
		ncIP:         "10.1.0.5",
		ncType:       "JobObject",
		ncVersion:    "0",
		podName:      "testpod",
		podNamespace: "testpodnamespace",
	}

	err := createOrUpdateNetworkContainerWithParams(t, params)
	if err != nil {
		t.Errorf("Failed to save the goal state for network container of type JobObject "+
			" due to error: %+v", err)
		t.Fatal(err)
	}

	fmt.Println("Deleting the saved goal state for network container of type JobObject")
	err = deleteNetworkContainerWithParams(t, params)
	if err != nil {
		t.Errorf("Failed to delete the saved goal state due to error: %+v", err)
		t.Fatal(err)
	}

	// Test create network container of type WebApps
	fmt.Println("TestCreateNetworkContainer: WebApps")
	params = createOrUpdateNetworkContainerParams{
		ncID:         "ethWebApp",
		ncIP:         "192.0.0.5",
		ncType:       "WebApps",
		ncVersion:    "0",
		podName:      "testpod",
		podNamespace: "testpodnamespace",
	}

	err = createOrUpdateNetworkContainerWithParams(t, params)
	if err != nil {
		t.Errorf("creatOrUpdateWebAppContainerWithName failed Err:%+v", err)
		t.Fatal(err)
	}

	params = createOrUpdateNetworkContainerParams{
		ncID:         "ethWebApp",
		ncIP:         "192.0.0.6",
		ncType:       "WebApps",
		ncVersion:    "0",
		podName:      "testpod",
		podNamespace: "testpodnamespace",
	}

	err = createOrUpdateNetworkContainerWithParams(t, params)
	if err != nil {
		t.Errorf("Updating interface failed Err:%+v", err)
		t.Fatal(err)
	}

	fmt.Println("Now calling DeleteNetworkContainer")

	err = deleteNetworkContainerWithParams(t, params)
	if err != nil {
		t.Errorf("Deleting interface failed Err:%+v", err)
		t.Fatal(err)
	}

	// Test create network container of type COW
	params = createOrUpdateNetworkContainerParams{
		ncID:         "testCOWContainer",
		ncIP:         "10.0.0.5",
		ncType:       "COW",
		ncVersion:    "0",
		podName:      "testpod",
		podNamespace: "testpodnamespace",
	}

	err = createOrUpdateNetworkContainerWithParams(t, params)
	if err != nil {
		t.Errorf("Failed to save the goal state for network container of type COW"+
			" due to error: %+v", err)
		t.Fatal(err)
	}

	fmt.Println("Deleting the saved goal state for network container of type COW")
	err = deleteNetworkContainerWithParams(t, params)
	if err != nil {
		t.Errorf("Failed to delete the saved goal state due to error: %+v", err)
		t.Fatal(err)
	}
}

func TestGetNetworkContainerByOrchestratorContext(t *testing.T) {
	// requires more than 30 seconds to run
	fmt.Println("Test: TestGetNetworkContainerByOrchestratorContext")

	setEnv(t)
	setOrchestratorType(t, cns.Kubernetes)

	params := createOrUpdateNetworkContainerParams{
		ncID:         "ethWebApp",
		ncIP:         "11.0.0.5",
		ncType:       cns.AzureContainerInstance,
		ncVersion:    "0",
		podName:      "testpod",
		podNamespace: "testpodnamespace",
	}

	err := createOrUpdateNetworkContainerWithParams(t, params)
	if err != nil {
		t.Errorf("createOrUpdateNetworkContainerWithParams failed Err:%+v", err)
		t.Fatal(err)
	}

	fmt.Println("Now calling getNetworkContainerByContext")
	err = getNetworkContainerByContext(t, params)
	if err != nil {
		t.Errorf("TestGetNetworkContainerByOrchestratorContext failed Err:%+v", err)
		t.Fatal(err)
	}

	fmt.Println("Now calling DeleteNetworkContainer")

	err = deleteNetworkContainerWithParams(t, params)
	if err != nil {
		t.Errorf("Deleting interface failed Err:%+v", err)
		t.Fatal(err)
	}

	err = getNonExistNetworkContainerByContext(t, params)
	if err != nil {
		t.Errorf("TestGetNetworkContainerByOrchestratorContext failed Err:%+v", err)
		t.Fatal(err)
	}
}

func TestGetInterfaceForNetworkContainer(t *testing.T) {
	// requires more than 30 seconds to run
	fmt.Println("Test: TestCreateNetworkContainer")

	setEnv(t)
	setOrchestratorType(t, cns.Kubernetes)

	params := createOrUpdateNetworkContainerParams{
		ncID:         "ethWebApp",
		ncIP:         "11.0.0.5",
		ncType:       "WebApps",
		ncVersion:    "0",
		podName:      "testpod",
		podNamespace: "testpodnamespace",
	}

	err := createOrUpdateNetworkContainerWithParams(t, params)
	if err != nil {
		t.Errorf("creatOrUpdateWebAppContainerWithName failed Err:%+v", err)
		t.Fatal(err)
	}

	fmt.Println("Now calling getInterfaceForContainer")
	err = getInterfaceForContainer(t, params)
	if err != nil {
		t.Errorf("getInterfaceForContainer failed Err:%+v", err)
		t.Fatal(err)
	}

	fmt.Println("Now calling DeleteNetworkContainer")

	err = deleteNetworkContainerWithParams(t, params)
	if err != nil {
		t.Errorf("Deleting interface failed Err:%+v", err)
		t.Fatal(err)
	}
}

func TestGetNumOfCPUCores(t *testing.T) {
	fmt.Println("Test: getNumberOfCPUCores")

	var (
		err error
		req *http.Request
	)

	req, err = http.NewRequest(http.MethodGet, cns.NumberOfCPUCoresPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	var numOfCoresResponse cns.NumOfCPUCoresResponse

	err = decodeResponse(w, &numOfCoresResponse)
	if err != nil || numOfCoresResponse.Response.ReturnCode != 0 {
		t.Errorf("getNumberOfCPUCores failed with response %+v", numOfCoresResponse)
	} else {
		fmt.Printf("getNumberOfCPUCores Responded with %+v\n", numOfCoresResponse)
	}
}

func TestGetNetworkContainerVersionStatus(t *testing.T) {
	fmt.Println("Test: TestGetNetworkContainerVersionStatus")

	setEnv(t)
	setOrchestratorType(t, cns.Kubernetes)

	params := createOrUpdateNetworkContainerParams{
		ncID:         "nc-nma-success",
		ncIP:         "11.0.0.5",
		ncType:       cns.AzureContainerInstance,
		ncVersion:    "0",
		vnetID:       "vnet1",
		podName:      "testpod",
		podNamespace: "testpodnamespace",
	}

	createNC(t, params, false)

	if err := getNetworkContainerByContext(t, params); err != nil {
		t.Errorf("TestGetNetworkContainerByOrchestratorContext failed Err:%+v", err)
		t.Fatal(err)
	}

	// Get NC goal state again to test the path where the NMA API doesn't need to be executed but
	// instead use the cached state ( in json ) of version status
	if err := getNetworkContainerByContext(t, params); err != nil {
		t.Errorf("TestGetNetworkContainerByOrchestratorContext failed Err:%+v", err)
		t.Fatal(err)
	}

	if err := deleteNetworkContainerWithParams(t, params); err != nil {
		t.Errorf("Deleting interface failed Err:%+v", err)
		t.Fatal(err)
	}

	// Testing the path where the NC version with CNS is higher than the one with NMAgent.
	// This indicates that the NMAgent is yet to program the NC version.
	params = createOrUpdateNetworkContainerParams{
		ncID:         "nc-nma-fail-version-mismatch",
		ncIP:         "11.0.0.5",
		ncType:       cns.AzureContainerInstance,
		ncVersion:    "1",
		vnetID:       "vnet1",
		podName:      "testpod",
		podNamespace: "testpodnamespace",
	}

	createNC(t, params, false)

	if err := getNetworkContainerByContextExpectedError(t, params); err != nil {
		t.Errorf("TestGetNetworkContainerVersionStatus failed")
		t.Fatal(err)
	}

	if err := deleteNetworkContainerWithParams(t, params); err != nil {
		t.Errorf("Deleting interface failed Err:%+v", err)
		t.Fatal(err)
	}

	// Testing the path where NMAgent response status code is not 200.
	// 2. NMAgent response status code is 200 but embedded response is 401
	params = createOrUpdateNetworkContainerParams{
		ncID:         "nc-nma-fail-500",
		ncIP:         "11.0.0.5",
		ncType:       cns.AzureContainerInstance,
		ncVersion:    "0",
		vnetID:       "vnet1",
		podName:      "testpod",
		podNamespace: "testpodnamespace",
	}

	createNC(t, params, true)

	if err := getNetworkContainerByContext(t, params); err != nil {
		t.Errorf("TestGetNetworkContainerVersionStatus failed")
		t.Fatal(err)
	}

	if err := deleteNetworkContainerWithParams(t, params); err != nil {
		t.Errorf("Deleting interface failed Err:%+v", err)
		t.Fatal(err)
	}

	// Testing the path where NMAgent response status code is 200 but embedded response is 401
	params = createOrUpdateNetworkContainerParams{
		ncID:         "nc-nma-fail-unavailable",
		ncIP:         "11.0.0.5",
		ncType:       cns.AzureContainerInstance,
		ncVersion:    "0",
		vnetID:       "vnet1",
		podName:      "testpod",
		podNamespace: "testpodnamespace",
	}

	createNC(t, params, false)

	if err := getNetworkContainerByContextExpectedError(t, params); err != nil {
		t.Errorf("TestGetNetworkContainerVersionStatus failed")
		t.Fatal(err)
	}

	if err := deleteNetworkContainerWithParams(t, params); err != nil {
		t.Errorf("Deleting interface failed Err:%+v", err)
		t.Fatal(err)
	}
}

//nolint:gocritic // param is just for testing
func createNC(
	t *testing.T,
	params createOrUpdateNetworkContainerParams,
	expectError bool,
) {
	if err := createOrUpdateNetworkContainerWithParams(t, params); err != nil {
		t.Errorf("createOrUpdateNetworkContainerWithParams failed Err:%+v", err)
		t.Fatal(err)
	}

	createNetworkContainerURL := "http://" + nmagentEndpoint +
		"/machine/plugins/?comp=nmagent&type=NetworkManagement/interfaces/dummyIntf/networkContainers/dummyNCURL/authenticationToken/dummyT/api-version/1"

	err := publishNCViaCNS(t, params.vnetID, params.ncID, createNetworkContainerURL, expectError)
	if err != nil {
		t.Fatal(err)
	}
}

func TestPublishNCViaCNS(t *testing.T) {
	fmt.Println("Test: publishNetworkContainer")

	createNetworkContainerURL := "http://" + nmagentEndpoint +
		"/machine/plugins/?comp=nmagent&type=NetworkManagement/interfaces/dummyIntf/networkContainers/dummyNCURL/authenticationToken/dummyT/api-version/1"
	err := publishNCViaCNS(t, "vnet1", "ethWebApp", createNetworkContainerURL, false)
	if err != nil {
		t.Fatal(fmt.Errorf("publish container failed %w ", err))
	}

	createNetworkContainerURL = "http://" + nmagentEndpoint +
		"/machine/plugins/?comp=nmagent&type=NetworkManagement/interfaces/dummyIntf/networkContainers/dummyNCURL/authenticationTok/" +
		"8636c99d-7861-401f-b0d3-7e5b7dc8183c" +
		"/api-version/1"

	err = publishNCViaCNS(t, "vnet1", "ethWebApp", createNetworkContainerURL, true)
	if err == nil {
		t.Fatal("Expected a bad request error due to create network url being incorrect")
	}

	createNetworkContainerURL = "http://" + nmagentEndpoint +
		"/machine/plugins/?comp=nmagent&NetworkManagement/interfaces/dummyIntf/networkContainers/dummyNCURL/authenticationToken/" +
		"8636c99d-7861-401f-b0d3-7e5b7dc8183c8636c99d-7861-401f-b0d3-7e5b7dc8183c" +
		"/api-version/1"

	err = publishNCViaCNS(t, "vnet1", "ethWebApp", createNetworkContainerURL, true)
	if err == nil {
		t.Fatal("Expected a bad request error due to create network url having more characters than permitted in auth token")
	}
}

func publishNCViaCNS(t *testing.T,
	networkID,
	networkContainerID,
	createNetworkContainerURL string,
	expectError bool,
) error {
	var (
		body bytes.Buffer
		resp cns.PublishNetworkContainerResponse
	)

	joinNetworkURL := "http://" + nmagentEndpoint + "/dummyVnetURL"

	publishNCRequest := &cns.PublishNetworkContainerRequest{
		NetworkID:                         networkID,
		NetworkContainerID:                networkContainerID,
		JoinNetworkURL:                    joinNetworkURL,
		CreateNetworkContainerURL:         createNetworkContainerURL,
		CreateNetworkContainerRequestBody: make([]byte, 0),
	}

	json.NewEncoder(&body).Encode(publishNCRequest)
	req, err := http.NewRequest(http.MethodPost, cns.PublishNetworkContainer, &body)
	if err != nil {
		return fmt.Errorf("Failed to create publish request %w", err)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	err = decodeResponse(w, &resp)
	if err != nil || resp.Response.ReturnCode != 0 {
		if !expectError {
			t.Errorf("PublishNetworkContainer failed with response %+v Err:%+v", resp, err)
		}
		return err
	}

	fmt.Printf("PublishNetworkContainer succeded with response %+v, raw:%+v\n", resp, w.Body)
	return nil
}

func TestUnpublishNCViaCNS(t *testing.T) {
	deleteNetworkContainerURL := "http://" + nmagentEndpoint +
		"/machine/plugins/?comp=nmagent&type=NetworkManagement/interfaces/dummyIntf/networkContainers/dummyNCURL/authenticationToken/dummyT/api-version/1/method/DELETE"
	err := publishNCViaCNS(t, "vnet1", "ethWebApp", deleteNetworkContainerURL, false)
	if err != nil {
		t.Fatal(err)
	}

	deleteNetworkContainerURL = "http://" + nmagentEndpoint +
		"/machine/plugins/?comp=nmagent&type=NetworkManagement/interfaces/dummyIntf/networkContainers/dummyNCURL/authenticationToke/" +
		"8636c99d-7861-401f-b0d3-7e5b7dc8183c" +
		"/api-version/1/method/DELETE"

	err = publishNCViaCNS(t, "vnet1", "ethWebApp", deleteNetworkContainerURL, true)
	if err == nil {
		t.Fatal("Expected a bad request error due to delete network url being incorrect")
	}

	deleteNetworkContainerURL = "http://" + nmagentEndpoint +
		"/machine/plugins/?comp=nmagent&NetworkManagement/interfaces/dummyIntf/networkContainers/dummyNCURL/authenticationToken/" +
		"8636c99d-7861-401f-b0d3-7e5b7dc8183c8636c99d-7861-401f-b0d3-7e5b7dc8183c" +
		"/api-version/1/method/DELETE"

	err = testUnpublishNCViaCNS(t, "vnet1", "ethWebApp", deleteNetworkContainerURL, true)
	if err == nil {
		t.Fatal("Expected a bad request error due to create network url having more characters than permitted in auth token")
	}
}

func testUnpublishNCViaCNS(t *testing.T,
	networkID,
	networkContainerID,
	deleteNetworkContainerURL string,
	expectError bool,
) error {
	var (
		body bytes.Buffer
		resp cns.UnpublishNetworkContainerResponse
	)

	fmt.Println("Test: unpublishNetworkContainer")

	joinNetworkURL := "http://" + nmagentEndpoint + "/dummyVnetURL"

	unpublishNCRequest := &cns.UnpublishNetworkContainerRequest{
		NetworkID:                 networkID,
		NetworkContainerID:        networkContainerID,
		JoinNetworkURL:            joinNetworkURL,
		DeleteNetworkContainerURL: deleteNetworkContainerURL,
	}

	json.NewEncoder(&body).Encode(unpublishNCRequest)
	req, err := http.NewRequest(http.MethodPost, cns.UnpublishNetworkContainer, &body)
	if err != nil {
		return fmt.Errorf("Failed to create unpublish request %w", err)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	err = decodeResponse(w, &resp)
	if err != nil || resp.Response.ReturnCode != 0 {
		if !expectError {
			t.Errorf("UnpublishNetworkContainer failed with response %+v Err:%+v", resp, err)
		}
		return err
	}

	fmt.Printf("UnpublishNetworkContainer succeded with response %+v, raw:%+v\n", resp, w.Body)

	return nil
}

func TestNmAgentSupportedApisHandler(t *testing.T) {
	fmt.Println("Test: nmAgentSupportedApisHandler")

	var (
		err        error
		req        *http.Request
		nmAgentReq cns.NmAgentSupportedApisRequest
		body       bytes.Buffer
	)

	json.NewEncoder(&body).Encode(nmAgentReq)
	req, err = http.NewRequest(http.MethodGet, cns.NmAgentSupportedApisPath, &body)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	var nmAgentSupportedApisResponse cns.NmAgentSupportedApisResponse

	err = decodeResponse(w, &nmAgentSupportedApisResponse)
	if err != nil || nmAgentSupportedApisResponse.Response.ReturnCode != 0 {
		t.Errorf("nmAgentSupportedApisHandler failed with response %+v", nmAgentSupportedApisResponse)
	}

	// Since we are testing the NMAgent API in internalapi_test, we will skip POST call
	// and test other paths
	fmt.Printf("nmAgentSupportedApisHandler Responded with %+v\n", nmAgentSupportedApisResponse)
}

func TestCreateHostNCApipaEndpoint(t *testing.T) {
	fmt.Println("Test: createHostNCApipaEndpoint")

	var (
		err           error
		req           *http.Request
		createHostReq cns.CreateHostNCApipaEndpointRequest
		body          bytes.Buffer
	)

	json.NewEncoder(&body).Encode(createHostReq)
	req, err = http.NewRequest(http.MethodPost, cns.CreateHostNCApipaEndpointPath, &body)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	var createHostNCApipaEndpointResponse cns.CreateHostNCApipaEndpointResponse

	err = decodeResponse(w, &createHostNCApipaEndpointResponse)
	if err != nil || createHostNCApipaEndpointResponse.Response.ReturnCode != types.UnknownContainerID {
		t.Errorf("createHostNCApipaEndpoint failed with response %+v", createHostNCApipaEndpointResponse)
	}

	fmt.Printf("createHostNCApipaEndpoint Responded with %+v\n", createHostNCApipaEndpointResponse)
}

func setOrchestratorType(t *testing.T, orchestratorType string) error {
	var body bytes.Buffer

	info := &cns.SetOrchestratorTypeRequest{OrchestratorType: orchestratorType}

	json.NewEncoder(&body).Encode(info)

	req, err := http.NewRequest(http.MethodPost, cns.SetOrchestratorType, &body)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var resp cns.Response
	err = decodeResponse(w, &resp)
	fmt.Printf("Raw response: %+v", w.Body)
	if err != nil || resp.ReturnCode != 0 {
		t.Errorf("setOrchestratorType failed with response %+v Err:%+v", resp, err)
		t.Fatal(err)
	} else {
		fmt.Printf("setOrchestratorType passed with response %+v Err:%+v", resp, err)
	}

	fmt.Printf("setOrchestratorType succeeded with response %+v\n", resp)
	return nil
}

func createOrUpdateNetworkContainerWithParams(t *testing.T, params createOrUpdateNetworkContainerParams) error {
	var body bytes.Buffer
	var ipConfig cns.IPConfiguration
	ipConfig.DNSServers = []string{"8.8.8.8", "8.8.4.4"}
	ipConfig.GatewayIPAddress = "11.0.0.1"
	var ipSubnet cns.IPSubnet
	ipSubnet.IPAddress = params.ncIP
	ipSubnet.PrefixLength = 24
	ipConfig.IPSubnet = ipSubnet
	podInfo := cns.KubernetesPodInfo{PodName: "testpod", PodNamespace: "testpodnamespace"}
	context, _ := json.Marshal(podInfo)

	info := &cns.CreateNetworkContainerRequest{
		Version:                    params.ncVersion,
		NetworkContainerType:       params.ncType,
		NetworkContainerid:         cns.SwiftPrefix + params.ncID,
		OrchestratorContext:        context,
		IPConfiguration:            ipConfig,
		PrimaryInterfaceIdentifier: "11.0.0.7",
	}

	json.NewEncoder(&body).Encode(info)

	req, err := http.NewRequest(http.MethodPost, cns.CreateOrUpdateNetworkContainer, &body)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	var resp cns.CreateNetworkContainerResponse
	err = decodeResponse(w, &resp)
	fmt.Printf("Raw response: %+v", w.Body)

	if err != nil || resp.Response.ReturnCode != 0 {
		t.Errorf("CreateNetworkContainerRequest failed with response %+v Err:%+v", resp, err)
		t.Fatal(err)
	} else {
		fmt.Printf("CreateNetworkContainerRequest passed with response %+v Err:%+v", resp, err)
	}

	fmt.Printf("CreateNetworkContainerRequest succeeded with response %+v\n", resp)
	return nil
}

func deleteNetworkContainerWithParams(t *testing.T, params createOrUpdateNetworkContainerParams) error {
	var (
		body bytes.Buffer
		resp cns.DeleteNetworkContainerResponse
	)

	deleteInfo := &cns.DeleteNetworkContainerRequest{
		NetworkContainerid: cns.SwiftPrefix + params.ncID,
	}

	json.NewEncoder(&body).Encode(deleteInfo)
	req, err := http.NewRequest(http.MethodPost, cns.DeleteNetworkContainer, &body)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	err = decodeResponse(w, &resp)
	if err != nil || resp.Response.ReturnCode != 0 {
		t.Errorf("DeleteNetworkContainer failed with response %+v Err:%+v", resp, err)
		t.Fatal(err)
	}

	fmt.Printf("DeleteNetworkContainer succeded with response %+v\n", resp)
	return nil
}

func getNetworkContainerByContext(t *testing.T, params createOrUpdateNetworkContainerParams) error {
	var body bytes.Buffer
	var resp cns.GetNetworkContainerResponse
	podInfo := cns.KubernetesPodInfo{PodName: params.podName, PodNamespace: params.podNamespace}

	podInfoBytes, err := json.Marshal(podInfo)
	getReq := &cns.GetNetworkContainerRequest{OrchestratorContext: podInfoBytes}

	json.NewEncoder(&body).Encode(getReq)
	req, err := http.NewRequest(http.MethodPost, cns.GetNetworkContainerByOrchestratorContext, &body)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	err = decodeResponse(w, &resp)
	if err != nil || resp.Response.ReturnCode != 0 {
		t.Errorf("GetNetworkContainerByContext failed with response %+v Err:%+v", resp, err)
		t.Fatal(err)
	}

	fmt.Printf("**GetNetworkContainerByContext succeded with response %+v, raw:%+v\n", resp, w.Body)
	return nil
}

func getNonExistNetworkContainerByContext(t *testing.T, params createOrUpdateNetworkContainerParams) error {
	var body bytes.Buffer
	var resp cns.GetNetworkContainerResponse
	podInfo := cns.KubernetesPodInfo{PodName: params.podName, PodNamespace: params.podNamespace}

	podInfoBytes, err := json.Marshal(podInfo)
	getReq := &cns.GetNetworkContainerRequest{OrchestratorContext: podInfoBytes}

	json.NewEncoder(&body).Encode(getReq)
	req, err := http.NewRequest(http.MethodPost, cns.GetNetworkContainerByOrchestratorContext, &body)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	err = decodeResponse(w, &resp)
	if err != nil || resp.Response.ReturnCode != types.UnknownContainerID {
		t.Errorf("GetNetworkContainerByContext unexpected response %+v Err:%+v", resp, err)
		t.Fatal(err)
	}

	fmt.Printf("**GetNonExistNetworkContainerByContext succeded with response %+v, raw:%+v\n", resp, w.Body)
	return nil
}

func getNetworkContainerByContextExpectedError(t *testing.T, params createOrUpdateNetworkContainerParams) error {
	var body bytes.Buffer
	var resp cns.GetNetworkContainerResponse
	podInfo := cns.KubernetesPodInfo{PodName: params.podName, PodNamespace: params.podNamespace}

	podInfoBytes, err := json.Marshal(podInfo)
	getReq := &cns.GetNetworkContainerRequest{OrchestratorContext: podInfoBytes}

	json.NewEncoder(&body).Encode(getReq)
	req, err := http.NewRequest(http.MethodPost, cns.GetNetworkContainerByOrchestratorContext, &body)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	err = decodeResponse(w, &resp)
	if err != nil || resp.Response.ReturnCode == 0 {
		t.Errorf("GetNetworkContainerByContext failed with response %+v Err:%+v", resp, err)
		t.Fatal(err)
	}

	fmt.Printf("**getNetworkContainerByContextExpectedError succeded with response %+v, raw:%+v\n", resp, w.Body)
	return nil
}

func getInterfaceForContainer(t *testing.T, params createOrUpdateNetworkContainerParams) error {
	var body bytes.Buffer
	var resp cns.GetInterfaceForContainerResponse

	getReq := &cns.GetInterfaceForContainerRequest{
		NetworkContainerID: cns.SwiftPrefix + params.ncID,
	}

	json.NewEncoder(&body).Encode(getReq)
	req, err := http.NewRequest(http.MethodPost, cns.GetInterfaceForContainer, &body)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	err = decodeResponse(w, &resp)
	if err != nil || resp.Response.ReturnCode != 0 {
		t.Errorf("GetInterfaceForContainer failed with response %+v Err:%+v", resp, err)
		t.Fatal(err)
	}

	fmt.Printf("**GetInterfaceForContainer succeded with response %+v, raw:%+v\n", resp, w.Body)
	return nil
}

// Decodes service's responses to test requests.
func decodeResponse(w *httptest.ResponseRecorder, response interface{}) error {
	if w.Code != http.StatusOK {
		return fmt.Errorf("Request failed with HTTP error %d", w.Code)
	}

	if w.Result().Body == nil {
		return fmt.Errorf("Response body is empty")
	}

	return json.NewDecoder(w.Body).Decode(&response)
}

func setEnv(t *testing.T) *httptest.ResponseRecorder {
	envRequest := cns.SetEnvironmentRequest{Location: "Azure", NetworkType: "Underlay"}
	envRequestJSON := new(bytes.Buffer)
	json.NewEncoder(envRequestJSON).Encode(envRequest)

	req, err := http.NewRequest(http.MethodPost, cns.V2Prefix+cns.SetEnvironmentPath, envRequestJSON)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func startService() error {
	var err error
	// Create the service.
	config := common.ServiceConfig{}
	// Create the key value store.
	if config.Store, err = store.NewJsonFileStore(cnsJsonFileName, processlock.NewMockFileLock(false)); err != nil {
		logger.Errorf("Failed to create store file: %s, due to error %v\n", cnsJsonFileName, err)
		return err
	}

	nmagentClient := &fakes.NMAgentClientFake{}
	service, err = NewHTTPRestService(&config, &fakes.WireserverClientFake{}, nmagentClient, nil)
	if err != nil {
		return err
	}
	svc = service.(*HTTPRestService)
	svc.Name = "cns-test-server"
	if err != nil {
		logger.Errorf("Failed to create CNS object, err:%v.\n", err)
		return err
	}

	svc.IPAMPoolMonitor = &fakes.MonitorFake{}
	nmagentClient.GetNCVersionListFunc = func(context.Context) (*nmagent.NetworkContainerListResponse, error) {
		var hostVersionNeedsUpdateContainers []string
		for idx := range svc.state.ContainerStatus {
			hostVersion, err := strconv.Atoi(svc.state.ContainerStatus[idx].HostVersion) //nolint:govet // intentional shadowing
			if err != nil {
				logger.Errorf("Received err when change containerstatus.HostVersion %s to int, err msg %v", svc.state.ContainerStatus[idx].HostVersion, err)
				continue
			}
			dncNcVersion, err := strconv.Atoi(svc.state.ContainerStatus[idx].CreateNetworkContainerRequest.Version)
			if err != nil {
				logger.Errorf("Received err when change nc version %s in containerstatus to int, err msg %v", svc.state.ContainerStatus[idx].CreateNetworkContainerRequest.Version, err)
				continue
			}
			// host NC version is the NC version from NMAgent, if it's smaller than NC version from DNC, then append it to indicate it needs update.
			if hostVersion < dncNcVersion {
				hostVersionNeedsUpdateContainers = append(hostVersionNeedsUpdateContainers, svc.state.ContainerStatus[idx].ID)
			} else if hostVersion > dncNcVersion {
				logger.Errorf("NC version from NMAgent is larger than DNC, NC version from NMAgent is %d, NC version from DNC is %d", hostVersion, dncNcVersion)
			}
		}
		resp := &nmagent.NetworkContainerListResponse{
			Containers: []nmagent.ContainerInfo{},
		}
		for _, cs := range hostVersionNeedsUpdateContainers {
			resp.Containers = append(resp.Containers, nmagent.ContainerInfo{Version: "0", NetworkContainerID: cs})
		}
		return resp, nil
	}

	if service != nil {
		// Create empty azure-cns.json. CNS should start successfully by deleting this file
		file, _ := os.Create(cnsJsonFileName)
		file.Close()

		err = service.Init(&config)
		if err != nil {
			logger.Errorf("Failed to Init CNS, err:%v.\n", err)
			return err
		}

		err = service.Start(&config)
		if err != nil {
			logger.Errorf("Failed to start CNS, err:%v.\n", err)
			return err
		}

		if _, err := os.Stat(cnsJsonFileName); err == nil || !os.IsNotExist(err) {
			logger.Errorf("Failed to remove empty CNS state file: %s, err:%v", cnsJsonFileName, err)
			return err
		}
	}

	// Get the internal http mux as test hook.
	mux = service.(*HTTPRestService).Listener.GetMux()

	return nil
}

// IGNORE TEST AS IT IS FAILING. TODO:- Fix it https://msazure.visualstudio.com/One/_workitems/edit/7720083
// // Tests CreateNetwork functionality.

/*
func TestCreateNetwork(t *testing.T) {
	fmt.Println("Test: CreateNetwork")

	var body bytes.Buffer
	setEnv(t)
	// info := &cns.CreateNetworkRequest{
	// 	NetworkName: "azurenet",
	// }

	// json.NewEncoder(&body).Encode(info)

	// req, err := http.NewRequest(http.MethodPost, cns.CreateNetworkPath, &body)
	// if err != nil {
	// 	t.Fatal(err)
	// }

	// w := httptest.NewRecorder()
	// mux.ServeHTTP(w, req)

	httpc := &http.Client{}
	url := defaultCnsURL + cns.CreateNetworkPath
	log.Printf("CreateNetworkRequest url %v", url)

	payload := &cns.CreateNetworkRequest{
		NetworkName: "azurenet",
	}

	err := json.NewEncoder(&body).Encode(payload)
	if err != nil {
		t.Errorf("encoding json failed with %v", err)
	}

	res, err := httpc.Post(url, contentTypeJSON, &body)
	if err != nil {
		t.Errorf("[Azure cnsclient] HTTP Post returned error %v", err.Error())
	}

	defer res.Body.Close()
	var resp cns.Response

	// err = decodeResponse(res, &resp)
	// if err != nil || resp.ReturnCode != 0 {
	// 	t.Errorf("CreateNetwork failed with response %+v", resp)
	// } else {
	// 	fmt.Printf("CreateNetwork Responded with %+v\n", resp)
	// }

	err = json.NewDecoder(res.Body).Decode(&resp)
	if err != nil {
		t.Errorf("[Azure cnsclient] Error received while parsing ReleaseIPAddress response resp:%v err:%v", res.Body, err.Error())
	}

	if resp.ReturnCode != 0 {
		t.Errorf("[Azure cnsclient] ReleaseIPAddress received error response :%v", resp.Message)
		// return fmt.Errorf(resp.Message)
	}
}

func TestDeleteNetwork(t *testing.T) {
	fmt.Println("Test: DeleteNetwork")

	var body bytes.Buffer
	setEnv(t)
	info := &cns.DeleteNetworkRequest{
		NetworkName: "azurenet",
	}

	json.NewEncoder(&body).Encode(info)

	req, err := http.NewRequest(http.MethodPost, cns.DeleteNetworkPath, &body)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	var resp cns.Response

	err = decodeResponse(w, &resp)
	if err != nil || resp.ReturnCode != 0 {
		t.Errorf("DeleteNetwork failed with response %+v", resp)
	} else {
		fmt.Printf("DeleteNetwork Responded with %+v\n", resp)
	}
}

func TestReserveIPAddress(t *testing.T) {
	fmt.Println("Test: ReserveIPAddress")

	reserveIPRequest := cns.ReserveIPAddressRequest{ReservationID: "ip01"}
	reserveIPRequestJSON := new(bytes.Buffer)
	json.NewEncoder(reserveIPRequestJSON).Encode(reserveIPRequest)
	envRequest := cns.SetEnvironmentRequest{Location: "Azure", NetworkType: "Underlay"}
	envRequestJSON := new(bytes.Buffer)
	json.NewEncoder(envRequestJSON).Encode(envRequest)

	req, err := http.NewRequest(http.MethodPost, cns.ReserveIPAddressPath, envRequestJSON)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	var reserveIPAddressResponse cns.ReserveIPAddressResponse

	err = decodeResponse(w, &reserveIPAddressResponse)
	if err != nil || reserveIPAddressResponse.Response.ReturnCode != 0 {
		t.Errorf("SetEnvironment failed with response %+v", reserveIPAddressResponse)
	} else {
		fmt.Printf("SetEnvironment Responded with %+v\n", reserveIPAddressResponse)
	}
}

func TestReleaseIPAddress(t *testing.T) {
	fmt.Println("Test: ReleaseIPAddress")

	releaseIPRequest := cns.ReleaseIPAddressRequest{ReservationID: "ip01"}
	releaseIPAddressRequestJSON := new(bytes.Buffer)
	json.NewEncoder(releaseIPAddressRequestJSON).Encode(releaseIPRequest)

	req, err := http.NewRequest(http.MethodPost, cns.ReleaseIPAddressPath, releaseIPAddressRequestJSON)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	var releaseIPAddressResponse cns.Response

	err = decodeResponse(w, &releaseIPAddressResponse)
	if err != nil || releaseIPAddressResponse.ReturnCode != 0 {
		t.Errorf("SetEnvironment failed with response %+v", releaseIPAddressResponse)
	} else {
		fmt.Printf("SetEnvironment Responded with %+v\n", releaseIPAddressResponse)
	}
}

func TestGetUnhealthyIPAddresses(t *testing.T) {
	fmt.Println("Test: GetGhostIPAddresses")

	req, err := http.NewRequest(http.MethodGet, cns.GetUnhealthyIPAddressesPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	var getIPAddressesResponse cns.GetIPAddressesResponse

	err = decodeResponse(w, &getIPAddressesResponse)
	if err != nil || getIPAddressesResponse.Response.ReturnCode != 0 {
		t.Errorf("GetUnhealthyIPAddresses failed with response %+v", getIPAddressesResponse)
	} else {
		fmt.Printf("GetUnhealthyIPAddresses Responded with %+v\n", getIPAddressesResponse)
	}
}

func TestGetIPAddressUtilization(t *testing.T) {
	fmt.Println("Test: GetIPAddressUtilization")

	req, err := http.NewRequest(http.MethodGet, cns.GetIPAddressUtilizationPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	var iPAddressesUtilizationResponse cns.IPAddressesUtilizationResponse

	err = decodeResponse(w, &iPAddressesUtilizationResponse)
	if err != nil || iPAddressesUtilizationResponse.Response.ReturnCode != 0 {
		t.Errorf("GetIPAddressUtilization failed with response %+v\n", iPAddressesUtilizationResponse)
	} else {
		fmt.Printf("GetIPAddressUtilization Responded with %+v\n", iPAddressesUtilizationResponse)
	}
}

func TestGetHostLocalIP(t *testing.T) {
	fmt.Println("Test: GetHostLocalIP")

	setEnv(t)

	req, err := http.NewRequest(http.MethodGet, cns.GetHostLocalIPPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	var hostLocalIPAddressResponse cns.HostLocalIPAddressResponse

	err = decodeResponse(w, &hostLocalIPAddressResponse)
	if err != nil || hostLocalIPAddressResponse.Response.ReturnCode != 0 {
		t.Errorf("GetHostLocalIP failed with response %+v", hostLocalIPAddressResponse)
	} else {
		fmt.Printf("GetHostLocalIP Responded with %+v\n", hostLocalIPAddressResponse)
	}
}
*/
