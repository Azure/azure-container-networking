package kubecontroller

import (
	"context"

	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
)

// ScopedClient is provided to interface with a single configured NodeNetworkConfig.
type ScopedClient struct {
	types.NamespacedName
	*nodenetworkconfig.Client
}

// NewScopedClient returns a NodeNetworkConfig client scoped to a single NodeNetworkConfig.
func NewScopedClient(cli *nodenetworkconfig.Client, key types.NamespacedName) *ScopedClient {
	return &ScopedClient{
		NamespacedName: key,
		Client:         cli,
	}
}

// Get returns the NodeNetworkConfig that this scoped client is associated to.
func (sc *ScopedClient) Get(ctx context.Context) (*v1alpha.NodeNetworkConfig, error) {
	nnc, err := sc.Client.Get(ctx, sc.NamespacedName)
	return nnc, errors.Wrapf(err, "failed to get nnc %v", sc.NamespacedName)
}

// UpdateSpec updates the associated NodeNetworkConfig with the passed NodeNetworkConfigSpec.
func (sc *ScopedClient) UpdateSpec(ctx context.Context, spec *v1alpha.NodeNetworkConfigSpec) (*v1alpha.NodeNetworkConfig, error) {
	nnc, err := sc.Client.UpdateSpec(ctx, sc.NamespacedName, spec)
	return nnc, errors.Wrapf(err, "failed to update nnc %v", sc.NamespacedName)
}
