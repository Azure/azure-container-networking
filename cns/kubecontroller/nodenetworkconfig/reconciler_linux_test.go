package nodenetworkconfig

import (
	"context"
	"net/netip"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	cnstypes "github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestReconcilePlatformSink(t *testing.T) {
	logger.InitLogger("", 0, 0, "") //nolint:staticcheck // legacy reconciler logger

	ipv6 := validOverlayNC
	ipv6.ID, ipv6.PrimaryIP, ipv6.SubnetAddressSpace = "ipv6", "fd00::7/120", "fd00::/64"
	staticBlock := validVNETBlockNC
	staticBlock.NodeIP = nodeIP
	otherNode := validOverlayNC
	otherNode.ID, otherNode.NodeIP = "other-node", "10.1.0.6"
	overlay := []v1alpha.NetworkContainer{validOverlayNC}
	dualStack := []v1alpha.NetworkContainer{validOverlayNC, ipv6}
	v4 := netip.MustParsePrefix("10.0.0.0/30")
	v6 := netip.MustParsePrefix("fd00::/120")
	routeErr := unix.EPERM
	existingRouteErr := errors.Wrap(unix.EEXIST, "netlink route add")

	type step struct {
		ncs        []v1alpha.NetworkContainer
		wantRoutes []netip.Prefix
		wantErr    error
	}
	for _, tt := range []struct {
		name        string
		disabled    bool
		routeErrors []error
		steps       []step
	}{
		{
			name: "overlay initializes once",
			steps: []step{
				{ncs: overlay, wantRoutes: []netip.Prefix{v4}},
				{ncs: overlay},
			},
		},
		{
			name: "dual-stack initializes both families once",
			steps: []step{
				{ncs: dualStack, wantRoutes: []netip.Prefix{v4, v6}},
				{ncs: dualStack},
			},
		},
		{
			name:        "existing route initializes once",
			routeErrors: []error{existingRouteErr},
			steps: []step{
				{ncs: overlay, wantRoutes: []netip.Prefix{v4}},
				{ncs: overlay},
			},
		},
		{
			name:     "disabled",
			disabled: true,
			steps: []step{
				{ncs: overlay},
			},
		},
		{
			name: "podsubnet",
			steps: []step{
				{ncs: []v1alpha.NetworkContainer{validSwiftNC}},
			},
		},
		{
			name: "StaticBlock",
			steps: []step{
				{ncs: []v1alpha.NetworkContainer{staticBlock}},
			},
		},
		{
			name: "other node excluded",
			steps: []step{
				{ncs: []v1alpha.NetworkContainer{otherNode}},
				{ncs: []v1alpha.NetworkContainer{otherNode, validOverlayNC}, wantRoutes: []netip.Prefix{v4}},
			},
		},
		{
			name:        "first route failure retries",
			routeErrors: []error{routeErr},
			steps: []step{
				{ncs: overlay, wantRoutes: []netip.Prefix{v4}, wantErr: routeErr},
				{ncs: overlay, wantRoutes: []netip.Prefix{v4}},
				{ncs: overlay},
			},
		},
		{
			name:        "second family failure retries both",
			routeErrors: []error{nil, routeErr, existingRouteErr},
			steps: []step{
				{ncs: dualStack, wantRoutes: []netip.Prefix{v4, v6}, wantErr: routeErr},
				{ncs: dualStack, wantRoutes: []netip.Prefix{v4, v6}},
				{ncs: dualStack},
			},
		},
		{
			name: "empty then populated",
			steps: []step{
				{},
				{ncs: overlay, wantRoutes: []netip.Prefix{v4}},
				{ncs: overlay},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockCNSClient{
				state:            cnsClientState{reqsByNCID: make(map[string]*cns.CreateNetworkContainerRequest)},
				createOrUpdateNC: func(*cns.CreateNetworkContainerRequest) cnstypes.ResponseCode { return cnstypes.Success },
				update:           func(*v1alpha.NodeNetworkConfig) error { return nil },
			}
			r := NewReconciler(client, nil, client, nodeIP, false, 0, !tt.disabled)

			var current step
			r.nnccli = &mockNCGetter{get: func(context.Context, types.NamespacedName) (*v1alpha.NodeNetworkConfig, error) {
				return &v1alpha.NodeNetworkConfig{
					Status: v1alpha.NodeNetworkConfigStatus{NetworkContainers: current.ncs},
				}, nil
			}}
			var gotRoutes []netlink.Route
			routeCalls := 0
			if tt.disabled {
				require.Nil(t, r.platformSink)
			} else {
				require.NotNil(t, r.platformSink)
				r.platformSink = newPlatformSink(func(route *netlink.Route) error {
					gotRoutes = append(gotRoutes, *route)
					call := routeCalls
					routeCalls++
					if call < len(tt.routeErrors) {
						return tt.routeErrors[call]
					}
					return nil
				})
			}

			for i, next := range tt.steps {
				current = next
				gotRoutes = nil
				_, err := r.Reconcile(context.Background(), reconcile.Request{})
				if current.wantErr != nil {
					require.ErrorIs(t, err, current.wantErr, "step %d", i)
				} else {
					require.NoError(t, err, "step %d", i)
				}
				require.Len(t, gotRoutes, len(current.wantRoutes), "step %d route attempts", i)
				for j, prefix := range current.wantRoutes {
					route := gotRoutes[j]
					require.NotNil(t, route.Dst)
					assert.Equal(t, prefix.String(), route.Dst.String())
					family := netlink.FAMILY_V4
					if prefix.Addr().Is6() {
						family = netlink.FAMILY_V6
					}
					assert.Equal(t, family, route.Family)
					assert.Equal(t, unix.RTN_BLACKHOLE, route.Type)
					assert.Equal(t, unix.RT_TABLE_MAIN, route.Table)
					assert.Equal(t, netlink.RouteProtocol(unix.RTPROT_STATIC), route.Protocol)
					assert.Zero(t, route.LinkIndex)
					assert.Nil(t, route.Gw)
				}
			}
		})
	}
}
