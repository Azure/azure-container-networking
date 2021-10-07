package ipsets

import (
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testSetName  = "test-set"
	testListName = "test-list"
	testPodKey   = "test-pod-key"
	testPodIP    = "10.0.0.0"
)

func TestCreateIPSet(t *testing.T) {
	iMgr := NewIPSetManager("azure")

	setMetaData := NewIPSetMetadata(testSetName, NameSpace)
	iMgr.CreateIPSet(setMetaData)
	// creating twice
	iMgr.CreateIPSet(setMetaData)

	assert.True(t, iMgr.exists(setMetaData.GetPrefixName()))

	set := iMgr.GetIPSet(setMetaData.GetPrefixName())
	require.NotNil(t, set)
	assert.Equal(t, setMetaData.GetPrefixName(), set.Name)
	assert.Equal(t, util.GetHashedName(setMetaData.GetPrefixName()), set.HashedName)
}

func TestAddToSet(t *testing.T) {
	iMgr := NewIPSetManager("azure")

	setMetaData := NewIPSetMetadata(testSetName, NameSpace)
	iMgr.CreateIPSet(setMetaData)

	err := iMgr.AddToSet([]*IPSetMetaData{setMetaData}, testPodIP, testPodKey)
	require.NoError(t, err)

	err = iMgr.AddToSet([]*IPSetMetaData{setMetaData}, "2001:db8:0:0:0:0:2:1", "newpod")
	require.Error(t, err)

	// same IP changed podkey
	err = iMgr.AddToSet([]*IPSetMetaData{setMetaData}, testPodIP, "newpod")
	require.NoError(t, err)

	listMetaData := NewIPSetMetadata("testipsetlist", KeyLabelOfNameSpace)
	iMgr.CreateIPSet(listMetaData)
	err = iMgr.AddToSet([]*IPSetMetaData{listMetaData}, testPodIP, testPodKey)
	require.Error(t, err)
}

func TestRemoveFromSet(t *testing.T) {
	iMgr := NewIPSetManager("azure")

	setMetaData := NewIPSetMetadata(testSetName, NameSpace)
	iMgr.CreateIPSet(setMetaData)
	err := iMgr.AddToSet([]*IPSetMetaData{setMetaData}, testPodIP, testPodKey)
	require.NoError(t, err)
	err = iMgr.RemoveFromSet([]*IPSetMetaData{setMetaData}, testPodIP, testPodKey)
	require.NoError(t, err)
}

func TestRemoveFromSetMissing(t *testing.T) {
	iMgr := NewIPSetManager("azure")
	setMetaData := NewIPSetMetadata(testSetName, NameSpace)
	err := iMgr.RemoveFromSet([]*IPSetMetaData{setMetaData}, testPodIP, testPodKey)
	require.Error(t, err)
}

func TestAddToListMissing(t *testing.T) {
	iMgr := NewIPSetManager("azure")
	setMetaData := NewIPSetMetadata(testSetName, NameSpace)
	listMetaData := NewIPSetMetadata("testlabel", KeyLabelOfNameSpace)
	err := iMgr.AddToList(listMetaData, []*IPSetMetaData{setMetaData})
	require.Error(t, err)
}

func TestAddToList(t *testing.T) {
	iMgr := NewIPSetManager("azure")
	setMetaData := NewIPSetMetadata(testSetName, NameSpace)
	listMetaData := NewIPSetMetadata(testListName, KeyLabelOfNameSpace)
	iMgr.CreateIPSet(setMetaData)
	iMgr.CreateIPSet(listMetaData)

	err := iMgr.AddToList(listMetaData, []*IPSetMetaData{setMetaData})
	require.NoError(t, err)

	set := iMgr.GetIPSet(listMetaData.GetPrefixName())
	assert.NotNil(t, set)
	assert.Equal(t, listMetaData.GetPrefixName(), set.Name)
	assert.Equal(t, util.GetHashedName(listMetaData.GetPrefixName()), set.HashedName)
	assert.Equal(t, 1, len(set.MemberIPSets))
	assert.Equal(t, setMetaData.GetPrefixName(), set.MemberIPSets[setMetaData.GetPrefixName()].Name)
}

func TestRemoveFromList(t *testing.T) {
	iMgr := NewIPSetManager("azure")
	setMetaData := NewIPSetMetadata(testSetName, NameSpace)
	listMetaData := NewIPSetMetadata(testListName, KeyLabelOfNameSpace)
	iMgr.CreateIPSet(setMetaData)
	iMgr.CreateIPSet(listMetaData)

	err := iMgr.AddToList(listMetaData, []*IPSetMetaData{setMetaData})
	require.NoError(t, err)

	set := iMgr.GetIPSet(listMetaData.GetPrefixName())
	assert.NotNil(t, set)
	assert.Equal(t, listMetaData.GetPrefixName(), set.Name)
	assert.Equal(t, util.GetHashedName(listMetaData.GetPrefixName()), set.HashedName)
	assert.Equal(t, 1, len(set.MemberIPSets))
	assert.Equal(t, setMetaData.GetPrefixName(), set.MemberIPSets[setMetaData.GetPrefixName()].Name)

	err = iMgr.RemoveFromList(listMetaData, []*IPSetMetaData{setMetaData})
	require.NoError(t, err)

	set = iMgr.GetIPSet(listMetaData.GetPrefixName())
	assert.NotNil(t, set)
	assert.Equal(t, 0, len(set.MemberIPSets))
}

func TestRemoveFromListMissing(t *testing.T) {
	iMgr := NewIPSetManager("azure")

	setMetaData := NewIPSetMetadata(testSetName, NameSpace)
	listMetaData := NewIPSetMetadata(testListName, KeyLabelOfNameSpace)
	iMgr.CreateIPSet(listMetaData)

	err := iMgr.RemoveFromList(listMetaData, []*IPSetMetaData{setMetaData})
	require.Error(t, err)
}

func TestDeleteIPSet(t *testing.T) {
	iMgr := NewIPSetManager("azure")
	setMetaData := NewIPSetMetadata(testSetName, NameSpace)
	iMgr.CreateIPSet(setMetaData)

	iMgr.DeleteIPSet(setMetaData.GetPrefixName())
	// TODO add cache check
}

func TestGetIPsFromSelectorIPSets(t *testing.T) {
	iMgr := NewIPSetManager("azure")
	setsTocreate := []*IPSetMetaData{
		{
			Name: "setNs1",
			Type: NameSpace,
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

	for _, v := range setsTocreate {
		iMgr.CreateIPSet(v)
	}

	err := iMgr.AddToSet(setsTocreate, "10.0.0.1", "test")
	require.NoError(t, err)

	err = iMgr.AddToSet(setsTocreate, "10.0.0.2", "test1")
	require.NoError(t, err)

	err = iMgr.AddToSet([]*IPSetMetaData{setsTocreate[0], setsTocreate[2], setsTocreate[3]}, "10.0.0.3", "test3")
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
	iMgr := NewIPSetManager("azure")
	setsTocreate := []*IPSetMetaData{
		{
			Name: "setNs1",
			Type: NameSpace,
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
	for _, v := range setsTocreate {
		iMgr.CreateIPSet(v)
	}
	// Add setpod4 to setpod3
	err := iMgr.AddToList(setsTocreate[3], []*IPSetMetaData{setsTocreate[4]})
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

	err = iMgr.RemoveFromList(setsTocreate[3], []*IPSetMetaData{setsTocreate[4]})
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
	iMgr := NewIPSetManager("azure")
	setsTocreate := []*IPSetMetaData{
		{
			Name: "setNs1",
			Type: NameSpace,
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
	for _, v := range setsTocreate {
		iMgr.CreateIPSet(v)
	}
	err := iMgr.AddToList(setsTocreate[3], []*IPSetMetaData{setsTocreate[4]})
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

	err = iMgr.RemoveFromList(setsTocreate[3], []*IPSetMetaData{setsTocreate[4]})
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
