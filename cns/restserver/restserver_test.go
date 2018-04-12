// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package restserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/common"
)

var service HTTPService
var mux *http.ServeMux

// Wraps the test run with service setup and teardown.
func TestMain(m *testing.M) {
	var config common.ServiceConfig
	var err error

	// Create the service.
	service, err = NewHTTPRestService(&config)
	if err != nil {
		fmt.Printf("Failed to create CNS object %v\n", err)
		os.Exit(1)
	}

	// Configure test mode.
	service.(*httpRestService).Name = "cns-test-server"

	// Start the service.
	err = service.Start(&config)
	if err != nil {
		fmt.Printf("Failed to start CNS %v\n", err)
		os.Exit(2)
	}

	// Get the internal http mux as test hook.
	mux = service.(*httpRestService).Listener.GetMux()

	// Run tests.
	exitCode := m.Run()

	// Cleanup.
	service.Stop()

	os.Exit(exitCode)
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

	req, err := http.NewRequest(http.MethodPost, cns.V1Prefix+cns.SetEnvironmentPath, envRequestJSON)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
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

// Tests CreateNetwork functionality.
func TestCreateNetwork(t *testing.T) {
	fmt.Println("Test: CreateNetwork")

	var body bytes.Buffer
	setEnv(t)
	info := &cns.CreateNetworkRequest{
		NetworkName: "azurenet",
	}

	json.NewEncoder(&body).Encode(info)

	req, err := http.NewRequest(http.MethodPost, cns.CreateNetworkPath, &body)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	var resp cns.Response

	err = decodeResponse(w, &resp)
	if err != nil || resp.ReturnCode != 0 {
		t.Errorf("CreateNetwork failed with response %+v", resp)
	} else {
		fmt.Printf("CreateNetwork Responded with %+v\n", resp)
	}
}

// Tests DeleteNetwork functionality.
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

func creatOrUpdateNetworkContainerWithName(t *testing.T, name string, ip string, containerType string) error {
	var body bytes.Buffer
	var ipConfig cns.IPConfiguration
	ipConfig.DNSServers = []string{"8.8.8.8", "8.8.4.4"}
	ipConfig.GatewayIPAddress = "11.0.0.1"
	var ipSubnet cns.IPSubnet
	ipSubnet.IPAddress = ip
	ipSubnet.PrefixLength = 24
	ipConfig.IPSubnet = ipSubnet
	podInfo := cns.KubernetesPodInfo{PodName: "testpod", PodNamespace: "testpodnamespace"}
	context, _ := json.Marshal(podInfo)

	info := &cns.CreateNetworkContainerRequest{
		Version:                    "0.1",
		NetworkContainerType:       containerType,
		NetworkContainerid:         name,
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

func deleteNetworkAdapterWithName(t *testing.T, name string) error {
	var body bytes.Buffer
	var resp cns.DeleteNetworkContainerResponse

	deleteInfo := &cns.DeleteNetworkContainerRequest{
		NetworkContainerid: "ethWebApp",
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

func getNetworkCotnainerByContext(t *testing.T, name string) error {
	var body bytes.Buffer
	var resp cns.GetNetworkContainerResponse

	podInfo := cns.KubernetesPodInfo{PodName: "testpod", PodNamespace: "testpodnamespace"}
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

func getNonExistNetworkCotnainerByContext(t *testing.T, name string) error {
	var body bytes.Buffer
	var resp cns.GetNetworkContainerResponse

	podInfo := cns.KubernetesPodInfo{PodName: "testpod", PodNamespace: "testpodnamespace"}
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
	if err != nil || resp.Response.ReturnCode != UnknownContainerID {
		t.Errorf("GetNetworkContainerByContext unexpected response %+v Err:%+v", resp, err)
		t.Fatal(err)
	}

	fmt.Printf("**GetNonExistNetworkContainerByContext succeded with response %+v, raw:%+v\n", resp, w.Body)
	return nil
}

func getNetworkCotnainerStatus(t *testing.T, name string) error {
	var body bytes.Buffer
	var resp cns.GetNetworkContainerStatusResponse

	getReq := &cns.GetNetworkContainerStatusRequest{
		NetworkContainerid: "ethWebApp",
	}

	json.NewEncoder(&body).Encode(getReq)
	req, err := http.NewRequest(http.MethodPost, cns.GetNetworkContainerStatus, &body)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	err = decodeResponse(w, &resp)
	if err != nil || resp.Response.ReturnCode != 0 {
		t.Errorf("GetNetworkContainerStatus failed with response %+v Err:%+v", resp, err)
		t.Fatal(err)
	}

	fmt.Printf("**GetNetworkContainerStatus succeded with response %+v, raw:%+v\n", resp, w.Body)
	return nil
}

func getInterfaceForContainer(t *testing.T, name string) error {
	var body bytes.Buffer
	var resp cns.GetInterfaceForContainerResponse

	getReq := &cns.GetInterfaceForContainerRequest{
		NetworkContainerID: "ethWebApp",
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

func TestSetOrchestratorType(t *testing.T) {
	fmt.Println("Test: TestSetOrchestratorType")

	setEnv(t)

	err := setOrchestratorType(t, cns.Kubernetes)
	if err != nil {
		t.Errorf("setOrchestratorType failed Err:%+v", err)
		t.Fatal(err)
	}
}

func TestCreateNetworkContainer(t *testing.T) {
	// requires more than 30 seconds to run
	fmt.Println("Test: TestCreateNetworkContainer")

	setEnv(t)

	err := creatOrUpdateNetworkContainerWithName(t, "ethWebApp", "11.0.0.5", "WebApps")
	if err != nil {
		t.Errorf("creatOrUpdateWebAppContainerWithName failed Err:%+v", err)
		t.Fatal(err)
	}

	err = creatOrUpdateNetworkContainerWithName(t, "ethWebApp", "11.0.0.6", "WebApps")
	if err != nil {
		t.Errorf("Updating interface failed Err:%+v", err)
		t.Fatal(err)
	}

	fmt.Println("Now calling DeleteNetworkContainer")

	err = deleteNetworkAdapterWithName(t, "ethWebApp")
	if err != nil {
		t.Errorf("Deleting interface failed Err:%+v", err)
		t.Fatal(err)
	}
}

func TestGetNetworkContainerByOrchestratorContext(t *testing.T) {
	// requires more than 30 seconds to run
	fmt.Println("Test: TestGetNetworkContainerByOrchestratorContext")

	setEnv(t)
	setOrchestratorType(t, cns.Kubernetes)

	err := creatOrUpdateNetworkContainerWithName(t, "ethWebApp", "11.0.0.5", "AzureContainerInstance")
	if err != nil {
		t.Errorf("creatOrUpdateNetworkContainerWithName failed Err:%+v", err)
		t.Fatal(err)
	}

	fmt.Println("Now calling getNetworkCotnainerStatus")
	err = getNetworkCotnainerByContext(t, "ethWebApp")
	if err != nil {
		t.Errorf("TestGetNetworkContainerByOrchestratorContext failed Err:%+v", err)
		t.Fatal(err)
	}

	fmt.Println("Now calling DeleteNetworkContainer")

	err = deleteNetworkAdapterWithName(t, "ethWebApp")
	if err != nil {
		t.Errorf("Deleting interface failed Err:%+v", err)
		t.Fatal(err)
	}

	err = getNonExistNetworkCotnainerByContext(t, "ethWebApp")
	if err != nil {
		t.Errorf("TestGetNetworkContainerByOrchestratorContext failed Err:%+v", err)
		t.Fatal(err)
	}
}

func TestGetNetworkContainerStatus(t *testing.T) {
	// requires more than 30 seconds to run
	fmt.Println("Test: TestCreateNetworkContainer")

	setEnv(t)

	err := creatOrUpdateNetworkContainerWithName(t, "ethWebApp", "11.0.0.5", "WebApps")
	if err != nil {
		t.Errorf("creatOrUpdateWebAppContainerWithName failed Err:%+v", err)
		t.Fatal(err)
	}

	fmt.Println("Now calling getNetworkCotnainerStatus")
	err = getNetworkCotnainerStatus(t, "ethWebApp")
	if err != nil {
		t.Errorf("getNetworkCotnainerStatus failed Err:%+v", err)
		t.Fatal(err)
	}

	fmt.Println("Now calling DeleteNetworkContainer")

	err = deleteNetworkAdapterWithName(t, "ethWebApp")
	if err != nil {
		t.Errorf("Deleting interface failed Err:%+v", err)
		t.Fatal(err)
	}
}

func TestGetInterfaceForNetworkContainer(t *testing.T) {
	// requires more than 30 seconds to run
	fmt.Println("Test: TestCreateNetworkContainer")

	setEnv(t)

	err := creatOrUpdateNetworkContainerWithName(t, "ethWebApp", "11.0.0.5", "WebApps")
	if err != nil {
		t.Errorf("creatOrUpdateWebAppContainerWithName failed Err:%+v", err)
		t.Fatal(err)
	}

	fmt.Println("Now calling getInterfaceForContainer")
	err = getInterfaceForContainer(t, "ethWebApp")
	if err != nil {
		t.Errorf("getInterfaceForContainer failed Err:%+v", err)
		t.Fatal(err)
	}

	fmt.Println("Now calling DeleteNetworkContainer")

	err = deleteNetworkAdapterWithName(t, "ethWebApp")
	if err != nil {
		t.Errorf("Deleting interface failed Err:%+v", err)
		t.Fatal(err)
	}
}
