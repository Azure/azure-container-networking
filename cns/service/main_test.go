package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/fakes"
	"github.com/Azure/azure-container-networking/cns/logger"
	mtv1alpha1 "github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
	"github.com/Azure/azure-container-networking/nmagent"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// mockIMDSClient is a mock implementation of the VMUniqueIDGetter interface
type mockIMDSClient struct {
	vmUniqueID string
	err        error
}

func (m *mockIMDSClient) GetVMUniqueID(_ context.Context) (string, error) {
	return m.vmUniqueID, m.err
}

// mockNMAgentClient is a mock implementation of the HomeAzGetter interface
type mockNMAgentClient struct {
	homeAzResponse nmagent.AzResponse
	err            error
}

func (m *mockNMAgentClient) GetHomeAz(_ context.Context) (nmagent.AzResponse, error) {
	return m.homeAzResponse, m.err
}

// mockNodeInfoClient is a mock implementation of the NodeInfoClient interface
type mockNodeInfoClient struct {
	createdNodeInfo *mtv1alpha1.NodeInfo
	err             error
}

func (m *mockNodeInfoClient) CreateOrUpdate(_ context.Context, nodeInfo *mtv1alpha1.NodeInfo, _ string) error {
	m.createdNodeInfo = nodeInfo
	return m.err
}

func TestBuildNodeInfoSpec_WithHomeAZ(t *testing.T) {
	tests := []struct {
		name            string
		vmUniqueID      string
		vmUniqueIDErr   error
		homeAzResponse  nmagent.AzResponse
		homeAzErr       error
		expectedSpec    mtv1alpha1.NodeInfoSpec
		expectedNodeErr bool
	}{
		{
			name:           "success with HomeAZ zone 1",
			vmUniqueID:     "test-vm-unique-id",
			vmUniqueIDErr:  nil,
			homeAzResponse: nmagent.AzResponse{HomeAz: 1},
			homeAzErr:      nil,
			expectedSpec: mtv1alpha1.NodeInfoSpec{
				VMUniqueID: "test-vm-unique-id",
				HomeAZ:     "AZ01",
			},
			expectedNodeErr: false,
		},
		{
			name:           "success with HomeAZ zone 2",
			vmUniqueID:     "another-vm-id",
			vmUniqueIDErr:  nil,
			homeAzResponse: nmagent.AzResponse{HomeAz: 2},
			homeAzErr:      nil,
			expectedSpec: mtv1alpha1.NodeInfoSpec{
				VMUniqueID: "another-vm-id",
				HomeAZ:     "AZ02",
			},
			expectedNodeErr: false,
		},
		{
			name:           "success with HomeAZ zone 10",
			vmUniqueID:     "vm-id-zone10",
			vmUniqueIDErr:  nil,
			homeAzResponse: nmagent.AzResponse{HomeAz: 10},
			homeAzErr:      nil,
			expectedSpec: mtv1alpha1.NodeInfoSpec{
				VMUniqueID: "vm-id-zone10",
				HomeAZ:     "AZ10",
			},
			expectedNodeErr: false,
		},
		{
			name:           "HomeAZ not available",
			vmUniqueID:     "test-vm-id",
			vmUniqueIDErr:  nil,
			homeAzResponse: nmagent.AzResponse{},
			homeAzErr:      errors.New("nmagent HomeAZ not available"),
			expectedSpec: mtv1alpha1.NodeInfoSpec{
				VMUniqueID: "test-vm-id",
				HomeAZ:     "", // HomeAZ should be empty when not available
			},
			expectedNodeErr: true,
		},
		{
			name:            "IMDS error", // should fail
			vmUniqueID:      "",
			vmUniqueIDErr:   errors.New("imds error"),
			homeAzResponse:  nmagent.AzResponse{HomeAz: 1},
			homeAzErr:       nil,
			expectedSpec:    mtv1alpha1.NodeInfoSpec{},
			expectedNodeErr: true,
		},
		{
			name:           "HomeAZ zone 0", // should be treated as not available
			vmUniqueID:     "test-vm-id",
			vmUniqueIDErr:  nil,
			homeAzResponse: nmagent.AzResponse{HomeAz: 0},
			homeAzErr:      nil,
			expectedSpec: mtv1alpha1.NodeInfoSpec{
				VMUniqueID: "test-vm-id",
				HomeAZ:     "",
			},
			expectedNodeErr: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			imdsCli := &mockIMDSClient{
				vmUniqueID: test.vmUniqueID,
				err:        test.vmUniqueIDErr,
			}
			nmaCli := &mockNMAgentClient{
				homeAzResponse: test.homeAzResponse,
				err:            test.homeAzErr,
			}
			nodeInfoCli := &mockNodeInfoClient{}
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
					UID:  "test-uid",
				},
			}

			err := buildAndCreateNodeInfo(context.Background(), imdsCli, nmaCli, nodeInfoCli, node)
			if err != nil && !test.expectedNodeErr {
				t.Fatal("unexpected error: err:", err)
			}
			if err == nil && test.expectedNodeErr {
				t.Fatal("expected error but received none")
			}

			if err != nil {
				// we should make no further assertions
				return
			}

			got := nodeInfoCli.createdNodeInfo.Spec
			exp := test.expectedSpec

			if exp.HomeAZ != got.HomeAZ {
				t.Error("received NodeInfo HomeAZ differs from expected: exp:", exp.HomeAZ, "got:", got.HomeAZ)
			}

			if exp.VMUniqueID != got.VMUniqueID {
				t.Error("received NodeInfo VMUniqueID differs from expected: exp:", exp.VMUniqueID, "got:", got.VMUniqueID)
			}

		})
	}
}
