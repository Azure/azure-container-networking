package ipsets

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// members are only important for Linux
type dirtyCacheResults struct {
	toCreate  map[string]testDiff
	toUpdate  map[string]testDiff
	toDestroy map[string]testDiff
}

type testDiff struct {
	toAdd    []string
	toDelete []string
}

const (
	ip1    = "1.1.1.1"
	ip2    = "2.2.2.2"
	podKey = "pod1"
)

func TestDirtyCacheReset(t *testing.T) {
	set1 := NewIPSet(NewIPSetMetadata("set1", Namespace))
	set2 := NewIPSet(NewIPSetMetadata("set2", Namespace))
	set3 := NewIPSet(NewIPSetMetadata("set3", Namespace))
	set4 := NewIPSet(NewIPSetMetadata("set4", Namespace))
	dc := newDirtyCache()
	dc.create(set1)
	dc.addMember(set2, ip1)
	dc.deleteMember(set3, ip2)
	dc.destroy(set4)
	dc.reset()
	assertDirtyCache(t, dc, &dirtyCacheResults{})
}

func TestDirtyCacheResetAddOrUpdate(t *testing.T) {
	set1 := NewIPSet(NewIPSetMetadata("set1", Namespace))
	set2 := NewIPSet(NewIPSetMetadata("set2", Namespace))
	set3 := NewIPSet(NewIPSetMetadata("set3", Namespace))
	set4 := NewIPSet(NewIPSetMetadata("set4", Namespace))
	dc := newDirtyCache()
	dc.create(set1)
	dc.addMember(set2, ip1)
	dc.deleteMember(set3, ip2)
	// destroy will maintain this member to delete
	set4.IPPodKey[ip1] = podKey
	dc.destroy(set4)
	dc.resetAddOrUpdateCache()
	assertDirtyCache(t, dc, &dirtyCacheResults{
		toDestroy: map[string]testDiff{
			set4.Name: {
				toDelete: []string{ip1},
			},
		},
	})
}

func TestDirtyCacheCreate(t *testing.T) {
	set1 := NewIPSet(NewIPSetMetadata("set1", Namespace))
	dc := newDirtyCache()
	dc.create(set1)
	assertDirtyCache(t, dc, &dirtyCacheResults{
		toCreate: map[string]testDiff{
			set1.Name: {},
		},
	})
}

func TestDirtyCacheCreateWithMembers(t *testing.T) {
	// hash set
	dc := newDirtyCache()
	set1 := NewIPSet(NewIPSetMetadata("set2", Namespace))
	set1.IPPodKey[ip1] = podKey
	dc.create(set1)
	assertDirtyCache(t, dc, &dirtyCacheResults{
		toCreate: map[string]testDiff{
			set1.Name: {
				toAdd: []string{ip1},
			},
		},
	})

	// list
	dc.reset()
	list := NewIPSet(NewIPSetMetadata("list", KeyValueLabelOfNamespace))
	list.MemberIPSets[set1.Name] = set1
	dc.create(list)
	assertDirtyCache(t, dc, &dirtyCacheResults{
		toCreate: map[string]testDiff{
			list.Name: {
				toAdd: []string{set1.HashedName},
			},
		},
	})
}

func TestDirtyCacheCreateAfterAddOrDelete(t *testing.T) {
	set1 := NewIPSet(NewIPSetMetadata("set1", Namespace))
	dc := newDirtyCache()
	dc.addMember(set1, ip1)
	// already updated: this would create a warning log
	dc.create(set1)
	assertDirtyCache(t, dc, &dirtyCacheResults{
		toUpdate: map[string]testDiff{
			set1.Name: {
				toAdd: []string{ip1},
			},
		},
	})

	dc.reset()
	dc.deleteMember(set1, ip1)
	// already updated: this would create a warning log
	dc.create(set1)
	assertDirtyCache(t, dc, &dirtyCacheResults{
		toUpdate: map[string]testDiff{
			set1.Name: {
				toDelete: []string{ip1},
			},
		},
	})
}

func TestDirtyCacheCreateIdempotence(t *testing.T) {
	set1 := NewIPSet(NewIPSetMetadata("set1", Namespace))
	dc := newDirtyCache()
	dc.create(set1)
	// already created: no warning log
	dc.create(set1)
	assertDirtyCache(t, dc, &dirtyCacheResults{
		toCreate: map[string]testDiff{
			set1.Name: {},
		},
	})
}

func TestDirtyCacheCreateAfterDestroy(t *testing.T) {
	set1 := NewIPSet(NewIPSetMetadata("set1", Namespace))
	dc := newDirtyCache()
	set1.IPPodKey[ip1] = podKey
	dc.destroy(set1)
	// maintain members to delete in Linux
	dc.create(set1)
	assertDirtyCache(t, dc, &dirtyCacheResults{
		toUpdate: map[string]testDiff{
			set1.Name: {
				toDelete: []string{ip1},
			},
		},
	})
}

func TestDirtyCacheDestroy(t *testing.T) {
	set1 := NewIPSet(NewIPSetMetadata("set1", Namespace))
	dc := newDirtyCache()
	dc.destroy(set1)
	assertDirtyCache(t, dc, &dirtyCacheResults{
		toDestroy: map[string]testDiff{
			set1.Name: {},
		},
	})
}

func TestDirtyCacheDestroyWithMembers(t *testing.T) {
	// hash set
	dc := newDirtyCache()
	set1 := NewIPSet(NewIPSetMetadata("set2", Namespace))
	set1.IPPodKey[ip1] = podKey
	dc.destroy(set1)
	assertDirtyCache(t, dc, &dirtyCacheResults{
		toDestroy: map[string]testDiff{
			set1.Name: {
				toDelete: []string{ip1},
			},
		},
	})

	// list
	dc.reset()
	list := NewIPSet(NewIPSetMetadata("list", KeyValueLabelOfNamespace))
	list.MemberIPSets[set1.Name] = set1
	dc.destroy(list)
	assertDirtyCache(t, dc, &dirtyCacheResults{
		toDestroy: map[string]testDiff{
			list.Name: {
				toDelete: []string{set1.HashedName},
			},
		},
	})
}

func TestDirtyCacheDestroyIdempotence(t *testing.T) {
	set1 := NewIPSet(NewIPSetMetadata("set1", Namespace))
	dc := newDirtyCache()
	dc.destroy(set1)
	// already destroyed: this would create an error log
	dc.destroy(set1)
	assertDirtyCache(t, dc, &dirtyCacheResults{
		toDestroy: map[string]testDiff{
			set1.Name: {},
		},
	})
}

func TestDirtyCacheDestroyAfterCreate(t *testing.T) {
	set1 := NewIPSet(NewIPSetMetadata("set1", Namespace))
	dc := newDirtyCache()
	dc.create(set1)
	dc.addMember(set1, ip1)
	dc.deleteMember(set1, ip2)
	dc.destroy(set1)
	// no set/diff to cache since the set was never in the kernel
	assertDirtyCache(t, dc, &dirtyCacheResults{})
}

func TestDirtyCacheDestroyAfterAdd(t *testing.T) {
	set1 := NewIPSet(NewIPSetMetadata("set1", Namespace))
	dc := newDirtyCache()
	dc.addMember(set1, ip1)
	dc.destroy(set1)
	// assumes the set was in the kernel before
	assertDirtyCache(t, dc, &dirtyCacheResults{
		toDestroy: map[string]testDiff{
			set1.Name: {},
		},
	})
}

func TestDirtyCacheDestroyAfterDelete(t *testing.T) {
	set1 := NewIPSet(NewIPSetMetadata("set1", Namespace))
	dc := newDirtyCache()
	dc.deleteMember(set1, ip1)
	dc.destroy(set1)
	// assumes the set was in the kernel before
	assertDirtyCache(t, dc, &dirtyCacheResults{
		toDestroy: map[string]testDiff{
			set1.Name: {
				toDelete: []string{ip1},
			},
		},
	})
}

func TestDirtyCacheAdd(t *testing.T) {
	set1 := NewIPSet(NewIPSetMetadata("set1", Namespace))
	dc := newDirtyCache()
	dc.addMember(set1, ip1)
	assertDirtyCache(t, dc, &dirtyCacheResults{
		toUpdate: map[string]testDiff{
			set1.Name: {
				toAdd: []string{ip1},
			},
		},
	})
}

func TestDirtyCacheAddIdempotence(t *testing.T) {
	set1 := NewIPSet(NewIPSetMetadata("set1", Namespace))
	dc := newDirtyCache()
	dc.addMember(set1, ip1)
	// no warning log
	dc.addMember(set1, ip1)
	assertDirtyCache(t, dc, &dirtyCacheResults{
		toUpdate: map[string]testDiff{
			set1.Name: {
				toAdd: []string{ip1},
			},
		},
	})
}

func TestDirtyCacheAddAfterCreate(t *testing.T) {
	set1 := NewIPSet(NewIPSetMetadata("set1", Namespace))
	dc := newDirtyCache()
	dc.create(set1)
	dc.addMember(set1, ip1)
	assertDirtyCache(t, dc, &dirtyCacheResults{
		toCreate: map[string]testDiff{
			set1.Name: {
				toAdd: []string{ip1},
			},
		},
	})
}

func TestDirtyCacheAddAfterDelete(t *testing.T) {
	set1 := NewIPSet(NewIPSetMetadata("set1", Namespace))
	dc := newDirtyCache()
	dc.deleteMember(set1, ip1)
	dc.addMember(set1, ip1)
	// in update cache despite no-diff
	assertDirtyCache(t, dc, &dirtyCacheResults{
		toUpdate: map[string]testDiff{
			set1.Name: {},
		},
	})
}

func TestDirtyCacheAddAfterDestroy(t *testing.T) {
	set1 := NewIPSet(NewIPSetMetadata("set1", Namespace))
	dc := newDirtyCache()
	dc.deleteMember(set1, ip1)
	dc.destroy(set1)
	dc.addMember(set1, ip2)
	assertDirtyCache(t, dc, &dirtyCacheResults{
		toUpdate: map[string]testDiff{
			set1.Name: {
				toAdd:    []string{ip2},
				toDelete: []string{ip1},
			},
		},
	})
}

func TestDirtyCacheDelete(t *testing.T) {
	set1 := NewIPSet(NewIPSetMetadata("set1", Namespace))
	dc := newDirtyCache()
	dc.deleteMember(set1, ip1)
	assertDirtyCache(t, dc, &dirtyCacheResults{
		toUpdate: map[string]testDiff{
			set1.Name: {
				toDelete: []string{ip1},
			},
		},
	})
}

func TestDirtyCacheDeleteIdempotence(t *testing.T) {
	set1 := NewIPSet(NewIPSetMetadata("set1", Namespace))
	dc := newDirtyCache()
	dc.deleteMember(set1, ip1)
	// no error log
	dc.deleteMember(set1, ip1)
	assertDirtyCache(t, dc, &dirtyCacheResults{
		toUpdate: map[string]testDiff{
			set1.Name: {
				toDelete: []string{ip1},
			},
		},
	})
}

func TestDirtyCacheDeleteAfterCreate(t *testing.T) {
	set1 := NewIPSet(NewIPSetMetadata("set1", Namespace))
	dc := newDirtyCache()
	dc.create(set1)
	dc.deleteMember(set1, ip1)
	assertDirtyCache(t, dc, &dirtyCacheResults{
		toCreate: map[string]testDiff{
			set1.Name: {},
		},
	})
}

func TestDirtyCacheDeleteAfterAdd(t *testing.T) {
	set1 := NewIPSet(NewIPSetMetadata("set1", Namespace))
	dc := newDirtyCache()
	dc.addMember(set1, ip1)
	dc.deleteMember(set1, ip1)
	// in update cache despite no-diff
	assertDirtyCache(t, dc, &dirtyCacheResults{
		toUpdate: map[string]testDiff{
			set1.Name: {},
		},
	})
}

func TestDirtyCacheDeleteAfterDestroy(t *testing.T) {
	set1 := NewIPSet(NewIPSetMetadata("set1", Namespace))
	dc := newDirtyCache()
	dc.deleteMember(set1, ip1)
	dc.destroy(set1)
	// do nothing and create a warning log
	dc.deleteMember(set1, ip2)
	assertDirtyCache(t, dc, &dirtyCacheResults{
		toDestroy: map[string]testDiff{
			set1.Name: {
				toDelete: []string{ip1},
			},
		},
	})
}

func TestDirtyCacheNumSetsToAddOrUpdate(t *testing.T) {
	dc := newDirtyCache()
	dc.toCreateCache["a"] = &memberDiff{}
	dc.toCreateCache["b"] = &memberDiff{}
	dc.toUpdateCache["c"] = &memberDiff{}
	require.Equal(t, 3, dc.numSetsToAddOrUpdate())
}

func assertDirtyCache(t *testing.T, dc *dirtyCache, expected *dirtyCacheResults) {
	require.Equal(t, len(expected.toCreate), len(dc.toCreateCache), "unexpected number of sets to create")
	require.Equal(t, len(expected.toUpdate), len(dc.toUpdateCache), "unexpected number of sets to update")
	require.Equal(t, len(expected.toDestroy), dc.numSetsToDelete(), "unexpected number of sets to delete")
	for setName, diff := range expected.toCreate {
		actualDiff, ok := dc.toCreateCache[setName]
		require.True(t, ok, "set %s not found in toCreateCache", setName)
		require.NotNil(t, actualDiff, "member diff should not be nil for set %s", setName)
		require.True(t, dc.isSetToAddOrUpdate(setName), "set %s should be added/updated", setName)
		require.False(t, dc.isSetToDelete(setName), "set %s should not be deleted", setName)
		// implemented in OS-specific test file
		assertDiff(t, diff, dc.memberDiff(setName))
	}
	for setName, diff := range expected.toUpdate {
		actualDiff, ok := dc.toUpdateCache[setName]
		require.True(t, ok, "set %s not found in toUpdateCache", setName)
		require.NotNil(t, actualDiff, "member diff should not be nil for set %s", setName)
		require.True(t, dc.isSetToAddOrUpdate(setName), "set %s should be added/updated", setName)
		require.False(t, dc.isSetToDelete(setName), "set %s should not be deleted", setName)
		// implemented in OS-specific test file
		assertDiff(t, diff, dc.memberDiff(setName))
	}
	for setName, diff := range expected.toDestroy {
		require.NotNil(t, dc.toDestroyCache[setName], "member diff should not be nil for set %s", setName)
		require.True(t, dc.isSetToDelete(setName), "set %s should be deleted", setName)
		require.False(t, dc.isSetToAddOrUpdate(setName), "set %s should not be added/updated", setName)
		// implemented in OS-specific test file
		assertDiff(t, diff, dc.memberDiff(setName))
	}
}
