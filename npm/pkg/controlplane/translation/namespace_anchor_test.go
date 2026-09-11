package translation

import (
	"testing"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNamespaceAnchorDoesNotAliasLabelSets(t *testing.T) {
	for _, requirement := range []metav1.LabelSelectorRequirement{
		{Key: "all-namespaces", Operator: metav1.LabelSelectorOpDoesNotExist},
		{Key: "all-namespaces", Operator: metav1.LabelSelectorOpNotIn, Values: []string{"flagged"}},
		{Key: "all", Operator: metav1.LabelSelectorOpNotIn, Values: []string{"namespaces"}},
	} {
		t.Run(requirement.Key+"/"+string(requirement.Operator), func(t *testing.T) {
			sets, matches := nameSpaceSelector(policies.SrcMatch, &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{requirement},
			})
			require.Len(t, sets, 2)
			require.Len(t, matches, 2)
			require.NotEqual(t, sets[0].Metadata.GetPrefixName(), sets[1].Metadata.GetPrefixName())
			require.NotEqual(t, sets[0].Metadata.GetHashedName(), sets[1].Metadata.GetHashedName())
			require.False(t, matches[0].Included)
			require.True(t, matches[1].Included)
		})
	}
}
