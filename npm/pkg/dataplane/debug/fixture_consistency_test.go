package debug

import (
	"os"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/util"
	"github.com/stretchr/testify/require"
)

func TestV2NestedFixtureIdentitiesMatchKernelNames(t *testing.T) {
	converter := &Converter{EnableV2NPM: true}
	require.NoError(t, converter.NpmCacheFromFile(npmCacheFileV2))
	rules, err := os.ReadFile(iptableSaveFileV2)
	require.NoError(t, err)
	count := 0
	for hashedName, name := range converter.NPMCache.GetSetMap() {
		unprefixedName, nested := strings.CutPrefix(name, util.NestedLabelPrefix)
		if !nested {
			continue
		}
		count++
		t.Run(name, func(t *testing.T) {
			metadata := ipsets.NewIPSetMetadata(unprefixedName, ipsets.NestedLabelOfPod)
			require.Equal(t, metadata.GetHashedName(), hashedName)
			require.Contains(t, string(rules), hashedName)
			require.Contains(t, string(rules), name)
		})
	}
	require.Equal(t, 2, count)
}

func TestV2AggregateFixtureUsesCurrentIdentity(t *testing.T) {
	converter := &Converter{EnableV2NPM: true}
	require.NoError(t, converter.NpmCacheFromFile(npmCacheFileV2))
	metadata := ipsets.NewIPSetMetadata(util.KubeAllNamespacesFlagV2, ipsets.KeyLabelOfNamespace)
	require.Contains(t, converter.NPMCache.GetSetMap(), metadata.GetHashedName())
	require.Equal(t, metadata.GetPrefixName(), converter.NPMCache.GetSetMap()[metadata.GetHashedName()])
}
