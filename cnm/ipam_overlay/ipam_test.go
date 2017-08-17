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
	"os"
	"strconv"
	"testing"

	"github.com/Azure/azure-container-networking/cnm"
	cnmIpam "github.com/Azure/azure-container-networking/cnm/ipam"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/ipam"
)

var plugin IpamPlugin
var mux *http.ServeMux

var localAsId string
var poolId1 string
var address1 string

// Wraps the test run with plugin setup and teardown.
func TestMain(m *testing.M) {
	var config common.PluginConfig

	// Create the plugin.
	plugin, err := NewPlugin(&config)
	if err != nil {
		fmt.Printf("Failed to create IPAM plugin, err:%v.\n", err)
		return
	}

	// Configure test mode.
	plugin.SetOption(common.OptEnvironment, "null")
	plugin.SetOption(common.OptAPIServerURL, "null")

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

	os.Exit(exitCode)
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
	var resp cnmIpam.GetCapabilitiesResponse

	req, err := http.NewRequest(http.MethodGet, cnmIpam.GetCapabilitiesPath, nil)
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
	var resp cnmIpam.GetDefaultAddressSpacesResponse

	req, err := http.NewRequest(http.MethodGet, cnmIpam.GetAddressSpacesPath, nil)
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
	var resp cnmIpam.RequestPoolResponse

	payload := &cnmIpam.RequestPoolRequest{
		AddressSpace: localAsId,
		Pool:         "11.1.0.0/24",
	}

	json.NewEncoder(&body).Encode(payload)

	req, err := http.NewRequest(http.MethodGet, cnmIpam.RequestPoolPath, &body)
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
	var resp cnmIpam.RequestAddressResponse

	payload := &cnmIpam.RequestAddressRequest{
		PoolID:  poolId1,
		Address: "",
		Options: nil,
	}

	json.NewEncoder(&body).Encode(payload)

	req, err := http.NewRequest(http.MethodGet, cnmIpam.RequestAddressPath, &body)
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
	var resp cnmIpam.ReleaseAddressResponse

	payload := &cnmIpam.ReleaseAddressRequest{
		PoolID:  poolId1,
		Address: address1,
	}

	json.NewEncoder(&body).Encode(payload)

	req, err := http.NewRequest(http.MethodGet, cnmIpam.ReleaseAddressPath, &body)
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

// Tests IpamDriver.GetPoolInfo functionality.
func TestGetPoolInfo(t *testing.T) {
	var body bytes.Buffer
	var resp cnmIpam.GetPoolInfoResponse

	payload := &cnmIpam.GetPoolInfoRequest{
		PoolID: poolId1,
	}

	json.NewEncoder(&body).Encode(payload)

	req, err := http.NewRequest(http.MethodGet, cnmIpam.GetPoolInfoPath, &body)
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
func reqAddrInternal(payload *cnmIpam.RequestAddressRequest) (string, error) {
	var body bytes.Buffer
	var resp cnmIpam.RequestAddressResponse
	json.NewEncoder(&body).Encode(payload)

	req, err := http.NewRequest(http.MethodGet, cnmIpam.RequestAddressPath, &body)
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
func releaseAddrInternal(payload *cnmIpam.ReleaseAddressRequest) error {
	var body bytes.Buffer
	var resp cnmIpam.ReleaseAddressResponse

	json.NewEncoder(&body).Encode(payload)

	req, err := http.NewRequest(http.MethodGet, cnmIpam.ReleaseAddressPath, &body)
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
		payload := &cnmIpam.RequestAddressRequest{
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
		payload := &cnmIpam.ReleaseAddressRequest{
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
	reqPayload := &cnmIpam.RequestAddressRequest{
		PoolID:  poolId1,
		Address: "",
		Options: make(map[string]string),
	}
	reqPayload.Options[ipam.OptAddressID] = "id1"

	_, err := reqAddrInternal(reqPayload)
	if err != nil {
		t.Errorf("RequestAddress response is invalid %+v", err)
	}

	releasePayload := &cnmIpam.ReleaseAddressRequest{
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

// Tests IpamDriver.ReleasePool functionality.
func TestReleasePool(t *testing.T) {
	var body bytes.Buffer
	var resp cnmIpam.ReleasePoolResponse

	payload := &cnmIpam.ReleasePoolRequest{
		PoolID: poolId1,
	}

	json.NewEncoder(&body).Encode(payload)

	req, err := http.NewRequest(http.MethodGet, cnmIpam.ReleasePoolPath, &body)
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
