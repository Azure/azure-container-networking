package main

import (
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/crd/multitenancy"
	mtv1alpha1 "github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
	"github.com/Azure/azure-container-networking/nmagent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

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
