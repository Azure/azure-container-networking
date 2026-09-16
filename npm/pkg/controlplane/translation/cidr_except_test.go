package translation

import (
	"errors"
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
		{"outside except", networkingv1.IPBlock{CIDR: parent, Except: []string{outsideExceptCIDR}}, ErrInvalidIPBlockExcept},
		{"all-addresses except", networkingv1.IPBlock{CIDR: allAddressesCIDR, Except: []string{allAddressesCIDR}}, ErrInvalidIPBlockExcept},
		{"noncanonical all-addresses except", networkingv1.IPBlock{CIDR: nonCanonAllAddrCIDR, Except: []string{nonCanonAllAddrCIDR}}, ErrInvalidIPBlockExcept},
	} {
		t.Run(test.name, func(t *testing.T) {
			set, info, err := ipBlockRule("except", defaultNS, policies.Ingress, policies.SrcMatch, 0, 0, &test.block, false)
			require.Nil(t, set)
			require.Equal(t, policies.SetInfo{}, info)
			if util.IsWindowsDP() {
				// Windows rejects the unsupported Except feature before parsing its CIDRs.
				require.ErrorIs(t, err, ErrUnsupportedExceptCIDR)
				return
			}
			require.ErrorIs(t, err, test.cause)
			require.NotErrorIs(t, err, ErrUnsupportedExceptCIDR)
			if errors.Is(test.cause, ErrInvalidIPBlockExcept) {
				// A non-strict-subset except is a relationship failure, not a parse/family
				// one, so it must not also satisfy errors.Is(err, ErrUnsupportedIPAddress).
				require.NotErrorIs(t, err, ErrUnsupportedIPAddress)
			}
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
		&networkingv1.IPBlock{CIDR: "10.0.0.0/8", Except: []string{"10.1.2.3/24"}}, false)
	require.NoError(t, err)
	require.NotNil(t, set)
	// "10.1.2.3/24" denotes the block "10.1.2.0/24"; it must be programmed in canonical form.
	require.Contains(t, set.Members, exceptCidr("10.1.2.0/24"))
	require.NotContains(t, set.Members, exceptCidr("10.1.2.3/24"))
}

// TestIPBlockAllAddressesMixedExcepts verifies the member-building loop keeps split
// replacements and appended nomatch members in separate slots when an all-addresses parent
// is combined with a mix of split-half and ordinary exceptions, in any order. A split half
// (canonicalized 0.0.0.0/1 or 128.0.0.0/1) turns its base slot into a nomatch in place, while
// an ordinary exception is appended; because the loop index advances once per exception and
// the base count drops once per split, ordinary exceptions are always written past the base
// slots and never overwrite a split half.
func TestIPBlockAllAddressesMixedExcepts(t *testing.T) {
	if util.IsWindowsDP() {
		t.Skip("Except is unsupported on the Windows datapath")
	}
	for _, except := range [][]string{
		{lowerHalfAltCIDR, outsideExceptCIDR},             // split-half (non-canonical) then ordinary
		{outsideExceptCIDR, lowerHalfAltCIDR},             // ordinary then split-half
		{lowerHalfCIDR, upperHalfCIDR, outsideExceptCIDR}, // both halves then ordinary
		{outsideExceptCIDR, lowerHalfCIDR, "203.0.113.0/24"},
	} {
		set, err := ipBlockIPSet("p", defaultNS, policies.Ingress, 0, 0,
			&networkingv1.IPBlock{CIDR: nonCanonAllAddrCIDR, Except: except}, false)
		require.NoError(t, err)

		var lowerHalf, upperHalf int
		for _, m := range set.Members {
			require.NotEmpty(t, m, "no member slot may be left empty for except %v -> %v", except, set.Members)
			switch m {
			case lowerHalfCIDR, lowerHalfNomatch:
				lowerHalf++
			case upperHalfCIDR, upperHalfNomatch:
				upperHalf++
			}
		}
		// Each half of the split must survive exactly once; a dropped or overwritten slot
		// would leave part of the parent block unrepresented.
		require.Equal(t, 1, lowerHalf, "lower /1 half must appear exactly once for except %v -> %v", except, set.Members)
		require.Equal(t, 1, upperHalf, "upper /1 half must appear exactly once for except %v -> %v", except, set.Members)
	}
}

// TestIPBlockLitePreservesRawValidation verifies NPM Lite (out of scope for this change) keeps
// its original ipBlock handling on the ipset path: a non-canonical /0 is rejected on IsIPV4
// rather than canonicalized, and a non-strict-subset except is not rejected by the full-NPM
// strict-subset validation.
func TestIPBlockLitePreservesRawValidation(t *testing.T) {
	if util.IsWindowsDP() {
		t.Skip("Windows Lite uses the direct-rule path, not ipBlockIPSet")
	}
	// A non-canonical /0 stays rejected under Lite; full NPM would canonicalize and accept it.
	_, err := ipBlockIPSet("p", defaultNS, policies.Ingress, 0, 0,
		&networkingv1.IPBlock{CIDR: nonCanonAllAddrCIDR}, true)
	require.ErrorIs(t, err, ErrUnsupportedIPAddress)

	// A non-strict-subset except does not trigger the strict-subset validation under Lite.
	set, err := ipBlockIPSet("p", defaultNS, policies.Ingress, 0, 0,
		&networkingv1.IPBlock{CIDR: "10.1.0.0/16", Except: []string{"10.0.0.0/8"}}, true)
	require.NoError(t, err)
	require.NotNil(t, set)
}
