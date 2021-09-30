package ipsets

import (
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/npm/metrics"
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

func TestCreateIPSet(t *testing.T) {
	iMgr := NewIPSetManager("azure", common.NewMockIOShim([]testutils.TestCmd{}))

	err := iMgr.CreateIPSet(testSetName, Namespace)
	if err != nil {
		t.Errorf("CreateIPSet() returned error %s", err.Error())
	}
}

func TestAddToSet(t *testing.T) {
	iMgr := NewIPSetManager("azure", common.NewMockIOShim([]testutils.TestCmd{}))

	err := iMgr.CreateIPSet(testSetName, Namespace)
	if err != nil {
		t.Errorf("CreateIPSet() returned error %s", err.Error())
	}

	err := iMgr.AddToSet([]*IPSetMetadata{setMetadata}, testPodIP, testPodKey)
	require.NoError(t, err)

	err = iMgr.AddToSet([]*IPSetMetadata{setMetadata}, "2001:db8:0:0:0:0:2:1", "newpod")
	require.Error(t, err)

	// same IP changed podkey
	err = iMgr.AddToSet([]*IPSetMetadata{setMetadata}, testPodIP, "newpod")
	require.NoError(t, err)

	listMetadata := NewIPSetMetadata("testipsetlist", KeyLabelOfNameSpace)
	iMgr.CreateIPSet(listMetadata)
	err = iMgr.AddToSet([]*IPSetMetadata{listMetadata}, testPodIP, testPodKey)
	require.Error(t, err)
}

func TestRemoveFromSet(t *testing.T) {
	iMgr := NewIPSetManager("azure", common.NewMockIOShim([]testutils.TestCmd{}))

	err := iMgr.CreateIPSet(testSetName, Namespace)
	if err != nil {
		t.Errorf("CreateIPSet() returned error %s", err.Error())
	}
	err = iMgr.AddToSet([]string{testSetName}, testPodIP, testPodKey)
	if err != nil {
		t.Errorf("RemoveFromSet() returned error %s", err.Error())
	}
	err = iMgr.RemoveFromSet([]string{testSetName}, testPodIP, testPodKey)
	if err != nil {
		t.Errorf("RemoveFromSet() returned error %s", err.Error())
	}
}

func TestRemoveFromSetMissing(t *testing.T) {
	iMgr := NewIPSetManager("azure", common.NewMockIOShim([]testutils.TestCmd{}))
	setMetadata := NewIPSetMetadata(testSetName, NameSpace)
	err := iMgr.RemoveFromSet([]*IPSetMetadata{setMetadata}, testPodIP, testPodKey)
	require.Error(t, err)
}

func TestAddToListMissing(t *testing.T) {
	iMgr := NewIPSetManager("azure", common.NewMockIOShim([]testutils.TestCmd{}))
	setMetadata := NewIPSetMetadata(testSetName, NameSpace)
	listMetadata := NewIPSetMetadata("testlabel", KeyLabelOfNameSpace)
	err := iMgr.AddToList(listMetadata, []*IPSetMetadata{setMetadata})
	require.Error(t, err)
}

func TestAddToList(t *testing.T) {
	iMgr := NewIPSetManager()
	err := iMgr.CreateIPSet(testSetName, Namespace)
	if err != nil {
		t.Errorf("CreateIPSet() returned error %s", err.Error())
	}

	err := iMgr.AddToSet(setsTocreate, "10.0.0.1", "test")
	require.NoError(t, err)

	err = iMgr.AddToSet(setsTocreate, "10.0.0.2", "test1")
	require.NoError(t, err)

	err = iMgr.AddToSet([]*IPSetMetadata{setsTocreate[0], setsTocreate[2], setsTocreate[3]}, "10.0.0.3", "test3")
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

func TestRemoveFromList(t *testing.T) {
	iMgr := NewIPSetManager()
	err := iMgr.CreateIPSet(testSetName, Namespace)
	if err != nil {
		t.Errorf("CreateIPSet() returned error %s", err.Error())
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
	err := iMgr.AddToList(setsTocreate[3], []*IPSetMetadata{setsTocreate[4]})
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

func TestDeleteIPSet(t *testing.T) {
	iMgr := NewIPSetManager()
	err := iMgr.CreateIPSet(testSetName, Namespace)
	if err != nil {
		t.Errorf("CreateIPSet() returned error %s", err.Error())
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
