package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/fakes"
	"github.com/Azure/azure-container-networking/cns/logger"
	mtv1alpha1 "github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
	"github.com/Azure/azure-container-networking/nmagent"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			name:            "NMAgent unauthorized",
			vmUniqueID:      "test-vm-id",
			homeAzErr:       errors.Wrap(nmagent.Error{Code: http.StatusUnauthorized}, "request failed"),
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
				assert.Nil(t, nodeInfoCli.createdNodeInfo, "must not publish incomplete NodeInfo")
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

// TestBuildAndCreateNodeInfo_MultiTenantCRD verifies that buildAndCreateNodeInfo
// correctly creates a NodeInfo CRD in the MultiTenantCRD channel mode scenario.
func TestBuildAndCreateNodeInfo_MultiTenantCRD(t *testing.T) {
	tests := []struct {
		name        string
		vmUniqueID  string
		homeAz      uint
		expectedAZ  string
		expectError bool
	}{
		{
			name:        "multi-tenant CRD with valid HomeAZ",
			vmUniqueID:  "mt-vm-unique-id",
			homeAz:      3,
			expectedAZ:  "AZ03",
			expectError: false,
		},
		{
			name:        "multi-tenant CRD with no HomeAZ",
			vmUniqueID:  "mt-vm-no-az",
			homeAz:      0,
			expectedAZ:  "",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			imdsCli := &mockIMDSClient{
				vmUniqueID: tt.vmUniqueID,
			}
			nmaCli := &mockNMAgentClient{
				homeAzResponse: nmagent.AzResponse{HomeAz: tt.homeAz},
			}
			nodeInfoCli := &mockNodeInfoClient{}
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "mt-test-node",
					UID:  "mt-test-uid",
				},
			}

			err := buildAndCreateNodeInfo(context.Background(), imdsCli, nmaCli, nodeInfoCli, node)
			if tt.expectError {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, nodeInfoCli.createdNodeInfo)
			assert.Equal(t, tt.vmUniqueID, nodeInfoCli.createdNodeInfo.Spec.VMUniqueID)
			assert.Equal(t, tt.expectedAZ, nodeInfoCli.createdNodeInfo.Spec.HomeAZ)
			assert.Equal(t, "mt-test-node", nodeInfoCli.createdNodeInfo.Name)
		})
	}
}

func TestReconcileNodeInfo(t *testing.T) {
	logger.InitLogger("testlogs", 0, 0, t.TempDir()+"/")
	t.Cleanup(logger.Close)
	tests := []struct {
		name string
		fail func(*mockIMDSClient, *mockNMAgentClient, *mockNodeInfoClient)
	}{
		{
			name: "NMAgent unauthorized",
			fail: func(_ *mockIMDSClient, nma *mockNMAgentClient, _ *mockNodeInfoClient) {
				nma.err = errors.Wrap(nmagent.Error{Code: http.StatusUnauthorized}, "request failed")
			},
		},
		{
			name: "IMDS unavailable",
			fail: func(imds *mockIMDSClient, _ *mockNMAgentClient, _ *mockNodeInfoClient) {
				imds.err = errors.New("imds unavailable")
			},
		},
		{
			name: "NodeInfo API unavailable",
			fail: func(_ *mockIMDSClient, _ *mockNMAgentClient, nodeInfo *mockNodeInfoClient) {
				nodeInfo.err = errors.New("api unavailable")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			imds := &mockIMDSClient{vmUniqueID: "test-vm-id"}
			nma := &mockNMAgentClient{homeAzResponse: nmagent.AzResponse{HomeAz: 2}}
			nodeInfo := &mockNodeInfoClient{}
			node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "test-node", UID: "test-uid"}}
			attempts := 0
			err := reconcileNodeInfo(ctx, time.Millisecond, func(attemptCtx context.Context) error {
				attempts++
				require.NotEqual(t, ctx, attemptCtx)
				deadline, ok := attemptCtx.Deadline()
				require.True(t, ok, "each attempt must be bounded")
				require.LessOrEqual(t, time.Until(deadline), cnsReqTimeout)
				if attempts == 1 {
					tt.fail(imds, nma, nodeInfo)
				} else {
					imds.err, nma.err, nodeInfo.err = nil, nil, nil
				}
				return buildAndCreateNodeInfo(attemptCtx, imds, nma, nodeInfo, node)
			})
			require.NoError(t, err)
			require.Equal(t, 2, attempts)
			require.NotNil(t, nodeInfo.createdNodeInfo)
			assert.Equal(t, "AZ02", nodeInfo.createdNodeInfo.Spec.HomeAZ)
			assert.Equal(t, "test-vm-id", nodeInfo.createdNodeInfo.Spec.VMUniqueID)
			require.Len(t, nodeInfo.createdNodeInfo.OwnerReferences, 1)
			assert.Equal(t, node.UID, nodeInfo.createdNodeInfo.OwnerReferences[0].UID)
		})
	}
}

func TestReconcileNodeInfoPersistentUnauthorized(t *testing.T) {
	logger.InitLogger("testlogs", 0, 0, t.TempDir()+"/")
	t.Cleanup(logger.Close)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	imds := &mockIMDSClient{vmUniqueID: "test-vm-id"}
	nma := &mockNMAgentClient{err: nmagent.Error{Code: http.StatusUnauthorized}}
	nodeInfo := &mockNodeInfoClient{}
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "test-node", UID: "test-uid"}}
	attempts := 0
	err := reconcileNodeInfo(ctx, time.Millisecond, func(attemptCtx context.Context) error {
		attempts++
		if attempts == 3 {
			cancel()
		}
		return buildAndCreateNodeInfo(attemptCtx, imds, nma, nodeInfo, node)
	})
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 3, attempts)
	assert.Nil(t, nodeInfo.createdNodeInfo)
}

func TestReconcileNodeInfoCancellation(t *testing.T) {
	logger.InitLogger("testlogs", 0, 0, t.TempDir()+"/")
	t.Cleanup(logger.Close)
	for _, duringRequest := range []bool{false, true} {
		t.Run(fmt.Sprintf("duringRequest=%t", duringRequest), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			attempted := make(chan struct{})
			done := make(chan error, 1)
			go func() {
				done <- reconcileNodeInfo(ctx, time.Hour, func(attemptCtx context.Context) error {
					close(attempted)
					if duringRequest {
						<-attemptCtx.Done()
						return attemptCtx.Err()
					}
					return nmagent.Error{Code: http.StatusUnauthorized}
				})
			}()
			select {
			case <-attempted:
			case <-time.After(time.Second):
				t.Fatal("registration did not start")
			}
			cancel()
			select {
			case err := <-done:
				require.ErrorIs(t, err, context.Canceled)
			case <-time.After(time.Second):
				t.Fatal("registration did not stop on cancellation")
			}
		})
	}
}
