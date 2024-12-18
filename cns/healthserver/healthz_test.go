package healthserver

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/cns/configuration"
	"github.com/stretchr/testify/require"
)

const nncCRD = `{
  "kind": "APIResourceList",
  "apiVersion": "v1",
  "groupVersion": "acn.azure.com/v1alpha",
  "resources": [
    {
      "name": "nodenetworkconfigs",
      "singularName": "nodenetworkconfig",
      "namespaced": true,
      "kind": "NodeNetworkConfig",
      "verbs": [
        "delete",
        "deletecollection",
        "get",
        "list",
        "patch",
        "create",
        "update",
        "watch"
      ],
      "shortNames": [
        "nnc"
      ],
      "storageVersionHash": "aGVsbG93cmxk"
    },
    {
      "name": "nodenetworkconfigs/status",
      "singularName": "",
      "namespaced": true,
      "kind": "NodeNetworkConfig",
      "verbs": [
        "get",
        "patch",
        "update"
      ]
    }
  ]
}`

const nncResult = `{
  "apiVersion": "acn.azure.com/v1alpha",
  "items": [
    {
      "apiVersion": "acn.azure.com/v1alpha",
      "kind": "NodeNetworkConfig",
      "metadata": {
        "creationTimestamp": "2024-12-04T20:42:17Z",
        "finalizers": [
          "finalizers.acn.azure.com/dnc-operations"
        ],
        "generation": 1,
        "labels": {
          "kubernetes.azure.com/podnetwork-delegationguid": "",
          "kubernetes.azure.com/podnetwork-subnet": "",
          "kubernetes.azure.com/podnetwork-type": "overlay",
          "managed": "true",
          "owner": "aks-nodepool1-1234567-vmss000000"
        },
        "managedFields": [
          {
            "apiVersion": "acn.azure.com/v1alpha",
            "fieldsType": "FieldsV1",
            "fieldsV1": {
              "f:metadata": {
                "f:finalizers": {
                  ".": {},
                  "v:\"finalizers.acn.azure.com/dnc-operations\"": {}
                },
                "f:labels": {
                  ".": {},
                  "f:kubernetes.azure.com/podnetwork-delegationguid": {},
                  "f:kubernetes.azure.com/podnetwork-subnet": {},
                  "f:kubernetes.azure.com/podnetwork-type": {},
                  "f:managed": {},
                  "f:owner": {}
                },
                "f:ownerReferences": {
                  ".": {},
                  "k:{\"uid\":\"f5117020-bbc5-11ef-8433-1b9e59caeb1d\"}": {}
                }
              },
              "f:spec": {
                ".": {},
                "f:requestedIPCount": {}
              }
            },
            "manager": "dnc-rc",
            "operation": "Update",
            "time": "2024-12-04T20:42:17Z"
          },
          {
            "apiVersion": "acn.azure.com/v1alpha",
            "fieldsType": "FieldsV1",
            "fieldsV1": {
              "f:status": {
                ".": {},
                "f:assignedIPCount": {},
                "f:networkContainers": {}
              }
            },
            "manager": "dnc-rc",
            "operation": "Update",
            "subresource": "status",
            "time": "2024-12-04T20:42:18Z"
          }
        ],
        "name": "aks-nodepool1-1234567-vmss000000",
        "namespace": "kube-system",
        "ownerReferences": [
          {
            "apiVersion": "v1",
            "blockOwnerDeletion": true,
            "controller": true,
            "kind": "Node",
            "name": "aks-nodepool1-1234567-vmss000000",
            "uid": "02df1fcc-bbc6-11ef-a76a-4b1af8d399a2"
          }
        ],
        "resourceVersion": "123456789",
        "uid": "0dc75e5e-bbc6-11ef-878f-ab45432262d6"
      },
      "spec": {
        "requestedIPCount": 0
      },
      "status": {
        "assignedIPCount": 256,
        "networkContainers": [
          {
            "assignmentMode": "static",
            "id": "13f630c0-bbc6-11ef-b3b7-bb8e46de5973",
            "nodeIP": "10.224.0.4",
            "primaryIP": "10.244.2.0/24",
            "subnetAddressSpace": "10.244.0.0/16",
            "subnetName": "routingdomain_1f7eb6ba-bbc6-11ef-8c54-7b2c1e3cbbe4_overlaysubnet",
            "type": "overlay",
            "version": 0
          }
        ]
      }
    }
  ],
  "kind": "NodeNetworkConfigList",
  "metadata": {
    "continue": "",
    "resourceVersion": "9876543210"
  }
}`

func TestNewHealthzHandlerWithChecks(t *testing.T) {
	tests := []struct {
		name            string
		cnsConfig       *configuration.CNSConfig
		apiStatusCode   int
		expectedHealthy bool
	}{
		{
			name: "list NNC gives 200 should indicate healthy",
			cnsConfig: &configuration.CNSConfig{
				ChannelMode: "CRD",
			},
			apiStatusCode:   http.StatusOK,
			expectedHealthy: true,
		},
		{
			name: "unauthorized (401) from apiserver should be unhealthy",
			cnsConfig: &configuration.CNSConfig{
				ChannelMode: "CRD",
			},
			apiStatusCode:   http.StatusUnauthorized,
			expectedHealthy: false,
		},
		{
			name: "channel nodesubnet should not call apiserver so it doesn't matter if the status code is a 401",
			cnsConfig: &configuration.CNSConfig{
				ChannelMode: "AzureHost",
			},
			apiStatusCode:   http.StatusUnauthorized,
			expectedHealthy: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configureLocalAPIServer(t, tt.apiStatusCode)

			responseRecorder := httptest.NewRecorder()
			healthHandler, err := NewHealthzHandlerWithChecks(tt.cnsConfig)
			require.NoError(t, err)

			healthHandler.ServeHTTP(responseRecorder, httptest.NewRequest("GET", "/healthz", http.NoBody))

			require.Equal(t, tt.expectedHealthy, responseRecorder.Code == http.StatusOK)
		})
	}
}

func configureLocalAPIServer(t *testing.T, expectedNNCStatusCode int) {
	// setup apiserver
	server := setupMockAPIServer(expectedNNCStatusCode)

	// write kubeConfig for test server
	kubeConfigFile, err := writeTmpKubeConfig(server.URL)
	require.NoError(t, err)

	// set env var to kubeconfig
	os.Setenv("KUBECONFIG", kubeConfigFile)

	t.Cleanup(func() {
		server.Close()
		os.Remove(kubeConfigFile)
		os.Unsetenv("KUBECONFIG")
	})
}

func writeTmpKubeConfig(host string) (string, error) {
	tempKubeConfig := `
apiVersion: v1
clusters:
- cluster:
    server: ` + host + `
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: test-context
current-context: test-context
kind: Config
preferences: {}
users:
- name: test-user
  user:
    token: test-token
`
	kubeConfigFile, err := os.CreateTemp("", "kubeconfig")
	if err != nil {
		return "", fmt.Errorf("failed to create temp kubeconfig file: %w", err)
	}

	_, err = kubeConfigFile.WriteString(tempKubeConfig)
	if err != nil {
		return "", fmt.Errorf("failed to write kubeconfig to temp file: %w", err)
	}
	kubeConfigFile.Close()
	return kubeConfigFile.Name(), nil
}

func setupMockAPIServer(code int) *httptest.Server {
	// Start a mock HTTP server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle requests based on the path
		switch r.URL.Path {
		case "/apis/acn.azure.com/v1alpha":
			_, err := w.Write([]byte(nncCRD))
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		case "/apis/acn.azure.com/v1alpha/namespaces/kube-system/nodenetworkconfigs":
			if code == http.StatusOK {
				w.Header().Set("Cache-Control", "no-cache, private")
				w.Header().Set("Content-Type", "application/json")
				_, err := w.Write([]byte(nncResult))
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
			} else {
				w.WriteHeader(code)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	return mockServer
}
