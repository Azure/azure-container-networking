package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/fakes"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

// MockHTTPClient is a mock implementation of HTTPClient
type MockHTTPClient struct {
	Response *http.Response
	Err      error
}

// Post is the implementation of the Post method for MockHTTPClient
func (m *MockHTTPClient) Do(_ *http.Request) (*http.Response, error) {
	return m.Response, m.Err
}

func TestSendRegisterNodeRequest_StatusOK(t *testing.T) {
	ctx := context.Background()
	logger.InitLogger("testlogs", 0, 0, "./")
	httpServiceFake := fakes.NewHTTPServiceFake()
	nodeRegisterReq := cns.NodeRegisterRequest{
		NumCores:             2,
		NmAgentSupportedApis: nil,
	}

	url := "https://localhost:9000/api"

	// Create a mock HTTP client
	mockResponse := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString(`{"status": "success", "OrchestratorType": "Kubernetes", "DncPartitionKey": "1234", "NodeID": "5678"}`)),
		Header:     make(http.Header),
	}

	mockClient := &MockHTTPClient{Response: mockResponse, Err: nil}

	assert.NoError(t, sendRegisterNodeRequest(ctx, mockClient, httpServiceFake, nodeRegisterReq, url))
}

func TestSendRegisterNodeRequest_StatusAccepted(t *testing.T) {
	ctx := context.Background()
	logger.InitLogger("testlogs", 0, 0, "./")
	httpServiceFake := fakes.NewHTTPServiceFake()
	nodeRegisterReq := cns.NodeRegisterRequest{
		NumCores:             2,
		NmAgentSupportedApis: nil,
	}

	url := "https://localhost:9000/api"

	// Create a mock HTTP client
	mockResponse := &http.Response{
		StatusCode: http.StatusAccepted,
		Body:       io.NopCloser(bytes.NewBufferString(`{"status": "accepted", "OrchestratorType": "Kubernetes", "DncPartitionKey": "1234", "NodeID": "5678"}`)),
		Header:     make(http.Header),
	}

	mockClient := &MockHTTPClient{Response: mockResponse, Err: nil}

	assert.Error(t, sendRegisterNodeRequest(ctx, mockClient, httpServiceFake, nodeRegisterReq, url))
}

func TestCreateOrUpdateNodeInfoCRD_PopulatesHomeAZ(t *testing.T) {
	vmID := "test-vm-unique-id-12345"
	homeAZ := uint(2)
	HomeAZStr := fmt.Sprintf("AZ0%d", homeAZ)

	// Create mock IMDS server
	mockIMDSServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/metadata/instance/compute") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			response := map[string]interface{}{
				"vmId":              vmID,
				"name":              "test-vm",
				"resourceGroupName": "test-rg",
			}
			_ = json.NewEncoder(w).Encode(response)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockIMDSServer.Close()

	// Create mock CNS server
	mockCNSServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/homeaz") || strings.Contains(r.URL.Path, "homeaz") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			response := map[string]interface{}{
				"ReturnCode": 0,
				"Message":    "",
				"HomeAzResponse": map[string]interface{}{
					"IsSupported": true,
					"HomeAz":      homeAZ,
				},
			}
			_ = json.NewEncoder(w).Encode(response)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockCNSServer.Close()

	// Set up HTTP transport to mock IMDS and CNS
	originalTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = originalTransport }()

	http.DefaultTransport = &mockTransport{
		imdsServer: mockIMDSServer,
		cnsServer:  mockCNSServer,
		original:   originalTransport,
	}

	// Create a mock Kubernetes server that captures the NodeInfo being created
	var capturedNodeInfo *v1alpha1.NodeInfo

	mockK8sServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle specific API group discovery - multitenancy.acn.azure.com
		if r.URL.Path == "/apis/multitenancy.acn.azure.com/v1alpha1" && r.Method == "GET" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"kind":         "APIResourceList",
				"groupVersion": "multitenancy.acn.azure.com/v1alpha1",
				"resources": []map[string]interface{}{
					{
						"name":         "nodeinfos",
						"singularName": "nodeinfo",
						"namespaced":   false,
						"kind":         "NodeInfo",
						"verbs":        []string{"create", "delete", "get", "list", "patch", "update", "watch"},
					},
				},
			})
			return
		}

		// Handle NodeInfo resource requests
		if strings.Contains(r.URL.Path, "nodeinfos") || strings.Contains(r.URL.Path, "multitenancy") {
			if r.Method == "POST" || r.Method == "PATCH" || r.Method == "PUT" {
				body, _ := io.ReadAll(r.Body)

				// Try to parse the NodeInfo from the request
				var nodeInfo v1alpha1.NodeInfo
				if err := json.Unmarshal(body, &nodeInfo); err == nil {
					capturedNodeInfo = &nodeInfo
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				// Return the created NodeInfo
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"apiVersion": "multitenancy.acn.azure.com/v1alpha1",
					"kind":       "NodeInfo",
					"metadata": map[string]interface{}{
						"name": "test-node",
					},
					"spec": map[string]interface{}{
						"vmUniqueID": vmID,
						"homeAZ":     HomeAZStr,
					},
				})
				return
			}
		}

		// Default success response for any other API calls
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"kind":   "Status",
			"status": "Success",
		})
	}))
	defer mockK8sServer.Close()

	// Test the function with mocked dependencies
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Point to our mock Kubernetes server
	restConfig := &rest.Config{
		Host: mockK8sServer.URL,
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
	}

	// Call the createOrUpdateNodeInfoCRD function
	err := createOrUpdateNodeInfoCRD(ctx, restConfig, node)

	// Verify the function succeeded
	require.NoError(t, err, "Function should succeed with mocked dependencies")

	// Verify the captured values
	assert.NotNil(t, capturedNodeInfo, "NodeInfo should have been captured from K8s API call")
	if capturedNodeInfo != nil {
		assert.Equal(t, vmID, capturedNodeInfo.Spec.VMUniqueID, "VMUniqueID should be from IMDS")
		assert.Equal(t, HomeAZStr, capturedNodeInfo.Spec.HomeAZ, "HomeAZ should be formatted from CNS response")
	}
}

// mockTransport redirects HTTP requests to mock servers for testing.
// It intercepts requests to IMDS and CNS endpoints and routes them to local test servers.
type mockTransport struct {
	imdsServer *httptest.Server
	cnsServer  *httptest.Server
	original   http.RoundTripper
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Redirect IMDS calls to mock IMDS server
	if req.URL.Host == "169.254.169.254" {
		req.URL.Scheme = "http"
		req.URL.Host = strings.TrimPrefix(m.imdsServer.URL, "http://")
		resp, err := m.original.RoundTrip(req)
		if err != nil {
			return nil, fmt.Errorf("IMDS mock transport failed: %w", err)
		}
		return resp, nil
	}

	// Redirect CNS calls to mock CNS server
	if req.URL.Host == "localhost:10090" || strings.Contains(req.URL.Host, "10090") {
		req.URL.Scheme = "http"
		req.URL.Host = strings.TrimPrefix(m.cnsServer.URL, "http://")
		resp, err := m.original.RoundTrip(req)
		if err != nil {
			return nil, fmt.Errorf("CNS mock transport failed: %w", err)
		}
		return resp, nil
	}

	// All other calls go through original transport
	resp, err := m.original.RoundTrip(req)
	if err != nil {
		return nil, fmt.Errorf("mock transport failed: %w", err)
	}
	return resp, nil
}
