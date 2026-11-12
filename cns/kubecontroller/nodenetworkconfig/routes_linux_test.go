package nodenetworkconfig

import (
	"net/netip"
	"testing"

	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

type fakeRouteAPI struct {
	routes []netlink.Route
	addErr error
}

func (f *fakeRouteAPI) RouteAdd(route *netlink.Route) error {
	f.routes = append(f.routes, *route)
	return f.addErr
}

// TestEnsureBlackholeRoute checks IPv4/IPv6 route arguments, EEXIST success, and propagation of other add errors.
func TestEnsureBlackholeRoute(t *testing.T) {
	for _, tt := range []struct {
		name    string
		prefix  string
		family  int
		addErr  error
		wantErr error
	}{
		{name: "IPv4 success", prefix: "192.168.1.7/24", family: netlink.FAMILY_V4},
		{name: "IPv6 success", prefix: "fd00::7/120", family: netlink.FAMILY_V6},
		{name: "existing route", prefix: "192.168.1.0/24", family: netlink.FAMILY_V4, addErr: errors.Wrap(unix.EEXIST, "netlink route add")},
		{name: "permission denied", prefix: "192.168.1.0/24", family: netlink.FAMILY_V4, addErr: unix.EPERM, wantErr: unix.EPERM},
	} {
		t.Run(tt.name, func(t *testing.T) {
			api := &fakeRouteAPI{addErr: tt.addErr}
			client := &linuxRouteClient{client: api}
			prefix := netip.MustParsePrefix(tt.prefix)
			err := client.EnsureBlackholeRoute(prefix)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.Len(t, api.routes, 1)
			route := api.routes[0]
			assert.Equal(t, prefix.Masked().String(), route.Dst.String())
			assert.Equal(t, tt.family, route.Family)
			assert.Equal(t, unix.RTN_BLACKHOLE, route.Type)
			assert.Equal(t, unix.RT_TABLE_MAIN, route.Table)
			assert.Equal(t, netlink.RouteProtocol(unix.RTPROT_STATIC), route.Protocol)
			assert.Zero(t, route.LinkIndex)
			assert.Nil(t, route.Gw)
		})
	}
}

type fakeRouteClient struct {
	prefixes []netip.Prefix
	err      error
}

func (f *fakeRouteClient) EnsureBlackholeRoute(prefix netip.Prefix) error {
	f.prefixes = append(f.prefixes, prefix)
	return f.err
}

// TestInitializeOverlayRoutes checks we only add routes on overlay and validates all prefixes before programming routes.
func TestInitializeOverlayRoutes(t *testing.T) {
	ipv6 := validOverlayNC
	ipv6.ID, ipv6.PrimaryIP = "ipv6", "fd00::7/120"
	otherNode := validOverlayNC
	otherNode.NodeIP = "10.1.0.6"
	invalid := ipv6
	invalid.PrimaryIP = "not-a-prefix"
	defaultRoute := validOverlayNC
	defaultRoute.PrimaryIP = "0.0.0.0/0"
	v4, v6 := netip.MustParsePrefix("10.0.0.0/30"), netip.MustParsePrefix("fd00::/120")

	for _, tt := range []struct {
		name    string
		ncs     []v1alpha.NetworkContainer
		want    []netip.Prefix
		wantErr string
	}{
		{name: "single-stack overlay", ncs: []v1alpha.NetworkContainer{validOverlayNC}, want: []netip.Prefix{v4}},
		{name: "dual-stack overlay", ncs: []v1alpha.NetworkContainer{validOverlayNC, ipv6}, want: []netip.Prefix{v4, v6}},
		{name: "podsubnet unchanged", ncs: []v1alpha.NetworkContainer{validSwiftNC}},
		{name: "StaticBlock unchanged", ncs: []v1alpha.NetworkContainer{validVNETBlockNC}},
		{name: "different node skipped", ncs: []v1alpha.NetworkContainer{otherNode}},
		{name: "invalid second dual-stack prefix", ncs: []v1alpha.NetworkContainer{validOverlayNC, invalid}, wantErr: "invalid overlay prefix"},
		{name: "reject default route", ncs: []v1alpha.NetworkContainer{defaultRoute}, wantErr: "invalid default-route prefix"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeRouteClient{}
			r := &Reconciler{routes: client, nodeIP: nodeIP}
			err := r.initializeRoutes(&v1alpha.NodeNetworkConfig{Status: v1alpha.NodeNetworkConfigStatus{NetworkContainers: tt.ncs}})
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.want, client.prefixes)
		})
	}
}
