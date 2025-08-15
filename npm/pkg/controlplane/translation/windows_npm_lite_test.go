package translation

import (
	"testing"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
	"github.com/Azure/azure-container-networking/npm/util"
	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
)

func TestWindowsNPMLiteCIDRDirect(t *testing.T) {
	// Skip test on Linux since this is Windows-specific functionality
	if !util.IsWindowsDP() {
		t.Skip("Skipping Windows NPM Lite test on non-Windows platform")
	}

	tests := []struct {
		name          string
		npmLiteToggle bool
		ipBlock       *networkingv1.IPBlock
		expectedCIDRs []string
		expectIPSet   bool
	}{
		{
			name:          "Windows NPM Lite with CIDR - should use direct CIDR",
			npmLiteToggle: true,
			ipBlock: &networkingv1.IPBlock{
				CIDR: "192.168.1.0/24",
			},
			expectedCIDRs: []string{"192.168.1.0/24"},
			expectIPSet:   false,
		},
		{
			name:          "Windows traditional mode - should create IPSet",
			npmLiteToggle: false,
			ipBlock: &networkingv1.IPBlock{
				CIDR: "192.168.1.0/24",
			},
			expectedCIDRs: nil,
			expectIPSet:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translatedIPSet, setInfo, err := ipBlockRule(
				"test-policy",
				"test-namespace",
				policies.Ingress,
				policies.SrcMatch,
				0, // ipBlockSetIndex
				0, // ipBlockPeerIndex
				tt.ipBlock,
				tt.npmLiteToggle,
			)

			require.NoError(t, err)

			if tt.expectIPSet {
				// Traditional mode: should have IPSet
				require.NotNil(t, translatedIPSet, "Expected IPSet to be created in traditional mode")
				require.NotNil(t, setInfo.IPSet, "Expected SetInfo to have IPSet reference")
				require.Equal(t, ipsets.CIDRBlocks, setInfo.IPSet.Type)
				require.Empty(t, setInfo.CIDRs, "Expected no direct CIDRs in traditional mode")
			} else {
				// NPM Lite mode: should have direct CIDRs, no IPSet
				require.Nil(t, translatedIPSet, "Expected no IPSet to be created in NPM Lite mode")
				require.Nil(t, setInfo.IPSet, "Expected no IPSet reference in NPM Lite mode")
				require.Equal(t, tt.expectedCIDRs, setInfo.CIDRs, "Expected direct CIDRs in NPM Lite mode")
			}

			// Common checks
			require.True(t, setInfo.Included, "Expected SetInfo to be included")
			require.Equal(t, policies.SrcMatch, setInfo.MatchType, "Expected correct match type")
		})
	}
}