package main

import (
	"bytes"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/fakes"
	"io/ioutil"
	"net/http"
	"testing"
)

// MockHTTPClient is a mock implementation of HTTPClient
type MockHTTPClient struct {
	Response *http.Response
	Err      error
}

// Post is the implementation of the Post method for MockHTTPClient
func (m *MockHTTPClient) Post(url string, contentType string, body []byte) (*http.Response, error) {
	return m.Response, m.Err
}

func TestPostDataWithMockClient(t *testing.T) {
	// Create a mock HTTP client
	mockResponse := &http.Response{
		StatusCode: http.StatusOK,
		Body:       ioutil.NopCloser(bytes.NewBufferString(`{"status": "success", "OrchestratorType": "Kubernetes", "DncPartitionKey": "1234", "NodeID": "5678"}`)),
		Header:     make(http.Header),
	}
	mockClient := &MockHTTPClient{Response: mockResponse, Err: nil}
	httpServiceFake := fakes.NewHTTPServiceFake()

	// Make the HTTP request using the mock client
	err := sendRegisterNodeRequest(mockClient, httpServiceFake, cns.NodeRegisterRequest{
		NumCores:             2,
		NmAgentSupportedApis: nil,
	}, "https://localhost:9000/api")
	if err != nil {
		t.Fatalf("Error making HTTP request: %v", err)
	}
}
