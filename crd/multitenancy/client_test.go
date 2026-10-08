package multitenancy_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Azure/azure-container-networking/crd/multitenancy"
	"github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestNodeInfoIPv6CapabilityJSONAndDeepCopy(t *testing.T) {
	tests := []struct {
		name string
		wire string
		want *bool
	}{
		{name: "older producer", wire: `{}`},
		{name: "null report", wire: `{"nmaAppliedTheIPV6Fix":null}`},
		{name: "advertised", wire: `{"nmaAppliedTheIPV6Fix":true}`, want: ptr.To(true)},
		{name: "not advertised", wire: `{"nmaAppliedTheIPV6Fix":false}`, want: ptr.To(false)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var nodeInfo v1alpha1.NodeInfo
			require.NoError(t, json.Unmarshal([]byte(`{"spec":`+tt.wire+`}`), &nodeInfo))
			require.Equal(t, tt.want, nodeInfo.Spec.NmaAppliedTheIPV6Fix)

			data, err := json.Marshal(nodeInfo.Spec)
			require.NoError(t, err)
			if tt.want == nil {
				require.JSONEq(t, `{}`, string(data))
			} else {
				require.JSONEq(t, tt.wire, string(data))
			}

			copied := nodeInfo.DeepCopy()
			require.Equal(t, nodeInfo, *copied)
			list := &v1alpha1.NodeInfoList{Items: []v1alpha1.NodeInfo{nodeInfo}}
			copiedList := list.DeepCopy()
			require.Equal(t, list, copiedList)
			if tt.want != nil {
				require.NotSame(t, nodeInfo.Spec.NmaAppliedTheIPV6Fix, copied.Spec.NmaAppliedTheIPV6Fix)
				require.NotSame(t, nodeInfo.Spec.NmaAppliedTheIPV6Fix, copiedList.Items[0].Spec.NmaAppliedTheIPV6Fix)
				*copied.Spec.NmaAppliedTheIPV6Fix = !*tt.want
				*copiedList.Items[0].Spec.NmaAppliedTheIPV6Fix = !*tt.want
				require.Equal(t, tt.want, nodeInfo.Spec.NmaAppliedTheIPV6Fix)
			}
		})
	}
}

type mockClient struct {
	client.Client
	createFunc func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error
	patchFunc  func(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error
}

func (m *mockClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	return m.createFunc(ctx, obj, opts...)
}

func (m *mockClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	return m.patchFunc(ctx, obj, patch, opts...)
}

func TestCreateNodeInfo(t *testing.T) {
	cli := multitenancy.NodeInfoClient{
		Cli: &mockClient{
			createFunc: func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
				return nil
			},
		},
	}

	err := cli.CreateOrUpdate(context.Background(), &v1alpha1.NodeInfo{
		ObjectMeta: metav1.ObjectMeta{
			Name: "some-node",
		},
	}, "field-owner")

	require.NoError(t, err, "unexpected error creating nodeinfo crd")
}

func TestUpdateNodeInfo(t *testing.T) {
	cli := multitenancy.NodeInfoClient{
		Cli: &mockClient{
			createFunc: func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
				return apierrors.NewAlreadyExists(schema.GroupResource{}, obj.GetName())
			},
			patchFunc: func(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
				return nil
			},
		},
	}

	err := cli.CreateOrUpdate(context.Background(), &v1alpha1.NodeInfo{
		ObjectMeta: metav1.ObjectMeta{
			Name: "some-node",
		},
	}, "field-owner")

	require.NoError(t, err, "unexpected error creating nodeinfo crd")
}

func TestCreateNodeInfoInternalServerError(t *testing.T) {
	someInternalError := errors.New("some internal error") //nolint:goerr113 // dynamic error is fine here
	cli := multitenancy.NodeInfoClient{
		Cli: &mockClient{
			createFunc: func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
				return apierrors.NewInternalError(someInternalError)
			},
		},
	}

	err := cli.CreateOrUpdate(context.Background(), &v1alpha1.NodeInfo{
		ObjectMeta: metav1.ObjectMeta{
			Name: "some-node",
		},
	}, "field-owner")

	require.Error(t, err, "expected error")
	// NewInternalError doesn't wrap the error, so assert that the final error string at least preserves
	// the original error message
	require.Contains(t, err.Error(), someInternalError.Error())
}

func TestPatchNodeInfoInternalServerError(t *testing.T) {
	someInternalError := errors.New("some internal error") //nolint:goerr113 // dynamic error is fine here
	cli := multitenancy.NodeInfoClient{
		Cli: &mockClient{
			createFunc: func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
				return apierrors.NewAlreadyExists(schema.GroupResource{}, obj.GetName())
			},
			patchFunc: func(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
				return apierrors.NewInternalError(someInternalError)
			},
		},
	}

	err := cli.CreateOrUpdate(context.Background(), &v1alpha1.NodeInfo{
		ObjectMeta: metav1.ObjectMeta{
			Name: "some-node",
		},
	}, "field-owner")

	require.Error(t, err, "expected error")
	// NewInternalError doesn't wrap the error, so assert that the final error string at least preserves
	// the original error message
	require.Contains(t, err.Error(), someInternalError.Error())
}
