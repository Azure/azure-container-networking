package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/fakes"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/crd/multitenancy"
	mtv1alpha1 "github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
	"github.com/Azure/azure-container-networking/nmagent"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

const (
	testVMUniqueID = "test-vm-id"
	testHomeAZ     = "AZ01"
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
	calls      int
}

func (m *mockIMDSClient) GetVMUniqueID(_ context.Context) (string, error) {
	m.calls++
	return m.vmUniqueID, m.err
}

// mockNMAgentClient is a mock implementation of the HomeAzGetter interface
type mockNMAgentClient struct {
	homeAzResponse nmagent.AzResponse
	err            error
	calls          int
}

func (m *mockNMAgentClient) GetHomeAz(_ context.Context) (nmagent.AzResponse, error) {
	m.calls++
	return m.homeAzResponse, m.err
}

// mockNodeInfoClient is a mock implementation of the NodeInfoClient interface
type mockNodeInfoClient struct {
	createdNodeInfo *mtv1alpha1.NodeInfo
	err             error
	fieldOwner      string
}

func (m *mockNodeInfoClient) CreateOrUpdate(_ context.Context, nodeInfo *mtv1alpha1.NodeInfo, fieldOwner string) error {
	m.createdNodeInfo = nodeInfo
	m.fieldOwner = fieldOwner
	return m.err
}

func TestBuildNodeInfoSpec_WithHomeAZ(t *testing.T) {
	tests := []struct {
		name            string
		vmUniqueID      string
		vmUniqueIDErr   error
		homeAzResponse  nmagent.AzResponse
		homeAzErr       error
		nodeInfoErr     error
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
				VMUniqueID:           "test-vm-unique-id",
				HomeAZ:               testHomeAZ,
				NmaAppliedTheIPV6Fix: ptr.To(false),
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
				VMUniqueID:           "another-vm-id",
				HomeAZ:               "AZ02",
				NmaAppliedTheIPV6Fix: ptr.To(false),
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
				VMUniqueID:           "vm-id-zone10",
				HomeAZ:               "AZ10",
				NmaAppliedTheIPV6Fix: ptr.To(false),
			},
			expectedNodeErr: false,
		},
		{
			name:       "HomeAZ advertises IPv6 fix",
			vmUniqueID: testVMUniqueID,
			homeAzResponse: nmagent.AzResponse{
				HomeAz:       1,
				AppliedFixes: []nmagent.HomeAZFix{nmagent.HomeAZFixIPv6},
			},
			expectedSpec: mtv1alpha1.NodeInfoSpec{
				VMUniqueID:           testVMUniqueID,
				HomeAZ:               testHomeAZ,
				NmaAppliedTheIPV6Fix: ptr.To(true),
			},
		},
		{
			name:       "unrelated fix does not advertise IPv6 support",
			vmUniqueID: testVMUniqueID,
			homeAzResponse: nmagent.AzResponse{
				HomeAz:       1,
				AppliedFixes: []nmagent.HomeAZFix{nmagent.HomeAZFixInvalid},
			},
			expectedSpec: mtv1alpha1.NodeInfoSpec{
				VMUniqueID:           testVMUniqueID,
				HomeAZ:               testHomeAZ,
				NmaAppliedTheIPV6Fix: ptr.To(false),
			},
		},
		{
			name:       "HomeAZ error with advertised IPv6 fix",
			vmUniqueID: testVMUniqueID,
			homeAzResponse: nmagent.AzResponse{
				HomeAz:       1,
				AppliedFixes: []nmagent.HomeAZFix{nmagent.HomeAZFixIPv6},
			},
			homeAzErr:       errors.New("nmagent error"),
			expectedNodeErr: true,
		},
		{
			name:       "zero HomeAZ with advertised IPv6 fix",
			vmUniqueID: testVMUniqueID,
			homeAzResponse: nmagent.AzResponse{
				AppliedFixes: []nmagent.HomeAZFix{nmagent.HomeAZFixIPv6},
			},
			expectedSpec: mtv1alpha1.NodeInfoSpec{
				VMUniqueID: testVMUniqueID,
			},
		},
		{
			name:            "NodeInfo publication error",
			vmUniqueID:      testVMUniqueID,
			homeAzResponse:  nmagent.AzResponse{HomeAz: 1},
			nodeInfoErr:     errors.New("nodeinfo error"),
			expectedNodeErr: true,
		},
		{
			name:           "HomeAZ not available",
			vmUniqueID:     testVMUniqueID,
			vmUniqueIDErr:  nil,
			homeAzResponse: nmagent.AzResponse{},
			homeAzErr:      errors.New("nmagent HomeAZ not available"),
			expectedSpec: mtv1alpha1.NodeInfoSpec{
				VMUniqueID: testVMUniqueID,
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
			vmUniqueID:     testVMUniqueID,
			vmUniqueIDErr:  nil,
			homeAzResponse: nmagent.AzResponse{HomeAz: 0},
			homeAzErr:      nil,
			expectedSpec: mtv1alpha1.NodeInfoSpec{
				VMUniqueID: testVMUniqueID,
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
			nodeInfoCli := &mockNodeInfoClient{err: test.nodeInfoErr}
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
					UID:  "test-uid",
				},
			}

			err := buildAndCreateNodeInfo(context.Background(), imdsCli, nmaCli, nodeInfoCli, node)
			require.Equal(t, 1, imdsCli.calls)
			if test.vmUniqueIDErr != nil {
				require.Zero(t, nmaCli.calls)
				require.ErrorIs(t, err, test.vmUniqueIDErr)
			} else {
				require.Equal(t, 1, nmaCli.calls)
			}
			if test.homeAzErr != nil {
				require.ErrorIs(t, err, test.homeAzErr)
			}
			if test.nodeInfoErr != nil {
				require.ErrorIs(t, err, test.nodeInfoErr)
			}
			if test.expectedNodeErr {
				require.Error(t, err)
				if test.nodeInfoErr == nil {
					require.Nil(t, nodeInfoCli.createdNodeInfo)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, nodeInfoCli.createdNodeInfo)
			assert.Equal(t, test.expectedSpec, nodeInfoCli.createdNodeInfo.Spec)
			assert.Equal(t, node.Name, nodeInfoCli.createdNodeInfo.Name)
			assert.Equal(t, "azure-cns", nodeInfoCli.fieldOwner)
			require.Len(t, nodeInfoCli.createdNodeInfo.OwnerReferences, 1)
			assert.Equal(t, node.UID, nodeInfoCli.createdNodeInfo.OwnerReferences[0].UID)

			data, err := json.Marshal(nodeInfoCli.createdNodeInfo)
			require.NoError(t, err)
			if test.expectedSpec.NmaAppliedTheIPV6Fix != nil {
				var wire struct {
					Spec map[string]json.RawMessage `json:"spec"`
				}
				require.NoError(t, json.Unmarshal(data, &wire))
				require.Contains(t, wire.Spec, "nmaAppliedTheIPV6Fix")
				var reported bool
				require.NoError(t, json.Unmarshal(wire.Spec["nmaAppliedTheIPV6Fix"], &reported))
				assert.Equal(t, *test.expectedSpec.NmaAppliedTheIPV6Fix, reported)
			} else {
				assert.NotContains(t, string(data), "nmaAppliedTheIPV6Fix")
			}
		})
	}
}

func TestBuildAndCreateNodeInfo_UpdateIPv6Capability(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("KUBEBUILDER_ASSETS is required for the NodeInfo API round-trip")
	}
	ctx := t.Context()
	legacyCRD, err := multitenancy.GetNodeInfo()
	require.NoError(t, err)
	delete(legacyCRD.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["spec"].Properties, "nmaAppliedTheIPV6Fix")

	testEnv := &envtest.Environment{
		CRDs: []*apiextensionsv1.CustomResourceDefinition{legacyCRD},
	}
	cfg, err := testEnv.Start()
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, testEnv.Stop())
	})

	installer, err := multitenancy.NewInstaller(cfg)
	require.NoError(t, err)
	installed, err := installer.InstallOrUpdateNodeInfo(ctx)
	require.NoError(t, err)
	schema := installed.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["spec"]
	require.Contains(t, schema.Properties, "nmaAppliedTheIPV6Fix")
	unchanged, err := installer.InstallOrUpdateNodeInfo(ctx)
	require.NoError(t, err)
	assert.Equal(t, installed.ResourceVersion, unchanged.ResourceVersion)

	cli, err := client.New(cfg, client.Options{Scheme: multitenancy.Scheme})
	require.NoError(t, err)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "test-node"}}
	require.NoError(t, cli.Create(ctx, node))
	nodeInfoCli := &multitenancy.NodeInfoClient{Cli: cli}
	imdsCli := &mockIMDSClient{vmUniqueID: testVMUniqueID}
	nmaCli := &mockNMAgentClient{
		homeAzResponse: nmagent.AzResponse{
			HomeAz:       1,
			AppliedFixes: []nmagent.HomeAZFix{nmagent.HomeAZFixIPv6},
		},
	}

	require.NoError(t, buildAndCreateNodeInfo(ctx, imdsCli, nmaCli, nodeInfoCli, node))
	created, err := nodeInfoCli.Get(ctx, node.Name)
	require.NoError(t, err)
	require.Equal(t, ptr.To(true), created.Spec.NmaAppliedTheIPV6Fix)
	require.Len(t, created.OwnerReferences, 1)
	assert.Equal(t, node.UID, created.OwnerReferences[0].UID)

	created.Labels = map[string]string{"retained": "value"}
	require.NoError(t, cli.Update(ctx, created))
	created.Status.DeviceInfos = []mtv1alpha1.DeviceInfo{{MacAddress: "00:00:00:00:00:01"}}
	require.NoError(t, cli.Status().Update(ctx, created))

	nmaCli.homeAzResponse.AppliedFixes = nil
	require.NoError(t, buildAndCreateNodeInfo(ctx, imdsCli, nmaCli, nodeInfoCli, node))
	updated, err := nodeInfoCli.Get(ctx, node.Name)
	require.NoError(t, err)
	assert.Equal(t, mtv1alpha1.NodeInfoSpec{
		VMUniqueID:           testVMUniqueID,
		HomeAZ:               testHomeAZ,
		NmaAppliedTheIPV6Fix: ptr.To(false),
	}, updated.Spec)
	assert.Equal(t, created.OwnerReferences, updated.OwnerReferences)
	assert.Equal(t, created.Labels, updated.Labels)
	assert.Equal(t, created.Status, updated.Status)
	assert.Equal(t, 2, imdsCli.calls)
	assert.Equal(t, 2, nmaCli.calls)

	var cnsFields *metav1.FieldsV1
	for _, fields := range updated.ManagedFields {
		if fields.Manager == "azure-cns" && fields.Operation == metav1.ManagedFieldsOperationApply {
			cnsFields = fields.FieldsV1
		}
	}
	require.NotNil(t, cnsFields)
	assert.Contains(t, cnsFields.GetRawString(), `"f:nmaAppliedTheIPV6Fix"`)

	nmaCli.homeAzResponse = nmagent.AzResponse{
		HomeAz:       100,
		AppliedFixes: []nmagent.HomeAZFix{nmagent.HomeAZFixIPv6},
	}
	require.Error(t, buildAndCreateNodeInfo(ctx, imdsCli, nmaCli, nodeInfoCli, node))
	afterInvalid, err := nodeInfoCli.Get(ctx, node.Name)
	require.NoError(t, err)
	assert.Equal(t, updated.Spec, afterInvalid.Spec)
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
