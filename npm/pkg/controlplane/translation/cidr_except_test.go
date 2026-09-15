package translation

import (
	"testing"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
	"github.com/Azure/azure-container-networking/npm/util"
	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
)

// TestIPBlockExceptFailsClosed verifies that on the ipset path an except that is not an IPv4
// CIDR, or is not a strict subset of its parent, fails the translation instead of being carried
// untouched into the set. Passing it through would either widen the allow (a dropped exclusion)
// or take the whole set down at ipset restore time (an unprogrammable member). Kubernetes rejects
// these at admission, so this is defense in depth on the datapath's own boundary.
func TestIPBlockExceptFailsClosed(t *testing.T) {
	const parent = "10.0.0.0/8"
	for _, test := range []struct {
		name  string
		block networkingv1.IPBlock
		cause error
	}{
		{"malformed except", networkingv1.IPBlock{CIDR: parent, Except: []string{"invalid"}}, ErrUnsupportedIPAddress},
		{"IPv6 except", networkingv1.IPBlock{CIDR: parent, Except: []string{"2001:db8::/48"}}, ErrUnsupportedIPAddress},
		{"equal except", networkingv1.IPBlock{CIDR: parent, Except: []string{parent}}, ErrInvalidIPBlockExcept},
		{"noncanonical equal except", networkingv1.IPBlock{CIDR: parent, Except: []string{"10.1.2.3/8"}}, ErrInvalidIPBlockExcept},
		{"broader except", networkingv1.IPBlock{CIDR: "10.1.0.0/16", Except: []string{parent}}, ErrInvalidIPBlockExcept},
		{"outside except", networkingv1.IPBlock{CIDR: parent, Except: []string{"192.0.2.0/24"}}, ErrInvalidIPBlockExcept},
		{"all-addresses except", networkingv1.IPBlock{CIDR: "0.0.0.0/0", Except: []string{"0.0.0.0/0"}}, ErrInvalidIPBlockExcept},
		{"noncanonical all-addresses except", networkingv1.IPBlock{CIDR: "10.0.0.0/0", Except: []string{"10.0.0.0/0"}}, ErrInvalidIPBlockExcept},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			set, info, err := ipBlockRule("except", defaultNS, policies.Ingress, policies.SrcMatch, 0, 0, &test.block)
			require.Nil(t, set)
			require.Equal(t, policies.SetInfo{}, info)
			if util.IsWindowsDP() {
				// Windows rejects the unsupported Except feature before parsing its CIDRs.
				require.ErrorIs(t, err, ErrUnsupportedExceptCIDR)
				return
			}
			require.ErrorIs(t, err, ErrUnsupportedIPAddress)
			require.ErrorIs(t, err, test.cause)
			require.NotErrorIs(t, err, ErrUnsupportedExceptCIDR)
		})
	}
}

// TestIPBlockValidStrictSubsetExceptIsCanonicalized verifies the strict-subset validation does
// not reject a legitimate exclusion and that a non-canonical spelling is normalized before it is
// programmed as a nomatch member.
func TestIPBlockValidStrictSubsetExceptIsCanonicalized(t *testing.T) {
	if util.IsWindowsDP() {
		t.Skip("Except is unsupported on the Windows datapath")
	}
	set, _, err := ipBlockRule("except", defaultNS, policies.Ingress, policies.SrcMatch, 0, 0,
		&networkingv1.IPBlock{CIDR: "10.0.0.0/8", Except: []string{"10.1.2.3/24"}})
	require.NoError(t, err)
	require.NotNil(t, set)
	// "10.1.2.3/24" denotes the block "10.1.2.0/24"; it must be programmed in canonical form.
	require.Contains(t, set.Members, exceptCidr("10.1.2.0/24"))
	require.NotContains(t, set.Members, exceptCidr("10.1.2.3/24"))
}
