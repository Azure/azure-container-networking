package ipsets

import (
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/util"
	testutils "github.com/Azure/azure-container-networking/test/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testSetName  = "test-set"
	testListName = "test-list"
	testPodKey   = "test-pod-key"
	testPodIP    = "10.0.0.0"
)

var (
	applyOnNeedCfg = &IPSetManagerCfg{
		IPSetMode:   ApplyOnNeed,
		NetworkName: "azure",
	}

	applyAlwaysCfg = &IPSetManagerCfg{
		IPSetMode:   ApplyAllIPSets,
		NetworkName: "azure",
	}
)

type cacheValues struct {
	mainCache             []string
	addUpdateCacheMembers map[string][]member
	toDeleteCache         []string
}

type member struct {
	// either an IP or set name
	value string
	kind  memberKind
}

type memberKind bool

const (
	isIP  = memberKind(true)
	isSet = memberKind(false)
)

func TestCreateIPSet(t *testing.T) {
	m1 := NewIPSetMetadata(testSetName, Namespace)
	m2 := NewIPSetMetadata(testListName, KeyLabelOfNamespace)
	tests := []struct {
		name                      string
		cfg                       *IPSetManagerCfg
		metadatas                 []*IPSetMetadata
		expectedCache             cacheValues
		expectedNumIPSets         int
		expectedNumIPSetsInKernel int
	}{
		{
			name:      "Apply Always: create two new sets",
			cfg:       applyAlwaysCfg,
			metadatas: []*IPSetMetadata{m1, m2},
			expectedCache: cacheValues{
				mainCache: []string{m1.GetPrefixName(), m2.GetPrefixName()},
				addUpdateCacheMembers: map[string][]member{
					m1.GetPrefixName(): {},
					m2.GetPrefixName(): {},
				},
				toDeleteCache: []string{},
			},
			expectedNumIPSets:         2,
			expectedNumIPSetsInKernel: 2,
		},
		{
			name:      "Apply On Need: create two new sets",
			cfg:       applyOnNeedCfg,
			metadatas: []*IPSetMetadata{m1, m2},
			expectedCache: cacheValues{
				mainCache:             []string{m1.GetPrefixName(), m2.GetPrefixName()},
				addUpdateCacheMembers: map[string][]member{},
				toDeleteCache:         []string{},
			},
			expectedNumIPSets:         2,
			expectedNumIPSetsInKernel: 0,
		},
		{
			name:      "Apply Always: no-op for set that exists",
			cfg:       applyAlwaysCfg,
			metadatas: []*IPSetMetadata{m1, m1},
			expectedCache: cacheValues{
				mainCache: []string{m1.GetPrefixName()},
				addUpdateCacheMembers: map[string][]member{
					m1.GetPrefixName(): {},
				},
				toDeleteCache: []string{},
			},
			expectedNumIPSets:         1,
			expectedNumIPSetsInKernel: 1,
		},
		{
			name:      "Apply On Need: no-op for set that exists",
			cfg:       applyOnNeedCfg,
			metadatas: []*IPSetMetadata{m1, m1},
			expectedCache: cacheValues{
				mainCache:             []string{m1.GetPrefixName()},
				addUpdateCacheMembers: map[string][]member{},
				toDeleteCache:         []string{},
			},
			expectedNumIPSets:         1,
			expectedNumIPSetsInKernel: 0,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			metrics.ReinitializeAll()
			iMgr := NewIPSetManager(tt.cfg, common.NewMockIOShim(nil))
			iMgr.CreateIPSets(tt.metadatas)

			assertEqualCache(t, iMgr, tt.expectedCache)

			numIPSets, _ := metrics.GetNumIPSets()
			assert.Equal(t, tt.expectedNumIPSets, numIPSets)
			// TODO update when we have prometheus metric for in kernel
			numIPSetsInKernel := tt.expectedNumIPSetsInKernel
			assert.Equal(t, tt.expectedNumIPSetsInKernel, numIPSetsInKernel)
			numEntries, _ := metrics.GetNumIPSetEntries()
			assert.Equal(t, 0, numEntries)
			for _, setName := range tt.expectedCache.mainCache {
				numEntries, _ := metrics.GetNumEntriesForIPSet(setName)
				assert.Equal(t, 0, numEntries)
			}
		})
	}
}

func TestDeleteIPSet(t *testing.T) {
	m1 := NewIPSetMetadata(testSetName, Namespace)
	m2 := NewIPSetMetadata(testListName, KeyLabelOfNamespace)
	tests := []struct {
		name              string
		cfg               *IPSetManagerCfg
		toCreateMetadatas []*IPSetMetadata
		toDeleteName      string
		expectedCache     cacheValues
		expectedNumIPSets int
	}{
		{
			name:              "Apply Always: delete set",
			cfg:               applyAlwaysCfg,
			toCreateMetadatas: []*IPSetMetadata{m1},
			toDeleteName:      m1.GetPrefixName(),
			expectedCache: cacheValues{
				mainCache:             []string{},
				addUpdateCacheMembers: map[string][]member{},
				toDeleteCache:         []string{m1.GetPrefixName()},
			},
			expectedNumIPSets: 0,
		},
		{
			name:              "Apply On Need: delete set",
			cfg:               applyOnNeedCfg,
			toCreateMetadatas: []*IPSetMetadata{m1},
			toDeleteName:      m1.GetPrefixName(),
			expectedCache: cacheValues{
				mainCache:             []string{},
				addUpdateCacheMembers: map[string][]member{},
				toDeleteCache:         []string{},
			},
			expectedNumIPSets: 0,
		},
		{
			name:              "Apply Always: set doesn't exist",
			cfg:               applyAlwaysCfg,
			toCreateMetadatas: []*IPSetMetadata{m1},
			toDeleteName:      m2.GetPrefixName(),
			expectedCache: cacheValues{
				mainCache:             []string{m1.GetPrefixName()},
				addUpdateCacheMembers: map[string][]member{},
				toDeleteCache:         []string{},
			},
			expectedNumIPSets: 1,
		},
		{
			name:              "Apply On Need: set doesn't exist",
			cfg:               applyOnNeedCfg,
			toCreateMetadatas: []*IPSetMetadata{m1},
			toDeleteName:      m2.GetPrefixName(),
			expectedCache: cacheValues{
				mainCache:             []string{m1.GetPrefixName()},
				addUpdateCacheMembers: map[string][]member{},
				toDeleteCache:         []string{},
			},
			expectedNumIPSets: 1,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			metrics.ReinitializeAll()
			calls := GetApplyIPSetsTestCalls(tt.toCreateMetadatas, nil)
			iMgr := NewIPSetManager(tt.cfg, common.NewMockIOShim(calls))
			iMgr.CreateIPSets(tt.toCreateMetadatas)
			require.NoError(t, iMgr.ApplyIPSets())
			iMgr.DeleteIPSet(tt.toDeleteName)

			assertEqualCache(t, iMgr, tt.expectedCache)

			numIPSets, _ := metrics.GetNumIPSets()
			assert.Equal(t, tt.expectedNumIPSets, numIPSets)
			// TODO update when we have prometheus metric for in kernel
			numIPSetsInKernel := 0
			assert.Equal(t, 0, numIPSetsInKernel)
			numEntries, _ := metrics.GetNumIPSetEntries()
			assert.Equal(t, 0, numEntries)
			numEntries, _ = metrics.GetNumEntriesForIPSet(tt.toDeleteName)
			assert.Equal(t, 0, numEntries)
		})
	}
}

func TestDeleteIPSetNotAllowed(t *testing.T) {
	metrics.ReinitializeAll()
	m := NewIPSetMetadata(testSetName, Namespace)
	l := NewIPSetMetadata(testListName, KeyLabelOfNamespace)
	calls := GetApplyIPSetsTestCalls([]*IPSetMetadata{l, m}, nil)
	iMgr := NewIPSetManager(applyOnNeedCfg, common.NewMockIOShim(calls))
	require.NoError(t, iMgr.AddToLists([]*IPSetMetadata{l}, []*IPSetMetadata{m}))
	require.NoError(t, iMgr.ApplyIPSets())
	iMgr.DeleteIPSet(m.GetPrefixName())

	assertEqualCache(t, iMgr, cacheValues{
		mainCache:             []string{l.GetPrefixName(), m.GetPrefixName()},
		addUpdateCacheMembers: map[string][]member{},
		toDeleteCache:         []string{},
	})

	numIPSets, _ := metrics.GetNumIPSets()
	assert.Equal(t, 2, numIPSets)
	// TODO update when we have prometheus metric for in kernel
	numIPSetsInKernel := 0
	assert.Equal(t, 0, numIPSetsInKernel)
	numEntries, _ := metrics.GetNumIPSetEntries()
	assert.Equal(t, 1, numEntries)
	numEntries, _ = metrics.GetNumEntriesForIPSet(m.GetPrefixName())
	assert.Equal(t, 0, numEntries)
	numEntries, _ = metrics.GetNumEntriesForIPSet(l.GetPrefixName())
	assert.Equal(t, 1, numEntries)
}

func TestAddToSet(t *testing.T) {
	iMgr := NewIPSetManager(applyOnNeedCfg, common.NewMockIOShim([]testutils.TestCmd{}))

	setMetadata := NewIPSetMetadata(testSetName, Namespace)
	iMgr.CreateIPSets([]*IPSetMetadata{setMetadata})

	err := iMgr.AddToSets([]*IPSetMetadata{setMetadata}, testPodIP, testPodKey)
	require.NoError(t, err)

	err = iMgr.AddToSets([]*IPSetMetadata{setMetadata}, "2001:db8:0:0:0:0:2:1", "newpod")
	require.NoError(t, err)

	// same IP changed podkey
	err = iMgr.AddToSets([]*IPSetMetadata{setMetadata}, testPodIP, "newpod")
	require.NoError(t, err)

	listMetadata := NewIPSetMetadata("testipsetlist", KeyLabelOfNamespace)
	iMgr.CreateIPSets([]*IPSetMetadata{listMetadata})
	err = iMgr.AddToSets([]*IPSetMetadata{listMetadata}, testPodIP, testPodKey)
	require.Error(t, err)
}

func TestRemoveFromSet(t *testing.T) {
	iMgr := NewIPSetManager(applyOnNeedCfg, common.NewMockIOShim([]testutils.TestCmd{}))

	setMetadata := NewIPSetMetadata(testSetName, Namespace)
	iMgr.CreateIPSets([]*IPSetMetadata{setMetadata})
	err := iMgr.AddToSets([]*IPSetMetadata{setMetadata}, testPodIP, testPodKey)
	require.NoError(t, err)
	err = iMgr.RemoveFromSets([]*IPSetMetadata{setMetadata}, testPodIP, testPodKey)
	require.NoError(t, err)
}

func TestRemoveFromSetMissing(t *testing.T) {
	iMgr := NewIPSetManager(applyOnNeedCfg, common.NewMockIOShim([]testutils.TestCmd{}))
	setMetadata := NewIPSetMetadata(testSetName, Namespace)
	err := iMgr.RemoveFromSets([]*IPSetMetadata{setMetadata}, testPodIP, testPodKey)
	require.NoError(t, err)
}

func TestAddToListMissing(t *testing.T) {
	iMgr := NewIPSetManager(applyOnNeedCfg, common.NewMockIOShim([]testutils.TestCmd{}))
	setMetadata := NewIPSetMetadata(testSetName, Namespace)
	listMetadata := NewIPSetMetadata("testlabel", KeyLabelOfNamespace)
	err := iMgr.AddToLists([]*IPSetMetadata{listMetadata}, []*IPSetMetadata{setMetadata})
	require.NoError(t, err)
}

func TestAddToList(t *testing.T) {
	iMgr := NewIPSetManager(applyOnNeedCfg, common.NewMockIOShim([]testutils.TestCmd{}))
	setMetadata := NewIPSetMetadata(testSetName, Namespace)
	listMetadata := NewIPSetMetadata(testListName, KeyLabelOfNamespace)
	iMgr.CreateIPSets([]*IPSetMetadata{setMetadata, listMetadata})

	err := iMgr.AddToLists([]*IPSetMetadata{listMetadata}, []*IPSetMetadata{setMetadata})
	require.NoError(t, err)

	set := iMgr.GetIPSet(listMetadata.GetPrefixName())
	assert.NotNil(t, set)
	assert.Equal(t, listMetadata.GetPrefixName(), set.Name)
	assert.Equal(t, util.GetHashedName(listMetadata.GetPrefixName()), set.HashedName)
	assert.Equal(t, 1, len(set.MemberIPSets))
	assert.Equal(t, setMetadata.GetPrefixName(), set.MemberIPSets[setMetadata.GetPrefixName()].Name)
}

func TestRemoveFromList(t *testing.T) {
	iMgr := NewIPSetManager(applyOnNeedCfg, common.NewMockIOShim([]testutils.TestCmd{}))
	setMetadata := NewIPSetMetadata(testSetName, Namespace)
	listMetadata := NewIPSetMetadata(testListName, KeyLabelOfNamespace)
	iMgr.CreateIPSets([]*IPSetMetadata{setMetadata, listMetadata})

	err := iMgr.AddToLists([]*IPSetMetadata{listMetadata}, []*IPSetMetadata{setMetadata})
	require.NoError(t, err)

	set := iMgr.GetIPSet(listMetadata.GetPrefixName())
	assert.NotNil(t, set)
	assert.Equal(t, listMetadata.GetPrefixName(), set.Name)
	assert.Equal(t, util.GetHashedName(listMetadata.GetPrefixName()), set.HashedName)
	assert.Equal(t, 1, len(set.MemberIPSets))
	assert.Equal(t, setMetadata.GetPrefixName(), set.MemberIPSets[setMetadata.GetPrefixName()].Name)

	err = iMgr.RemoveFromList(listMetadata, []*IPSetMetadata{setMetadata})
	require.NoError(t, err)

	set = iMgr.GetIPSet(listMetadata.GetPrefixName())
	assert.NotNil(t, set)
	assert.Equal(t, 0, len(set.MemberIPSets))
}

func TestRemoveFromListMissing(t *testing.T) {
	iMgr := NewIPSetManager(applyOnNeedCfg, common.NewMockIOShim([]testutils.TestCmd{}))

	setMetadata := NewIPSetMetadata(testSetName, Namespace)
	listMetadata := NewIPSetMetadata(testListName, KeyLabelOfNamespace)
	iMgr.CreateIPSets([]*IPSetMetadata{listMetadata})

	err := iMgr.RemoveFromList(listMetadata, []*IPSetMetadata{setMetadata})
	require.NoError(t, err)
}
func TestGetIPsFromSelectorIPSets(t *testing.T) {
	iMgr := NewIPSetManager(applyOnNeedCfg, common.NewMockIOShim([]testutils.TestCmd{}))
	setsTocreate := []*IPSetMetadata{
		{
			Name: "setNs1",
			Type: Namespace,
		},
		{
			Name: "setpod1",
			Type: KeyLabelOfPod,
		},
		{
			Name: "setpod2",
			Type: KeyLabelOfPod,
		},
		{
			Name: "setpod3",
			Type: KeyValueLabelOfPod,
		},
	}

	iMgr.CreateIPSets(setsTocreate)

	err := iMgr.AddToSets(setsTocreate, "10.0.0.1", "test")
	require.NoError(t, err)

	err = iMgr.AddToSets(setsTocreate, "10.0.0.2", "test1")
	require.NoError(t, err)

	err = iMgr.AddToSets([]*IPSetMetadata{setsTocreate[0], setsTocreate[2], setsTocreate[3]}, "10.0.0.3", "test3")
	require.NoError(t, err)

	ipsetList := map[string]struct{}{}
	for _, v := range setsTocreate {
		ipsetList[v.GetPrefixName()] = struct{}{}
	}
	ips, err := iMgr.GetIPsFromSelectorIPSets(ipsetList)
	require.NoError(t, err)

	assert.Equal(t, 2, len(ips))

	expectedintersection := map[string]struct{}{
		"10.0.0.1": {},
		"10.0.0.2": {},
	}

	assert.Equal(t, ips, expectedintersection)
}

func TestAddDeleteSelectorReferences(t *testing.T) {
	iMgr := NewIPSetManager(applyOnNeedCfg, common.NewMockIOShim([]testutils.TestCmd{}))
	setsTocreate := []*IPSetMetadata{
		{
			Name: "setNs1",
			Type: Namespace,
		},
		{
			Name: "setpod1",
			Type: KeyLabelOfPod,
		},
		{
			Name: "setpod2",
			Type: KeyLabelOfPod,
		},
		{
			Name: "setpod3",
			Type: NestedLabelOfPod,
		},
		{
			Name: "setpod4",
			Type: KeyLabelOfPod,
		},
	}
	networkPolicName := "testNetworkPolicy"

	for _, k := range setsTocreate {
		err := iMgr.AddReference(k.GetPrefixName(), networkPolicName, SelectorType)
		require.Error(t, err)
	}

	iMgr.CreateIPSets(setsTocreate)

	// Add setpod4 to setpod3
	err := iMgr.AddToLists([]*IPSetMetadata{setsTocreate[3]}, []*IPSetMetadata{setsTocreate[4]})
	require.NoError(t, err)

	for _, v := range setsTocreate {
		err = iMgr.AddReference(v.GetPrefixName(), networkPolicName, SelectorType)
		require.NoError(t, err)
	}

	assert.Equal(t, 5, len(iMgr.toAddOrUpdateCache))
	assert.Equal(t, 0, len(iMgr.toDeleteCache))

	for _, v := range setsTocreate {
		err = iMgr.DeleteReference(v.GetPrefixName(), networkPolicName, SelectorType)
		if err != nil {
			t.Errorf("DeleteReference failed with error %s", err.Error())
		}
	}

	assert.Equal(t, 0, len(iMgr.toAddOrUpdateCache))
	assert.Equal(t, 5, len(iMgr.toDeleteCache))

	for _, v := range setsTocreate {
		iMgr.DeleteIPSet(v.GetPrefixName())
	}

	// Above delete will not remove setpod3 and setpod4
	// because they are referencing each other
	assert.Equal(t, 2, len(iMgr.setMap))

	err = iMgr.RemoveFromList(setsTocreate[3], []*IPSetMetadata{setsTocreate[4]})
	require.NoError(t, err)

	for _, v := range setsTocreate {
		iMgr.DeleteIPSet(v.GetPrefixName())
	}

	for _, v := range setsTocreate {
		set := iMgr.GetIPSet(v.GetPrefixName())
		assert.Nil(t, set)
	}
}

func TestAddDeleteNetPolReferences(t *testing.T) {
	iMgr := NewIPSetManager(applyOnNeedCfg, common.NewMockIOShim([]testutils.TestCmd{}))
	setsTocreate := []*IPSetMetadata{
		{
			Name: "setNs1",
			Type: Namespace,
		},
		{
			Name: "setpod1",
			Type: KeyLabelOfPod,
		},
		{
			Name: "setpod2",
			Type: KeyLabelOfPod,
		},
		{
			Name: "setpod3",
			Type: NestedLabelOfPod,
		},
		{
			Name: "setpod4",
			Type: KeyLabelOfPod,
		},
	}
	networkPolicName := "testNetworkPolicy"

	iMgr.CreateIPSets(setsTocreate)
	err := iMgr.AddToLists([]*IPSetMetadata{setsTocreate[3]}, []*IPSetMetadata{setsTocreate[4]})
	require.NoError(t, err)

	for _, v := range setsTocreate {
		err = iMgr.AddReference(v.GetPrefixName(), networkPolicName, NetPolType)
		require.NoError(t, err)
	}

	assert.Equal(t, 5, len(iMgr.toAddOrUpdateCache))
	assert.Equal(t, 0, len(iMgr.toDeleteCache))
	for _, v := range setsTocreate {
		err = iMgr.DeleteReference(v.GetPrefixName(), networkPolicName, NetPolType)
		require.NoError(t, err)
	}

	assert.Equal(t, 0, len(iMgr.toAddOrUpdateCache))
	assert.Equal(t, 5, len(iMgr.toDeleteCache))

	for _, v := range setsTocreate {
		iMgr.DeleteIPSet(v.GetPrefixName())
	}

	// Above delete will not remove setpod3 and setpod4
	// because they are referencing each other
	assert.Equal(t, 2, len(iMgr.setMap))

	err = iMgr.RemoveFromList(setsTocreate[3], []*IPSetMetadata{setsTocreate[4]})
	require.NoError(t, err)

	for _, v := range setsTocreate {
		iMgr.DeleteIPSet(v.GetPrefixName())
	}

	for _, v := range setsTocreate {
		set := iMgr.GetIPSet(v.GetPrefixName())
		assert.Nil(t, set)
	}

	for _, v := range setsTocreate {
		err = iMgr.DeleteReference(v.GetPrefixName(), networkPolicName, NetPolType)
		require.Error(t, err)
	}
}

func TestMain(m *testing.M) {
	metrics.InitializeAll()

	exitCode := m.Run()

	os.Exit(exitCode)
}

func assertEqualCache(t *testing.T, iMgr *IPSetManager, cache cacheValues) {
	require.Equal(t, len(cache.mainCache), len(iMgr.setMap))
	for _, setName := range cache.mainCache {
		require.True(t, iMgr.exists(setName))
		set := iMgr.GetIPSet(setName)
		require.NotNil(t, set)
		assert.Equal(t, util.GetHashedName(setName), set.HashedName)
	}

	require.Equal(t, len(cache.addUpdateCacheMembers), len(iMgr.toAddOrUpdateCache))
	for setName, members := range cache.addUpdateCacheMembers {
		require.True(t, iMgr.exists(setName))
		for _, member := range members {
			set := iMgr.setMap[setName]
			if member.kind == isIP {
				_, ok := set.IPPodKey[member.value]
				require.True(t, ok)
			} else {
				_, ok := set.MemberIPSets[member.value]
				require.True(t, ok)
			}
		}
	}

	require.Equal(t, len(cache.toDeleteCache), len(iMgr.toDeleteCache))
	for _, setName := range cache.toDeleteCache {
		_, ok := iMgr.toDeleteCache[setName]
		require.True(t, ok)
	}
}
