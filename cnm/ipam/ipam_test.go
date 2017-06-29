// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package ipam

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"testing"

	"github.com/Azure/azure-container-networking/cnm"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/ipam"
)

var plugin IpamPlugin
var mux *http.ServeMux

var ipamQueryUrl = "localhost:42424"
var ipamQueryResponse = "" +
	"<Interfaces>" +
	"	<Interface MacAddress=\"*\" IsPrimary=\"true\">" +
	"		<IPSubnet Prefix=\"10.0.0.0/16\">" +
	"			<IPAddress Address=\"10.0.0.4\" IsPrimary=\"true\"/>" +
	"			<IPAddress Address=\"10.0.0.5\" IsPrimary=\"false\"/>" +
	"			<IPAddress Address=\"10.0.0.6\" IsPrimary=\"false\"/>" +
	"			<IPAddress Address=\"10.0.0.7\" IsPrimary=\"false\"/>" +
	"			<IPAddress Address=\"10.0.0.8\" IsPrimary=\"false\"/>" +
	"			<IPAddress Address=\"10.0.0.9\" IsPrimary=\"false\"/>" +
	"		</IPSubnet>" +
	"	</Interface>" +
	"</Interfaces>"

var localAsId string
var poolId1 string
var address1 string

// Wraps the test run with plugin setup and teardown.
func TestMain(m *testing.M) {
	var config common.PluginConfig

	// Create a fake local agent to handle requests from IPAM plugin.
	u, _ := url.Parse("tcp://" + ipamQueryUrl)
	testAgent, err := common.NewListener(u)
	if err != nil {
		fmt.Printf("Failed to create agent, err:%v.\n", err)
		return
	}
	testAgent.AddHandler("/", handleIpamQuery)

	err = testAgent.Start(make(chan error, 1))
	if err != nil {
		fmt.Printf("Failed to start agent, err:%v.\n", err)
		return
	}

	// Create the plugin.
	plugin, err = NewPlugin(&config)
	if err != nil {
		fmt.Printf("Failed to create IPAM plugin, err:%v.\n", err)
		return
	}

	// Configure test mode.
	plugin.SetOption(common.OptEnvironment, common.OptEnvironmentAzure)
	plugin.SetOption(common.OptAPIServerURL, "null")
	plugin.SetOption(common.OptIpamQueryUrl, "http://"+ipamQueryUrl)

	// Start the plugin.
	err = plugin.Start(&config)
	if err != nil {
		fmt.Printf("Failed to start IPAM plugin, err:%v.\n", err)
		return
	}

	// Get the internal http mux as test hook.
	mux = plugin.(*ipamPlugin).Listener.GetMux()

	// Run tests.
	exitCode := m.Run()

	// Cleanup.
	plugin.Stop()
	testAgent.Stop()

	os.Exit(exitCode)
}

// Handles queries from IPAM source.
func handleIpamQuery(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(ipamQueryResponse))
}

// Decodes plugin's responses to test requests.
func decodeResponse(w *httptest.ResponseRecorder, response interface{}) error {
	if w.Code != http.StatusOK {
		return fmt.Errorf("Request failed with HTTP error %s", w.Code)
	}

	if w.Body == nil {
		return fmt.Errorf("Response body is empty")
	}

	return json.NewDecoder(w.Body).Decode(&response)
}

//
// Libnetwork remote IPAM API compliance tests
// https://github.com/docker/libnetwork/blob/master/docs/ipam.md
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

	if err != nil || resp.Err != "" || resp.Implements[0] != "IpamDriver" {
		t.Errorf("Activate response is invalid %+v", resp)
	}
}

// Tests IpamDriver.GetCapabilities functionality.
func TestGetCapabilities(t *testing.T) {
	var resp GetCapabilitiesResponse

	req, err := http.NewRequest(http.MethodGet, GetCapabilitiesPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	err = decodeResponse(w, &resp)

	if err != nil || resp.Err != "" {
		t.Errorf("GetCapabilities response is invalid %+v", resp)
	}
}

// Tests IpamDriver.GetDefaultAddressSpaces functionality.
func TestGetDefaultAddressSpaces(t *testing.T) {
	var resp GetDefaultAddressSpacesResponse

	req, err := http.NewRequest(http.MethodGet, GetAddressSpacesPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	err = decodeResponse(w, &resp)

	if err != nil || resp.Err != "" || resp.LocalDefaultAddressSpace == "" {
		t.Errorf("GetDefaultAddressSpaces response is invalid %+v", resp)
	}

	localAsId = resp.LocalDefaultAddressSpace
}

// Tests IpamDriver.RequestPool functionality.
func TestRequestPool(t *testing.T) {
	var body bytes.Buffer
	var resp RequestPoolResponse

	payload := &RequestPoolRequest{
		AddressSpace: localAsId,
	}

	json.NewEncoder(&body).Encode(payload)

	req, err := http.NewRequest(http.MethodGet, RequestPoolPath, &body)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	err = decodeResponse(w, &resp)

	if err != nil || resp.Err != "" {
		t.Errorf("RequestPool response is invalid %+v", resp)
	}

	poolId1 = resp.PoolID
}

// Tests IpamDriver.RequestAddress functionality.
func TestRequestAddress(t *testing.T) {
	var body bytes.Buffer
	var resp RequestAddressResponse

	payload := &RequestAddressRequest{
		PoolID:  poolId1,
		Address: "",
		Options: nil,
	}

	json.NewEncoder(&body).Encode(payload)

	req, err := http.NewRequest(http.MethodGet, RequestAddressPath, &body)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	err = decodeResponse(w, &resp)

	if err != nil || resp.Err != "" {
		t.Errorf("RequestAddress response is invalid %+v", resp)
	}

	address, _, _ := net.ParseCIDR(resp.Address)
	address1 = address.String()
}

// Tests IpamDriver.ReleaseAddress functionality.
func TestReleaseAddress(t *testing.T) {
	var body bytes.Buffer
	var resp ReleaseAddressResponse

	payload := &ReleaseAddressRequest{
		PoolID:  poolId1,
		Address: address1,
	}

	json.NewEncoder(&body).Encode(payload)

	req, err := http.NewRequest(http.MethodGet, ReleaseAddressPath, &body)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	err = decodeResponse(w, &resp)

	if err != nil || resp.Err != "" {
		t.Errorf("ReleaseAddress response is invalid %+v", resp)
	}
}

// Tests IpamDriver.ReleasePool functionality.
func TestReleasePool(t *testing.T) {
	var body bytes.Buffer
	var resp ReleasePoolResponse

	payload := &ReleasePoolRequest{
		PoolID: poolId1,
	}

	json.NewEncoder(&body).Encode(payload)

	req, err := http.NewRequest(http.MethodGet, ReleasePoolPath, &body)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	err = decodeResponse(w, &resp)

	if err != nil || resp.Err != "" {
		t.Errorf("ReleasePool response is invalid %+v", resp)
	}
}

// Tests IpamDriver.GetPoolInfo functionality.
func TestGetPoolInfo(t *testing.T) {
	var body bytes.Buffer
	var resp GetPoolInfoResponse

	payload := &GetPoolInfoRequest{
		PoolID: poolId1,
	}

	json.NewEncoder(&body).Encode(payload)

	req, err := http.NewRequest(http.MethodGet, GetPoolInfoPath, &body)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	err = decodeResponse(w, &resp)

	if err != nil || resp.Err != "" {
		t.Errorf("GetPoolInfo response is invalid %+v", resp)
	}
}

// Utility function to request address from IPAM.
func reqAddrInternal(payload *RequestAddressRequest) (string, error) {
	var body bytes.Buffer
	var resp RequestAddressResponse
	json.NewEncoder(&body).Encode(payload)

	req, err := http.NewRequest(http.MethodGet, RequestAddressPath, &body)
	if err != nil {
		return "", err
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	err = decodeResponse(w, &resp)

	if err != nil {
		return "", err
	}
	return resp.Address, nil
}

// Utility function to release address from IPAM.
func releaseAddrInternal(payload *ReleaseAddressRequest) error {
	var body bytes.Buffer
	var resp ReleaseAddressResponse

	json.NewEncoder(&body).Encode(payload)

	req, err := http.NewRequest(http.MethodGet, ReleaseAddressPath, &body)
	if err != nil {
		return err
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	err = decodeResponse(w, &resp)

	if err != nil {
		return err
	}
	return nil
}

// Tests IpamDriver.RequestAddress with id.
func TestRequestAddressWithID(t *testing.T) {
	var ipList [2]string

	for i := 0; i < 2; i++ {
		payload := &RequestAddressRequest{
			PoolID:  poolId1,
			Address: "",
			Options: make(map[string]string),
		}

		payload.Options[ipam.OptAddressID] = "id" + strconv.Itoa(i)

		addr1, err := reqAddrInternal(payload)
		if err != nil {
			t.Errorf("RequestAddress response is invalid %+v", err)
		}

		addr2, err := reqAddrInternal(payload)
		if err != nil {
			t.Errorf("RequestAddress response is invalid %+v", err)
		}

		if addr1 != addr2 {
			t.Errorf("RequestAddress with id %+v doesn't match with retrieved addr %+v ", addr1, addr2)
		}

		address, _, _ := net.ParseCIDR(addr1)
		ipList[i] = address.String()
	}

	for i := 0; i < 2; i++ {
		payload := &ReleaseAddressRequest{
			PoolID:  poolId1,
			Address: ipList[i],
		}
		err := releaseAddrInternal(payload)
		if err != nil {
			t.Errorf("ReleaseAddress response is invalid %+v", err)
		}
	}
}

// Tests IpamDriver.ReleaseAddress with id.
func TestReleaseAddressWithID(t *testing.T) {
	reqPayload := &RequestAddressRequest{
		PoolID:  poolId1,
		Address: "",
		Options: make(map[string]string),
	}
	reqPayload.Options[ipam.OptAddressID] = "id1"

	_, err := reqAddrInternal(reqPayload)
	if err != nil {
		t.Errorf("RequestAddress response is invalid %+v", err)
	}

	releasePayload := &ReleaseAddressRequest{
		PoolID:  poolId1,
		Address: "",
		Options: make(map[string]string),
	}
	releasePayload.Options[ipam.OptAddressID] = "id1"

	err = releaseAddrInternal(releasePayload)

	if err != nil {
		t.Errorf("ReleaseAddress response is invalid %+v", err)
	}
}
