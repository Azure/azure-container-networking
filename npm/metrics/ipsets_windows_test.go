package metrics

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMaxMembers(t *testing.T) {
	ResetIPSetEntries()
	// set1: 1, set2: 0
	AddEntryToIPSet("test-set1")
	assertMaxMembers(t, 1)
	// set1: 1, set2: 1
	AddEntryToIPSet("test-set2")
	assertMaxMembers(t, 1)
	// set1: 2, set2: 1
	AddEntryToIPSet("test-set1")
	assertMaxMembers(t, 2)
	// set1: 2, set2: 2
	AddEntryToIPSet("test-set2")
	assertMaxMembers(t, 2)
	// set1: 2, set2: 3
	AddEntryToIPSet("test-set2")
	assertMaxMembers(t, 3)
	// set1: 1, set2: 3
	RemoveEntryFromIPSet("test-set1")
	assertMaxMembers(t, 3)
	// set1: 2, set2: 3
	AddEntryToIPSet("test-set1")
	assertMaxMembers(t, 3)
	// set1: 2, set2: 2
	RemoveEntryFromIPSet("test-set2")
	assertMaxMembers(t, 2)
	// set1: 2, set2: 1
	RemoveEntryFromIPSet("test-set2")
	assertMaxMembers(t, 2)
	// set1: 2, set2: 0
	RemoveEntryFromIPSet("test-set2")
	assertMaxMembers(t, 2)
	// set1: 1, set2: 0
	RemoveEntryFromIPSet("test-set1")
	assertMaxMembers(t, 1)
	// set1: 0, set2: 0
	RemoveEntryFromIPSet("test-set1")
	assertMaxMembers(t, 0)
	// set1: 0, set2: 0
	RemoveEntryFromIPSet("test-set2")
	assertMaxMembers(t, 0)
	// set1: 1
	AddEntryToIPSet("test-set1")
	assertMaxMembers(t, 1)
	// set1: 0
	ResetIPSetEntries()
	assertMaxMembers(t, 0)

	AddEntryToIPSet("test-set1")
	AddEntryToIPSet("test-set1")
	AddEntryToIPSet("test-set2")
	assertMaxMembers(t, 2)
	ResetIPSetEntries()
	assertMaxMembers(t, 0)

	// set1: 1, set2: 0
	AddEntryToIPSet("test-set1")
	assertMaxMembers(t, 1)
	// set1: 1, set2: 1
	AddEntryToIPSet("test-set2")
	assertMaxMembers(t, 1)
	// set1: 2, set2: 1
	AddEntryToIPSet("test-set1")
	assertMaxMembers(t, 2)
	// set1: 0, set2: 2
	RemoveAllEntriesFromIPSet("test-set1")
	assertMaxMembers(t, 1)
	// set1: 0, set2: 0
	RemoveAllEntriesFromIPSet("test-set2")
	assertMaxMembers(t, 0)

	AddEntryToIPSet("test-set1")
	AddEntryToIPSet("test-set1")
	AddEntryToIPSet("test-set2")
	AddEntryToIPSet("test-set2")
	assertMaxMembers(t, 2)
	RemoveAllEntriesFromIPSet("test-set1")
	assertMaxMembers(t, 2)
}

func assertMaxMembers(t *testing.T, expectedVal int) {
	t.Helper()

	val, err := getValue(maxIPSetMembers)
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, expectedVal, val, "incorrect max members")
}
