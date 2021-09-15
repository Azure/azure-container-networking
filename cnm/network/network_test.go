// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package network

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"

	"github.com/Azure/azure-container-networking/cnm"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/netlink"
	driverApi "github.com/docker/libnetwork/driverapi"
	remoteApi "github.com/docker/libnetwork/drivers/remote/api"
)

var (
	plugin NetPlugin
	mux    *http.ServeMux
)

var (
	anyInterface = "dummy"
	anySubnet    = "192.168.1.0/24"
	ipnet        = net.IPNet{IP: net.ParseIP("192.168.1.0"), Mask: net.IPv4Mask(255, 255, 255, 0)}
	networkID    = "N1"
	netns        = "22212"
)

// endpoint ID must contain 7 characters
var endpointID = "E1-xxxx"

// Wraps the test run with plugin setup and teardown.
func TestMain(m *testing.M) {
	var config common.PluginConfig
	var err error

	// Create the plugin.
	plugin, err = NewPlugin(&config)
	if err != nil {
		fmt.Printf("Failed to create network plugin %v\n", err)
		os.Exit(1)
	}

	// Configure test mode.
	plugin.(*netPlugin).Name = "test"

	// Start the plugin.
	err = plugin.Start(&config)
	if err != nil {
		fmt.Printf("Failed to start network plugin %v\n", err)
		os.Exit(2)
	}
	nl := netlink.NewNetlink()

	// Create a dummy test network interface.
	err = nl.AddLink(&netlink.DummyLink{
		LinkInfo: netlink.LinkInfo{
			Type: netlink.LINK_TYPE_DUMMY,
			Name: anyInterface,
		},
	})

	if err != nil {
		fmt.Printf("Failed to create test network interface, err:%v.\n", err)
		os.Exit(3)
	}

	err = plugin.(*netPlugin).nm.AddExternalInterface(anyInterface, anySubnet)
	if err != nil {
		fmt.Printf("Failed to add test network interface, err:%v.\n", err)
		os.Exit(4)
	}

	err = nl.AddIPAddress(anyInterface, net.ParseIP("192.168.1.4"), &ipnet)
	if err != nil {
		fmt.Printf("Failed to add test IP address, err:%v.\n", err)
		os.Exit(5)
	}

	// Get the internal http mux as test hook.
	mux = plugin.(*netPlugin).Listener.GetMux()

	// Run tests.
	exitCode := m.Run()

	// Cleanup.
	err = nl.DeleteLink(anyInterface)
	if err != nil {
		fmt.Printf("Failed to delete link, err:%v.\n", err)
	}
	plugin.Stop()

	os.Exit(exitCode)
}

// Decodes plugin's responses to test requests.
func decodeResponse(w *httptest.ResponseRecorder, response interface{}) error {
	if w.Code != http.StatusOK {
		return fmt.Errorf("Request failed with HTTP error %d", w.Code)
	}

	if w.Body == nil {
		return fmt.Errorf("Response body is empty")
	}

	return json.NewDecoder(w.Body).Decode(&response)
}

//
// Libnetwork remote API compliance tests
// https://github.com/docker/libnetwork/blob/master/docs/remote.md
//

// Tests Plugin.Activate functionality.
func TestActivate(t *testing.T) {
	var resp cnm.ActivateResponse

	req, err := http.NewRequest(http.MethodGet, "/Plugin.Activate", nil)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	err = decodeResponse(w, &resp)

	if err != nil || resp.Err != "" || resp.Implements[0] != "NetworkDriver" {
		t.Errorf("Activate response is invalid %+v", resp)
	}
}

// Tests NetworkDriver.GetCapabilities functionality.
func TestGetCapabilities(t *testing.T) {
	var resp remoteApi.GetCapabilityResponse

	req, err := http.NewRequest(http.MethodGet, getCapabilitiesPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	err = decodeResponse(w, &resp)

	if err != nil || resp.Err != "" || resp.Scope != "local" {
		t.Errorf("GetCapabilities response is invalid %+v", resp)
	}
}

func TestCNM(t *testing.T) {
	cmd := exec.Command("ip", "netns", "add", netns)
	log.Printf("%v", cmd)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("%v:%v", output, err.Error())
		return
	}

	defer func() {
		cmd = exec.Command("ip", "netns", "delete", netns)
		_, err = cmd.Output()

		if err != nil {
			t.Fatalf("%v:%v", output, err)
			return
		}
	}()

	log.Printf("###CreateNetwork#####################################################################################")
	createNetworkT(t)
	log.Printf("###CreateEndpoint####################################################################################")
	createEndpointT(t)
	log.Printf("###EndpointOperInfo#####################################################################################")
	endpointOperInfoT(t)
	log.Printf("###DeleteEndpoint#####################################################################################")
	deleteEndpointT(t)
	log.Printf("###DeleteNetwork#####################################################################################")
	// deleteNetworkT(t)
}

// Tests NetworkDriver.CreateNetwork functionality.
func createNetworkT(t *testing.T) {
	var body bytes.Buffer
	var resp remoteApi.CreateNetworkResponse

	_, pool, _ := net.ParseCIDR(anySubnet)

	info := &remoteApi.CreateNetworkRequest{
		NetworkID: networkID,
		IPv4Data: []driverApi.IPAMData{
			{
				Pool: pool,
			},
		},
	}

	json.NewEncoder(&body).Encode(info)

	req, err := http.NewRequest(http.MethodGet, createNetworkPath, &body)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	err = decodeResponse(w, &resp)

	if err != nil || resp.Response.Err != "" {
		t.Errorf("CreateNetwork response is invalid %+v, received err %v", resp, err)
	}
}

// Tests NetworkDriver.CreateEndpoint functionality.
func createEndpointT(t *testing.T) {
	var body bytes.Buffer
	var resp remoteApi.CreateEndpointResponse

	info := &remoteApi.CreateEndpointRequest{
		NetworkID:  networkID,
		EndpointID: endpointID,
		Interface:  &remoteApi.EndpointInterface{Address: anySubnet},
	}

	json.NewEncoder(&body).Encode(info)

	req, err := http.NewRequest(http.MethodGet, createEndpointPath, &body)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	err = decodeResponse(w, &resp)

	if err != nil || resp.Response.Err != "" {
		t.Errorf("CreateEndpoint response is invalid %+v, received err %v", resp, err)
	}
}

// Tests NetworkDriver.EndpointOperInfo functionality.
func endpointOperInfoT(t *testing.T) {
	var body bytes.Buffer
	var resp remoteApi.EndpointInfoResponse

	info := &remoteApi.EndpointInfoRequest{
		NetworkID:  networkID,
		EndpointID: endpointID,
	}

	json.NewEncoder(&body).Encode(info)

	req, err := http.NewRequest(http.MethodGet, endpointOperInfoPath, &body)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	err = decodeResponse(w, &resp)
	if err != nil || resp.Err != "" {
		t.Errorf("EndpointOperInfo response is invalid %+v, received err %v", resp, err)
	}
}

func deleteEndpointT(t *testing.T) {
	var body bytes.Buffer
	var resp remoteApi.DeleteEndpointResponse

	info := &remoteApi.DeleteEndpointRequest{
		NetworkID:  networkID,
		EndpointID: endpointID,
	}

	json.NewEncoder(&body).Encode(info)

	req, err := http.NewRequest(http.MethodGet, deleteEndpointPath, &body)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	err = decodeResponse(w, &resp)

	if err != nil || resp.Response.Err != "" {
		t.Errorf("DeleteEndpoint response is invalid %+v, received err %v", resp, err)
	}
}

// Tests NetworkDriver.DeleteNetwork functionality.
func deleteNetworkT(t *testing.T) {
	var body bytes.Buffer
	var resp remoteApi.DeleteNetworkResponse

	info := &remoteApi.DeleteNetworkRequest{
		NetworkID: networkID,
	}

	json.NewEncoder(&body).Encode(info)

	req, err := http.NewRequest(http.MethodGet, deleteNetworkPath, &body)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	err = decodeResponse(w, &resp)

	if err != nil || resp.Err != "" {
		t.Errorf("DeleteNetwork response is invalid %+v, received err %v", resp, err)
	}
}
