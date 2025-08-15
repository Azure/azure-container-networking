package policies

import (
	"testing"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/stretchr/testify/require"
)

func TestNewSetInfoWithCIDRs(t *testing.T) {
	cidrs := []string{"192.168.1.0/24", "10.0.0.0/8"}
	included := true
	matchType := SrcMatch

	setInfo := NewSetInfoWithCIDRs(cidrs, included, matchType)

	require.Nil(t, setInfo.IPSet, "Expected IPSet to be nil for direct CIDR SetInfo")
	require.Equal(t, cidrs, setInfo.CIDRs, "Expected CIDRs to match input")
	require.Equal(t, included, setInfo.Included, "Expected Included flag to match")
	require.Equal(t, matchType, setInfo.MatchType, "Expected MatchType to match")
}

func TestSetInfoPrettyString_WithCIDRs(t *testing.T) {
	setInfo := NewSetInfoWithCIDRs([]string{"192.168.1.0/24", "10.0.0.0/8"}, true, SrcMatch)
	
	result := setInfo.PrettyString()
	
	require.Contains(t, result, "192.168.1.0/24", "Expected CIDR to be in pretty string")
	require.Contains(t, result, "10.0.0.0/8", "Expected CIDR to be in pretty string") 
	require.Contains(t, result, "0", "Expected match type (0=SrcMatch) to be in pretty string")
	require.Contains(t, result, "true", "Expected included flag to be in pretty string")
}

func TestSetInfoPrettyString_WithIPSet(t *testing.T) {
	setInfo := NewSetInfo("test-ipset", ipsets.CIDRBlocks, true, SrcMatch)
	
	result := setInfo.PrettyString()
	
	require.Contains(t, result, "test-ipset", "Expected IPSet name to be in pretty string")
	require.Contains(t, result, "0", "Expected match type (0=SrcMatch) to be in pretty string")
	require.Contains(t, result, "true", "Expected included flag to be in pretty string")
}