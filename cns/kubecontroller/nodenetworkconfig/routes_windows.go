package nodenetworkconfig

import (
	"net/netip"

	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
)

type windowsRouteClient struct{}

// NewRouteClient leaves Windows routing unchanged.
func NewRouteClient() RouteClient {
	return windowsRouteClient{}
}

func (windowsRouteClient) EnsureBlackholeRoute(netip.Prefix) error {
	return nil
}

func (*Reconciler) initializeRoutes(*v1alpha.NodeNetworkConfig) error {
	return nil
}
