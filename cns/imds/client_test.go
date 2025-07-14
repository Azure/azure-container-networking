// Copyright 2024 Microsoft. All rights reserved.
// MIT License

package imds_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/cns/imds"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetVMUniqueID(t *testing.T) {
	computeMetadata, err := os.ReadFile("testdata/computeMetadata.json")
	require.NoError(t, err, "error reading testdata compute metadata file")

	mockIMDSServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// request header "Metadata: true" must be present
		metadataHeader := r.Header.Get("Metadata")
		assert.Equal(t, "true", metadataHeader)

		// query params should include apiversion and json format
		apiVersion := r.URL.Query().Get("api-version")
		assert.Equal(t, "2021-01-01", apiVersion)
		format := r.URL.Query().Get("format")
		assert.Equal(t, "json", format)
		w.WriteHeader(http.StatusOK)
		_, writeErr := w.Write(computeMetadata)
		require.NoError(t, writeErr, "error writing response")
	}))
	defer mockIMDSServer.Close()

	imdsClient := imds.NewClient(imds.Endpoint(mockIMDSServer.URL))
	vmUniqueID, err := imdsClient.GetVMUniqueID(context.Background())
	require.NoError(t, err, "error querying testserver")

	require.Equal(t, "55b8499d-9b42-4f85-843f-24ff69f4a643", vmUniqueID)
}

func TestGetVMUniqueIDInvalidEndpoint(t *testing.T) {
	imdsClient := imds.NewClient(imds.Endpoint(string([]byte{0x7f})), imds.RetryAttempts(1))
	_, err := imdsClient.GetVMUniqueID(context.Background())
	require.Error(t, err, "expected invalid path")
}

func TestIMDSInternalServerError(t *testing.T) {
	mockIMDSServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// request header "Metadata: true" must be present
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer mockIMDSServer.Close()

	imdsClient := imds.NewClient(imds.Endpoint(mockIMDSServer.URL), imds.RetryAttempts(1))

	_, err := imdsClient.GetVMUniqueID(context.Background())
	require.ErrorIs(t, err, imds.ErrUnexpectedStatusCode, "expected internal server error")
}

func TestIMDSInvalidJSON(t *testing.T) {
	mockIMDSServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("not json"))
		require.NoError(t, err)
	}))
	defer mockIMDSServer.Close()

	imdsClient := imds.NewClient(imds.Endpoint(mockIMDSServer.URL), imds.RetryAttempts(1))

	_, err := imdsClient.GetVMUniqueID(context.Background())
	require.Error(t, err, "expected json decoding error")
}

func TestInvalidVMUniqueID(t *testing.T) {
	computeMetadata, err := os.ReadFile("testdata/invalidComputeMetadata.json")
	require.NoError(t, err, "error reading testdata compute metadata file")

	mockIMDSServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// request header "Metadata: true" must be present
		metadataHeader := r.Header.Get("Metadata")
		assert.Equal(t, "true", metadataHeader)

		// query params should include apiversion and json format
		apiVersion := r.URL.Query().Get("api-version")
		assert.Equal(t, "2021-01-01", apiVersion)
		format := r.URL.Query().Get("format")
		assert.Equal(t, "json", format)
		w.WriteHeader(http.StatusOK)
		_, writeErr := w.Write(computeMetadata)
		require.NoError(t, writeErr, "error writing response")
	}))
	defer mockIMDSServer.Close()

	imdsClient := imds.NewClient(imds.Endpoint(mockIMDSServer.URL))
	vmUniqueID, err := imdsClient.GetVMUniqueID(context.Background())
	require.Error(t, err, "error querying testserver")
	require.Equal(t, "", vmUniqueID)
}

func TestGetNCVersions(t *testing.T) {
	networkMetadata := []byte(`{
		"interface": [
			{
				"macAddress": "00:0D:3A:12:34:56",
				"interfaceCompartmentVersion": "1",
				"interfaceCompartmentId": "nc-12345-67890"
			},
			{
				"macAddress": "00:0D:3A:CD:EF:12",
				"interfaceCompartmentVersion": "1",
				"interfaceCompartmentId": ""
			}
		]
	}`)

	mockIMDSServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// request header "Metadata: true" must be present
		metadataHeader := r.Header.Get("Metadata")
		assert.Equal(t, "true", metadataHeader)

		// verify path is network metadata
		assert.Contains(t, r.URL.Path, "/metadata/instance/network")

		// query params should include apiversion and json format
		apiVersion := r.URL.Query().Get("api-version")
		assert.Equal(t, "2021-01-01", apiVersion)
		format := r.URL.Query().Get("format")
		assert.Equal(t, "json", format)

		w.WriteHeader(http.StatusOK)
		_, writeErr := w.Write(networkMetadata)
		require.NoError(t, writeErr, "error writing response")
	}))
	defer mockIMDSServer.Close()

	imdsClient := imds.NewClient(imds.Endpoint(mockIMDSServer.URL))
	interfaces, err := imdsClient.GetNCVersions(context.Background())
	require.NoError(t, err, "error querying testserver")

	// Verify we got the expected interfaces
	require.Len(t, interfaces, 2, "expected 2 interfaces")

	// Check first interface
	assert.Equal(t, "00:0D:3A:12:34:56", interfaces[0].MacAddress)
	assert.Equal(t, "nc-12345-67890", interfaces[0].InterfaceCompartmentId)
	assert.Equal(t, "1", interfaces[0].InterfaceCompartmentVersion)

	// Check second interface
	assert.Equal(t, "00:0D:3A:CD:EF:12", interfaces[1].MacAddress)
	assert.Equal(t, "", interfaces[1].InterfaceCompartmentId)
	assert.Equal(t, "1", interfaces[1].InterfaceCompartmentVersion)
}

func TestGetNCVersionsInvalidEndpoint(t *testing.T) {
	imdsClient := imds.NewClient(imds.Endpoint(string([]byte{0x7f})), imds.RetryAttempts(1))
	_, err := imdsClient.GetNCVersions(context.Background())
	require.Error(t, err, "expected invalid path")
}

func TestGetNCVersionsInvalidJSON(t *testing.T) {
	mockIMDSServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("not json"))
		require.NoError(t, err)
	}))
	defer mockIMDSServer.Close()

	imdsClient := imds.NewClient(imds.Endpoint(mockIMDSServer.URL), imds.RetryAttempts(1))
	_, err := imdsClient.GetNCVersions(context.Background())
	require.Error(t, err, "expected json decoding error")
}

func TestGetNCVersionsNoNCIDs(t *testing.T) {
	networkMetadataNoNC := []byte(`{
		"interface": [
			{
				"macAddress": "00:0D:3A:12:34:56",
				"ipv4": {
					"ipAddress": [
						{
							"privateIpAddress": "10.0.0.4",
							"publicIpAddress": ""
						}
					]
				}
			}
		]
	}`)

	mockIMDSServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		metadataHeader := r.Header.Get("Metadata")
		assert.Equal(t, "true", metadataHeader)

		w.WriteHeader(http.StatusOK)
		_, writeErr := w.Write(networkMetadataNoNC)
		require.NoError(t, writeErr, "error writing response")
	}))
	defer mockIMDSServer.Close()

	imdsClient := imds.NewClient(imds.Endpoint(mockIMDSServer.URL))
	interfaces, err := imdsClient.GetNCVersions(context.Background())
	require.NoError(t, err, "error querying testserver")

	// Verify we got interfaces but they don't have compartment IDs
	require.Len(t, interfaces, 1, "expected 1 interface")

	// Check that interfaces have MAC addresses but no compartment IDs
	assert.Equal(t, "00:0D:3A:12:34:56", interfaces[0].MacAddress)
	assert.Equal(t, "", interfaces[0].InterfaceCompartmentId)
}
