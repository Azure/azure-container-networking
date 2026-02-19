// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package restserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/common"
	"github.com/Azure/azure-container-networking/cns/fakes"
	"github.com/Azure/azure-container-networking/cns/types"
	acn "github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getTestServiceForEndpointTests creates a test service configured for stateless CNI endpoint tests
func getTestServiceForEndpointTests(t *testing.T) *HTTPRestService {
	t.Helper()
	var config common.ServiceConfig
	httpsvc, err := NewHTTPRestService(&config, &fakes.WireserverClientFake{}, &fakes.WireserverProxyFake{},
		&IPtablesProvider{}, &fakes.NMAgentClientFake{}, store.NewMockStore(""), nil, nil,
		fakes.NewMockIMDSClient())
	require.NoError(t, err, "NewHTTPRestService should not return an error")
	require.NotNil(t, httpsvc, "HTTPRestService should not be nil")

	// Enable endpoint state management (required for stateless CNI)
	httpsvc.Options = make(map[string]interface{})
	httpsvc.Options[acn.OptManageEndpointState] = true

	// Initialize endpoint state store
	httpsvc.EndpointStateStore = store.NewMockStore("")
	httpsvc.EndpointState = make(map[string]*EndpointInfo)

	return httpsvc
}

// persistEndpointState writes the in-memory EndpointState to the store.
// This is required because GetEndpointHelper reads from the store first.
func persistEndpointState(s *HTTPRestService) error {
	if err := s.EndpointStateStore.Write(EndpointStoreKey, s.EndpointState); err != nil {
		return fmt.Errorf("failed to persist endpoint state: %w", err)
	}
	return nil
}

// TestGetEndpointHelper tests the GetEndpointHelper function
func TestGetEndpointHelper(t *testing.T) {
	service := getTestServiceForEndpointTests(t)

	// Set up test endpoint state
	containerID := "test-container-12345678901234567890"
	testEndpoint := &EndpointInfo{
		PodName:      "test-pod",
		PodNamespace: "test-ns",
		IfnameToIPMap: map[string]*IPInfo{
			"eth0": {
				IPv4:          []net.IPNet{{IP: net.ParseIP("10.0.0.1").To4(), Mask: net.CIDRMask(24, 32)}},
				NICType:       cns.InfraNIC,
				HostVethName:  "veth-host",
				HnsEndpointID: "hns-ep-123",
			},
		},
	}
	service.EndpointState[containerID] = testEndpoint
	// Persist to store (required for GetEndpointHelper)
	require.NoError(t, persistEndpointState(service))

	tests := []struct {
		name        string
		endpointID  string
		wantErr     bool
		errContains string
	}{
		{
			name:       "Get existing endpoint - success",
			endpointID: containerID,
			wantErr:    false,
		},
		{
			name:        "Get non-existent endpoint - not found",
			endpointID:  "non-existent-container",
			wantErr:     true,
			errContains: ErrEndpointStateNotFound.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := service.GetEndpointHelper(tt.endpointID)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					require.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, testEndpoint.PodName, result.PodName)
				assert.Equal(t, testEndpoint.PodNamespace, result.PodNamespace)
			}
		})
	}
}

// TestGetEndpointHelper_LegacyFormat tests legacy endpoint ID format lookup
func TestGetEndpointHelper_LegacyFormat(t *testing.T) {
	service := getTestServiceForEndpointTests(t)

	// Set up endpoint with legacy format key (first 8 chars + "-eth0")
	fullContainerID := "abcdefgh12345678901234567890"
	legacyEndpointID := "abcdefgh-eth0"

	testEndpoint := &EndpointInfo{
		PodName:      "legacy-pod",
		PodNamespace: "legacy-ns",
		IfnameToIPMap: map[string]*IPInfo{
			"eth0": {
				IPv4:    []net.IPNet{{IP: net.ParseIP("10.0.0.5").To4(), Mask: net.CIDRMask(24, 32)}},
				NICType: cns.InfraNIC,
			},
		},
	}
	// Store with legacy key
	service.EndpointState[legacyEndpointID] = testEndpoint
	// Persist to store (required for GetEndpointHelper)
	require.NoError(t, persistEndpointState(service))

	// Query with full container ID should fall back to legacy lookup
	result, err := service.GetEndpointHelper(fullContainerID)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "legacy-pod", result.PodName)
}

// TestUpdateEndpointHelper tests the UpdateEndpointHelper function
func TestUpdateEndpointHelper(t *testing.T) {
	service := getTestServiceForEndpointTests(t)

	containerID := "update-test-container"

	// First, create an initial endpoint state
	initialEndpoint := &EndpointInfo{
		PodName:      "test-pod",
		PodNamespace: "test-ns",
		IfnameToIPMap: map[string]*IPInfo{
			"eth0": {
				IPv4:    []net.IPNet{{IP: net.ParseIP("10.0.0.1").To4(), Mask: net.CIDRMask(24, 32)}},
				NICType: cns.InfraNIC,
			},
		},
	}
	service.EndpointState[containerID] = initialEndpoint
	// Persist to store (required for UpdateEndpointHelper)
	require.NoError(t, persistEndpointState(service))

	tests := []struct {
		name       string
		endpointID string
		updateReq  map[string]*IPInfo
		wantErr    bool
		validate   func(*testing.T, *EndpointInfo)
	}{
		{
			name:       "Update existing endpoint with HNS ID",
			endpointID: containerID,
			updateReq: map[string]*IPInfo{
				"eth0": {
					HnsEndpointID: "new-hns-ep-id",
					HnsNetworkID:  "new-hns-net-id",
					NICType:       cns.InfraNIC,
				},
			},
			wantErr: false,
			validate: func(t *testing.T, ep *EndpointInfo) {
				require.NotNil(t, ep.IfnameToIPMap["eth0"])
				assert.Equal(t, "new-hns-ep-id", ep.IfnameToIPMap["eth0"].HnsEndpointID)
			},
		},
		{
			name:       "Create new endpoint if not exists (ACI scenario)",
			endpointID: "new-aci-container",
			updateReq: map[string]*IPInfo{
				"eth2": {
					NICType:            cns.DelegatedVMNIC,
					HnsEndpointID:      "aci-hns-ep",
					HnsNetworkID:       "aci-hns-net",
					MacAddress:         "aa:bb:cc:dd:ee:ff",
					NetworkContainerID: "aci-nc-id",
				},
			},
			wantErr: false,
			validate: func(t *testing.T, ep *EndpointInfo) {
				require.NotNil(t, ep.IfnameToIPMap["eth2"])
				assert.Equal(t, cns.DelegatedVMNIC, ep.IfnameToIPMap["eth2"].NICType)
			},
		},
		{
			name:       "Update endpoint with secondary NIC (SwiftV2)",
			endpointID: containerID,
			updateReq: map[string]*IPInfo{
				"eth1": {
					NICType:    cns.NodeNetworkInterfaceFrontendNIC,
					MacAddress: "11:22:33:44:55:66",
				},
			},
			wantErr: false,
			validate: func(t *testing.T, ep *EndpointInfo) {
				require.NotNil(t, ep.IfnameToIPMap["eth1"])
				assert.Equal(t, cns.NodeNetworkInterfaceFrontendNIC, ep.IfnameToIPMap["eth1"].NICType)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.UpdateEndpointHelper(tt.endpointID, tt.updateReq)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				// Verify the update
				ep, getErr := service.GetEndpointHelper(tt.endpointID)
				require.NoError(t, getErr)
				if tt.validate != nil {
					tt.validate(t, ep)
				}
			}
		})
	}
}

// TestDeleteEndpointStateHelper tests the DeleteEndpointStateHelper function
func TestDeleteEndpointStateHelper(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(*testing.T, *HTTPRestService)
		endpointID  string
		wantErr     bool
		errContains string
	}{
		{
			name: "Delete existing endpoint - success",
			setup: func(t *testing.T, s *HTTPRestService) {
				s.EndpointState["delete-test-container"] = &EndpointInfo{
					PodName: "delete-pod",
					IfnameToIPMap: map[string]*IPInfo{
						"eth0": {NICType: cns.InfraNIC},
					},
				}
				// Persist to store
				require.NoError(t, persistEndpointState(s))
			},
			endpointID: "delete-test-container",
			wantErr:    false,
		},
		{
			name:        "Delete non-existent endpoint - not found",
			setup:       func(_ *testing.T, _ *HTTPRestService) {},
			endpointID:  "non-existent",
			wantErr:     true,
			errContains: ErrEndpointStateNotFound.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := getTestServiceForEndpointTests(t)
			tt.setup(t, service)

			err := service.DeleteEndpointStateHelper(tt.endpointID)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					require.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				// Verify endpoint is deleted
				_, exists := service.EndpointState[tt.endpointID]
				assert.False(t, exists, "Endpoint should be deleted from state")
			}
		})
	}
}

// TestEndpointHandlerAPI_GetMethod tests GET requests to EndpointHandlerAPI
func TestEndpointHandlerAPI_GetMethod(t *testing.T) {
	service := getTestServiceForEndpointTests(t)

	containerID := "api-get-test-container"
	testEndpoint := &EndpointInfo{
		PodName:      "api-pod",
		PodNamespace: "api-ns",
		IfnameToIPMap: map[string]*IPInfo{
			"eth0": {
				IPv4:          []net.IPNet{{IP: net.ParseIP("10.0.0.10").To4(), Mask: net.CIDRMask(24, 32)}},
				NICType:       cns.InfraNIC,
				HnsEndpointID: "hns-api-ep",
			},
		},
	}
	service.EndpointState[containerID] = testEndpoint
	// Persist to store (required for GET operations)
	require.NoError(t, persistEndpointState(service))

	tests := []struct {
		name           string
		path           string
		wantStatusCode types.ResponseCode
	}{
		{
			name:           "GET existing endpoint - success",
			path:           cns.EndpointPath + containerID,
			wantStatusCode: types.Success,
		},
		{
			name:           "GET non-existent endpoint - not found",
			path:           cns.EndpointPath + "non-existent",
			wantStatusCode: types.NotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, http.NoBody)
			w := httptest.NewRecorder()

			service.EndpointHandlerAPI(w, req)

			resp := w.Result()
			defer resp.Body.Close()

			var response GetEndpointResponse
			err := json.NewDecoder(resp.Body).Decode(&response)
			require.NoError(t, err)

			assert.Equal(t, tt.wantStatusCode, response.Response.ReturnCode)
		})
	}
}

// TestEndpointHandlerAPI_PatchMethod tests PATCH requests to EndpointHandlerAPI
func TestEndpointHandlerAPI_PatchMethod(t *testing.T) {
	service := getTestServiceForEndpointTests(t)

	containerID := "api-patch-test-container"
	// Pre-populate endpoint
	service.EndpointState[containerID] = &EndpointInfo{
		PodName:       "patch-pod",
		PodNamespace:  "patch-ns",
		IfnameToIPMap: map[string]*IPInfo{"eth0": {NICType: cns.InfraNIC}},
	}
	// Persist to store
	require.NoError(t, persistEndpointState(service))

	updateReq := map[string]*IPInfo{
		"eth0": {
			HnsEndpointID: "updated-hns-id",
			NICType:       cns.InfraNIC,
		},
	}
	reqBody, err := json.Marshal(updateReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, cns.EndpointPath+containerID, bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	service.EndpointHandlerAPI(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	var response cns.Response
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, types.Success, response.ReturnCode)

	// Verify the update persisted
	ep, getErr := service.GetEndpointHelper(containerID)
	require.NoError(t, getErr)
	assert.Equal(t, "updated-hns-id", ep.IfnameToIPMap["eth0"].HnsEndpointID)
}

// TestEndpointHandlerAPI_DeleteMethod tests DELETE requests to EndpointHandlerAPI
func TestEndpointHandlerAPI_DeleteMethod(t *testing.T) {
	service := getTestServiceForEndpointTests(t)

	containerID := "api-delete-test-container"
	// Pre-populate endpoint
	service.EndpointState[containerID] = &EndpointInfo{
		PodName:       "delete-pod",
		PodNamespace:  "delete-ns",
		IfnameToIPMap: map[string]*IPInfo{"eth0": {NICType: cns.InfraNIC}},
	}
	// Persist to store
	require.NoError(t, persistEndpointState(service))

	tests := []struct {
		name           string
		path           string
		wantStatusCode types.ResponseCode
	}{
		{
			name:           "DELETE existing endpoint - success",
			path:           cns.EndpointPath + containerID,
			wantStatusCode: types.Success,
		},
		{
			name:           "DELETE non-existent endpoint - not found",
			path:           cns.EndpointPath + "non-existent-delete",
			wantStatusCode: types.NotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodDelete, tt.path, http.NoBody)
			w := httptest.NewRecorder()

			service.EndpointHandlerAPI(w, req)

			resp := w.Result()
			defer resp.Body.Close()

			var response cns.Response
			err := json.NewDecoder(resp.Body).Decode(&response)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatusCode, response.ReturnCode)
		})
	}

	// Verify endpoint is deleted
	_, exists := service.EndpointState[containerID]
	assert.False(t, exists)
}

// TestEndpointHandlerAPI_OptManageEndpointStateDisabled tests that API returns error when OptManageEndpointState is false
func TestEndpointHandlerAPI_OptManageEndpointStateDisabled(t *testing.T) {
	service := getTestServiceForEndpointTests(t)
	// Disable endpoint state management
	service.Options[acn.OptManageEndpointState] = false

	req := httptest.NewRequest(http.MethodGet, cns.EndpointPath+"test-container", http.NoBody)
	w := httptest.NewRecorder()

	service.EndpointHandlerAPI(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), ErrOptManageEndpointState.Error())
}

// TestStatelessCNI_EndToEnd_Flow tests the full stateless CNI flow:
// 1. CNS creates endpoint state during IP allocation
// 2. CNI calls UpdateEndpoint to add HNS/Veth info
// 3. CNI calls GetEndpoint during DELETE to retrieve state
// 4. CNS deletes endpoint state
func TestStatelessCNI_EndToEnd_Flow(t *testing.T) {
	service := getTestServiceForEndpointTests(t)
	containerID := "e2e-stateless-container-12345678"

	// Step 1: Simulate CNS creating initial endpoint state during IP allocation
	// (In real flow, this happens in requestIPConfigsHandler)
	initialState := &EndpointInfo{
		PodName:      "e2e-pod",
		PodNamespace: "e2e-ns",
		IfnameToIPMap: map[string]*IPInfo{
			"eth0": {
				IPv4:    []net.IPNet{{IP: net.ParseIP("10.0.0.100").To4(), Mask: net.CIDRMask(24, 32)}},
				NICType: cns.InfraNIC,
			},
		},
	}
	service.EndpointState[containerID] = initialState
	// Persist to store
	require.NoError(t, persistEndpointState(service))

	// Step 2: CNI calls UpdateEndpoint to add HNS/Veth info after endpoint creation
	updateReq := map[string]*IPInfo{
		"eth0": {
			HnsEndpointID: "e2e-hns-endpoint-id",
			HnsNetworkID:  "e2e-hns-network-id",
			HostVethName:  "e2e-veth-host",
			NICType:       cns.InfraNIC,
		},
	}
	err := service.UpdateEndpointHelper(containerID, updateReq)
	require.NoError(t, err)

	// Verify update
	ep, err := service.GetEndpointHelper(containerID)
	require.NoError(t, err)
	assert.Equal(t, "e2e-hns-endpoint-id", ep.IfnameToIPMap["eth0"].HnsEndpointID)
	assert.Equal(t, "e2e-veth-host", ep.IfnameToIPMap["eth0"].HostVethName)
	// Original IP should still be there
	assert.Len(t, ep.IfnameToIPMap["eth0"].IPv4, 1)

	// Step 3: CNI calls GetEndpoint during DELETE
	retrievedEp, err := service.GetEndpointHelper(containerID)
	require.NoError(t, err)
	assert.Equal(t, "e2e-pod", retrievedEp.PodName)
	assert.Equal(t, "e2e-hns-endpoint-id", retrievedEp.IfnameToIPMap["eth0"].HnsEndpointID)

	// Step 4: CNS deletes endpoint state (for SwiftV2 standalone or after IP release)
	err = service.DeleteEndpointStateHelper(containerID)
	require.NoError(t, err)

	// Verify deletion
	_, exists := service.EndpointState[containerID]
	assert.False(t, exists)
}

// TestStatelessCNI_SwiftV2_MultiNIC tests SwiftV2 multi-NIC scenario
func TestStatelessCNI_SwiftV2_MultiNIC(t *testing.T) {
	service := getTestServiceForEndpointTests(t)
	containerID := "swiftv2-multi-nic-container"

	// Set up initial state with InfraNIC
	service.EndpointState[containerID] = &EndpointInfo{
		PodName:      "swiftv2-pod",
		PodNamespace: "swiftv2-ns",
		IfnameToIPMap: map[string]*IPInfo{
			"eth0": {
				IPv4:    []net.IPNet{{IP: net.ParseIP("10.0.0.50").To4(), Mask: net.CIDRMask(24, 32)}},
				NICType: cns.InfraNIC,
			},
		},
	}
	// Persist to store
	require.NoError(t, persistEndpointState(service))

	// Update with FrontendNIC (SwiftV2 secondary NIC)
	updateReq := map[string]*IPInfo{
		"eth0": {
			HnsEndpointID: "infra-hns-id",
			HostVethName:  "infra-veth",
			NICType:       cns.InfraNIC,
		},
		"eth1": {
			IPv4:       []net.IPNet{{IP: net.ParseIP("20.20.20.20"), Mask: net.CIDRMask(32, 32)}},
			NICType:    cns.NodeNetworkInterfaceFrontendNIC,
			MacAddress: "aa:bb:cc:dd:ee:ff",
		},
	}
	err := service.UpdateEndpointHelper(containerID, updateReq)
	require.NoError(t, err)

	// Verify both NICs are in state
	ep, err := service.GetEndpointHelper(containerID)
	require.NoError(t, err)

	// Verify InfraNIC
	require.NotNil(t, ep.IfnameToIPMap["eth0"])
	assert.Equal(t, cns.InfraNIC, ep.IfnameToIPMap["eth0"].NICType)
	assert.Equal(t, "infra-hns-id", ep.IfnameToIPMap["eth0"].HnsEndpointID)

	// Verify FrontendNIC
	require.NotNil(t, ep.IfnameToIPMap["eth1"])
	assert.Equal(t, cns.NodeNetworkInterfaceFrontendNIC, ep.IfnameToIPMap["eth1"].NICType)
	assert.Equal(t, "aa:bb:cc:dd:ee:ff", ep.IfnameToIPMap["eth1"].MacAddress)
}
