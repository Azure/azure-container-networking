package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/configuration"
	"github.com/Azure/azure-container-networking/cns/fakes"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockHTTPClient is a mock implementation of HTTPClient
type MockHTTPClient struct {
	Response *http.Response
	Err      error
}

var errTestFailure = errors.New("test failure")

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

func TestEnableSwiftV1DualStackCRD(t *testing.T) {
	tests := []struct {
		name              string
		enabled           bool
		crdCreationErr    error
		expectCRDCreation bool
		wantErr           bool
	}{
		{
			name:              "flag disabled - CRD creation skipped",
			enabled:           false,
			expectCRDCreation: false,
			wantErr:           false,
		},
		{
			name:              "flag enabled - CRD created successfully",
			enabled:           true,
			expectCRDCreation: true,
			wantErr:           false,
		},
		{
			name:              "flag enabled - CRD creation fails",
			enabled:           true,
			crdCreationErr:    errTestFailure,
			expectCRDCreation: true,
			wantErr:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			createNodeInfoCRD := func() error {
				called = true
				return tt.crdCreationErr
			}

			config := &configuration.CNSConfig{
				EnableSwiftV1DualStack: tt.enabled,
			}

			err := enableSwiftV1DualStackCRD(config, createNodeInfoCRD)

			assert.Equal(t, tt.expectCRDCreation, called)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "swift v1 dualstack")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
