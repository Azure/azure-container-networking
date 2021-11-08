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

var iMgrApplyOnNeedCfg = &IPSetManagerCfg{
	IPSetMode:   ApplyOnNeed,
	NetworkName: "azure",
}

func TestCreateIPSet(t *testing.T) {
	iMgr := NewIPSetManager(iMgrApplyOnNeedCfg, common.NewMockIOShim([]testutils.TestCmd{}))

	setMetadata := NewIPSetMetadata(testSetName, Namespace)
	iMgr.CreateIPSets([]*IPSetMetadata{setMetadata})
	// creating twice
	iMgr.CreateIPSets([]*IPSetMetadata{setMetadata})

	assert.True(t, iMgr.exists(setMetadata.GetPrefixName()))

	set := iMgr.GetIPSet(setMetadata.GetPrefixName())
	require.NotNil(t, set)
	assert.Equal(t, setMetadata.GetPrefixName(), set.Name)
	assert.Equal(t, util.GetHashedName(setMetadata.GetPrefixName()), set.HashedName)
}

func TestAddToSet(t *testing.T) {
	iMgr := NewIPSetManager(iMgrApplyOnNeedCfg, common.NewMockIOShim([]testutils.TestCmd{}))

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
	iMgr := NewIPSetManager(iMgrApplyOnNeedCfg, common.NewMockIOShim([]testutils.TestCmd{}))

	setMetadata := NewIPSetMetadata(testSetName, Namespace)
	iMgr.CreateIPSets([]*IPSetMetadata{setMetadata})
	err := iMgr.AddToSets([]*IPSetMetadata{setMetadata}, testPodIP, testPodKey)
	require.NoError(t, err)
	err = iMgr.RemoveFromSets([]*IPSetMetadata{setMetadata}, testPodIP, testPodKey)
	require.NoError(t, err)
}

func TestRemoveFromSetMissing(t *testing.T) {
	iMgr := NewIPSetManager(iMgrApplyOnNeedCfg, common.NewMockIOShim([]testutils.TestCmd{}))
	setMetadata := NewIPSetMetadata(testSetName, Namespace)
	err := iMgr.RemoveFromSets([]*IPSetMetadata{setMetadata}, testPodIP, testPodKey)
	require.NoError(t, err)
}

func TestAddToListMissing(t *testing.T) {
	iMgr := NewIPSetManager(iMgrApplyOnNeedCfg, common.NewMockIOShim([]testutils.TestCmd{}))
	setMetadata := NewIPSetMetadata(testSetName, Namespace)
	listMetadata := NewIPSetMetadata("testlabel", KeyLabelOfNamespace)
	err := iMgr.AddToLists([]*IPSetMetadata{listMetadata}, []*IPSetMetadata{setMetadata})
	require.NoError(t, err)
}

func TestAddToList(t *testing.T) {
	iMgr := NewIPSetManager(iMgrApplyOnNeedCfg, common.NewMockIOShim([]testutils.TestCmd{}))
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
	iMgr := NewIPSetManager(iMgrApplyOnNeedCfg, common.NewMockIOShim([]testutils.TestCmd{}))
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
	iMgr := NewIPSetManager(iMgrApplyOnNeedCfg, common.NewMockIOShim([]testutils.TestCmd{}))

	setMetadata := NewIPSetMetadata(testSetName, Namespace)
	listMetadata := NewIPSetMetadata(testListName, KeyLabelOfNamespace)
	iMgr.CreateIPSets([]*IPSetMetadata{listMetadata})

	err := iMgr.RemoveFromList(listMetadata, []*IPSetMetadata{setMetadata})
	require.NoError(t, err)
}

func TestDeleteIPSet(t *testing.T) {
	iMgr := NewIPSetManager(iMgrApplyOnNeedCfg, common.NewMockIOShim([]testutils.TestCmd{}))
	setMetadata := NewIPSetMetadata(testSetName, Namespace)
	iMgr.CreateIPSets([]*IPSetMetadata{setMetadata})

	iMgr.DeleteIPSet(setMetadata.GetPrefixName())
	// TODO add cache check
}

func TestGetIPsFromSelectorIPSets(t *testing.T) {
	iMgr := NewIPSetManager(iMgrApplyOnNeedCfg, common.NewMockIOShim([]testutils.TestCmd{}))
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
	iMgr := NewIPSetManager(iMgrApplyOnNeedCfg, common.NewMockIOShim([]testutils.TestCmd{}))
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
	iMgr := NewIPSetManager(iMgrApplyOnNeedCfg, common.NewMockIOShim([]testutils.TestCmd{}))
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
