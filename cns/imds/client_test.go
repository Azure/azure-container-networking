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
