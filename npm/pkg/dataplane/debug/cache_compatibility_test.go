package debug

import (
	"fmt"
	"testing"

	common "github.com/Azure/azure-container-networking/npm/pkg/controlplane/controllers/common"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/pb"
	"github.com/Azure/azure-container-networking/npm/util"
	"github.com/stretchr/testify/require"
)

// This implementation intentionally exposes only the original cache contract.
type legacyDiagnosticCache struct {
	cache *common.Cache
}

func (c *legacyDiagnosticCache) GetPod(input *common.Input) (*common.NpmPod, error) {
	pod, err := c.cache.GetPod(input)
	if err != nil {
		return nil, fmt.Errorf("getting cached pod: %w", err)
	}
	return pod, nil
}

func (c *legacyDiagnosticCache) GetNamespaceLabel(namespace, key string) string {
	return c.cache.GetNamespaceLabel(namespace, key)
}

func (c *legacyDiagnosticCache) GetListMap() map[string]string {
	return c.cache.GetListMap()
}

func (c *legacyDiagnosticCache) GetSetMap() map[string]string {
	return c.cache.GetSetMap()
}

func TestLegacyDiagnosticCacheCompatibility(t *testing.T) {
	const labelKey = "team"
	cache := &legacyDiagnosticCache{cache: &common.Cache{NsMap: map[string]*common.Namespace{
		util.NamespacePrefix + anchorPeerNamespace: {LabelsMap: map[string]string{labelKey: matchedTeamValue}},
	}}}
	converter := &Converter{NPMCache: cache}
	pod := &common.NpmPod{Namespace: anchorPeerNamespace}
	set := &pb.RuleResponse_SetInfo{
		Name: util.NamespacePrefix + labelKey + ":" + matchedTeamValue, Type: pb.SetType_KEYVALUELABELOFNAMESPACE, Included: true,
	}
	rule := &pb.RuleResponse{Allowed: true, SrcList: []*pb.RuleResponse_SetInfo{set}}
	hits, _, _, err := getHitRules(pod, &common.NpmPod{}, map[*pb.RuleResponse]struct{}{rule: {}}, converter.NPMCache, false)
	require.NoError(t, err)
	require.ElementsMatch(t, []*pb.RuleResponse{rule}, hits)

	set.Name = util.NamespaceLabelPrefix + labelKey + ":" + matchedTeamValue
	matched, err := evaluateSetInfo("src", set, pod, rule, converter.NPMCache, true)
	require.ErrorIs(t, err, errNamespaceLabelsUnavailable)
	require.False(t, matched)
	hits, _, _, err = getHitRules(pod, &common.NpmPod{}, map[*pb.RuleResponse]struct{}{rule: {}}, converter.NPMCache, true)
	require.ErrorIs(t, err, errNamespaceLabelsUnavailable)
	require.Nil(t, hits)
}
