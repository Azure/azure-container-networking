package ipsets

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func assertDiff(t *testing.T, expected testDiff, actual *memberDiff) {
	if len(expected.toAdd) == 0 {
		require.Equal(t, 0, len(actual.membersToAdd), "expected 0 members to add")
	} else {
		require.Equal(t, stringSliceToSet(expected.toAdd), actual.membersToAdd, "unexpected members to add for set")
	}
	if len(expected.toDelete) == 0 {
		require.Equal(t, 0, len(actual.membersToDelete), "expected 0 members to delete")
	} else {
		require.Equal(t, stringSliceToSet(expected.toDelete), actual.membersToDelete, "unexpected members to delete for set")
	}
}

func stringSliceToSet(s []string) map[string]struct{} {
	m := make(map[string]struct{}, len(s))
	for _, v := range s {
		m[v] = struct{}{}
	}
	return m
}
