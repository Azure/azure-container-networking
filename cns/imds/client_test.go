// Copyright 2024 Microsoft. All rights reserved.
// MIT License

package imds_test

import (
	"context"
	"net"
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

func TestGetNetworkInterfaces(t *testing.T) {
	networkInterfaces := []byte(`{
        "interface": [
            {
                "interfaceCompartmentID": "nc-12345-67890",
                "macAddress": "00:00:5e:00:53:01"
            },
            {
                "interfaceCompartmentID": "",
                "macAddress": "00005e005302"
            }
        ]
    }`)

	mockIMDSServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// request header "Metadata: true" must be present
		metadataHeader := r.Header.Get("Metadata")
		assert.Equal(t, "true", metadataHeader)

		// verify path is network metadata
		assert.Contains(t, r.URL.Path, "/metadata/instance/network")

		w.WriteHeader(http.StatusOK)
		_, writeErr := w.Write(networkInterfaces)
		if writeErr != nil {
			t.Errorf("error writing response: %v", writeErr)
			return
		}
	}))
	defer mockIMDSServer.Close()

	imdsClient := imds.NewClient(imds.Endpoint(mockIMDSServer.URL))
	interfaces, err := imdsClient.GetNetworkInterfaces(context.Background())
	require.NoError(t, err, "error querying testserver")

	// Verify we got the expected interfaces
	assert.Len(t, interfaces, 2, "expected 2 interfaces")

	// Check first interface
	assert.Equal(t, "nc-12345-67890", interfaces[0].InterfaceCompartmentID)
	assert.Equal(t, "00:00:5e:00:53:01", interfaces[0].MacAddress.String(), "first interface MAC address should match")

	// Check second interface
	assert.Equal(t, "", interfaces[1].InterfaceCompartmentID)
	assert.Equal(t, "00:00:5e:00:53:02", interfaces[1].MacAddress.String(), "second interface MAC address should match")

	// Test that MAC addresses can be converted to net.HardwareAddr
	firstMAC := net.HardwareAddr(interfaces[0].MacAddress)
	secondMAC := net.HardwareAddr(interfaces[1].MacAddress)

	// Verify the underlying types work correctly
	assert.Len(t, firstMAC, 6, "MAC address should be 6 bytes")
	assert.Len(t, secondMAC, 6, "MAC address should be 6 bytes")

	// Test that they're different MAC addresses
	assert.NotEqual(t, firstMAC.String(), secondMAC.String(), "MAC addresses should be different")
}

func TestGetNetworkInterfacesInvalidMAC(t *testing.T) {
	networkInterfaces := []byte(`{
        "interface": [
            {
                "interfaceCompartmentID": "nc-12345-67890",
                "macAddress": "00005e00530" // incorrect windows MAC address length
            },
        ]
    }`)

	mockIMDSServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		metadataHeader := r.Header.Get("Metadata")
		assert.Equal(t, "true", metadataHeader)

		assert.Contains(t, r.URL.Path, "/metadata/instance/network")

		w.WriteHeader(http.StatusOK)
		_, writeErr := w.Write(networkInterfaces)
		if writeErr != nil {
			t.Errorf("error writing response: %v", writeErr)
			return
		}
	}))
	defer mockIMDSServer.Close()

	imdsClient := imds.NewClient(imds.Endpoint(mockIMDSServer.URL))
	interfaces, err := imdsClient.GetNetworkInterfaces(context.Background())
	require.Error(t, err, "expected error for invalid MAC address")
	require.Nil(t, interfaces, "expected nil interfaces on error")
}

func TestGetNetworkInterfacesInvalidEndpoint(t *testing.T) {
	imdsClient := imds.NewClient(imds.Endpoint(string([]byte{0x7f})), imds.RetryAttempts(1))
	_, err := imdsClient.GetNetworkInterfaces(context.Background())
	require.Error(t, err, "expected invalid path")
}

func TestGetNetworkInterfacesInvalidJSON(t *testing.T) {
	mockIMDSServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("not json"))
		if err != nil {
			t.Errorf("error writing response: %v", err)
			return
		}
	}))
	defer mockIMDSServer.Close()

	imdsClient := imds.NewClient(imds.Endpoint(mockIMDSServer.URL), imds.RetryAttempts(1))
	_, err := imdsClient.GetNetworkInterfaces(context.Background())
	require.Error(t, err, "expected json decoding error")
}

func TestGetNetworkInterfacesNoNCIDs(t *testing.T) {
	networkInterfacesNoNC := []byte(`{
        "interface": [
            {
                "ipv4": {
                    "ipAddress": [
                        {
                            "privateIpAddress": "10.0.0.4",
                            "publicIpAddress": ""
                        }
                    ]
                },
				"macAddress": "00:00:5e:00:53:01"
            }
        ]
    }`)

	mockIMDSServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		metadataHeader := r.Header.Get("Metadata")
		assert.Equal(t, "true", metadataHeader)

		w.WriteHeader(http.StatusOK)
		_, writeErr := w.Write(networkInterfacesNoNC)
		if writeErr != nil {
			t.Errorf("error writing response: %v", writeErr)
			return
		}
	}))
	defer mockIMDSServer.Close()

	imdsClient := imds.NewClient(imds.Endpoint(mockIMDSServer.URL))
	interfaces, err := imdsClient.GetNetworkInterfaces(context.Background())
	require.NoError(t, err, "error querying testserver")

	// Verify we got interfaces but they don't have compartment IDs
	assert.Len(t, interfaces, 1, "expected 1 interface")

	// Check that interfaces don't have compartment IDs
	assert.Equal(t, "", interfaces[0].InterfaceCompartmentID)
	assert.Equal(t, "00:00:5e:00:53:01", interfaces[0].MacAddress.String(), "MAC address should match")
}

func TestGetIMDSVersions(t *testing.T) {
	mockResponseBody := `{"apiVersions": ["2017-03-01", "2021-01-01", "2025-07-24"]}`
	expectedVersions := []string{"2017-03-01", "2021-01-01", "2025-07-24"}

	mockIMDSServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request headers
		metadataHeader := r.Header.Get("Metadata")
		assert.Equal(t, "true", metadataHeader)

		w.WriteHeader(http.StatusOK)
		_, writeErr := w.Write([]byte(mockResponseBody))
		if writeErr != nil {
			t.Errorf("error writing response: %v", writeErr)
			return
		}
	}))
	defer mockIMDSServer.Close()

	imdsClient := imds.NewClient(imds.Endpoint(mockIMDSServer.URL))
	versionsResp, err := imdsClient.GetIMDSVersions(context.Background())

	require.NoError(t, err, "unexpected error")
	assert.Equal(t, expectedVersions, versionsResp.APIVersions, "API versions should match expected")
}

func TestGetIMDSVersionsInvalidJSON(t *testing.T) {
	mockIMDSServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, writeErr := w.Write([]byte(`{"invalid": json}`))
		if writeErr != nil {
			t.Errorf("error writing response: %v", writeErr)
			return
		}
	}))
	defer mockIMDSServer.Close()

	imdsClient := imds.NewClient(imds.Endpoint(mockIMDSServer.URL), imds.RetryAttempts(1))
	versionsResp, err := imdsClient.GetIMDSVersions(context.Background())

	require.Error(t, err, "expected error for invalid JSON")
	assert.Nil(t, versionsResp, "response should be nil on error")
}

func TestGetIMDSVersionsInternalServerError(t *testing.T) {
	mockIMDSServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer mockIMDSServer.Close()

	imdsClient := imds.NewClient(imds.Endpoint(mockIMDSServer.URL), imds.RetryAttempts(1))
	versionsResp, err := imdsClient.GetIMDSVersions(context.Background())

	require.Error(t, err, "expected error for 500")
	assert.Nil(t, versionsResp, "response should be nil or error")
}

func TestGetIMDSVersionsMissingAPIVersionsField(t *testing.T) {
	mockResponseBody := `{"otherField": "value"}`

	mockIMDSServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, writeErr := w.Write([]byte(mockResponseBody))
		if writeErr != nil {
			t.Errorf("error writing response: %v", writeErr)
			return
		}
	}))
	defer mockIMDSServer.Close()

	imdsClient := imds.NewClient(imds.Endpoint(mockIMDSServer.URL))
	versionsResp, err := imdsClient.GetIMDSVersions(context.Background())

	require.NoError(t, err, "unexpected error")
	assert.Nil(t, versionsResp.APIVersions, "API versions should be nil when field is missing")
}

func TestGetIMDSVersionsInvalidEndpoint(t *testing.T) {
	imdsClient := imds.NewClient(imds.Endpoint(string([]byte{0x7f})), imds.RetryAttempts(1))
	_, err := imdsClient.GetIMDSVersions(context.Background())
	require.Error(t, err, "expected error for invalid endpoint")
}
