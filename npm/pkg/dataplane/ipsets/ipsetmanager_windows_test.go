package ipsets

import (
	"testing"

	"github.com/Azure/azure-container-networking/common"
	testutils "github.com/Azure/azure-container-networking/test/utils"
	"github.com/stretchr/testify/require"
)

func TestAddToSetWindows(t *testing.T) {
	iMgr := NewIPSetManager(iMgrApplyAlwaysCfg, getMockIOShim([]testutils.TestCmd{}))

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

	err = iMgr.applyIPSets()
	require.NoError(t, err)
}

func TestDestroyNPMIPSets(t *testing.T) {
	calls := []testutils.TestCmd{} // TODO
	iMgr := NewIPSetManager(iMgrApplyAlwaysCfg, getMockIOShim(calls))
	require.NoError(t, iMgr.resetIPSets())
}

// create all possible SetTypes
func TestApplyCreationsAndAdds(t *testing.T) {
	hns := GetHNSFake()
	io := common.NewMockIOShimWithFakeHNS([]testutils.TestCmd{}, hns)
	iMgr := NewIPSetManager(iMgrApplyAlwaysCfg, io)

	iMgr.CreateIPSets([]*IPSetMetadata{TestNSSet.Metadata})
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.0", "a"))
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.1", "b"))
	iMgr.CreateIPSets([]*IPSetMetadata{TestKeyPodSet.Metadata})
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestKeyPodSet.Metadata}, "10.0.0.5", "c"))
	iMgr.CreateIPSets([]*IPSetMetadata{TestKVPodSet.Metadata})
	iMgr.CreateIPSets([]*IPSetMetadata{TestNamedportSet.Metadata})
	iMgr.CreateIPSets([]*IPSetMetadata{TestCIDRSet.Metadata})
	iMgr.CreateIPSets([]*IPSetMetadata{TestKeyNSList.Metadata})
	require.NoError(t, iMgr.AddToLists([]*IPSetMetadata{TestKeyNSList.Metadata}, []*IPSetMetadata{TestNSSet.Metadata, TestKeyPodSet.Metadata}))
	iMgr.CreateIPSets([]*IPSetMetadata{TestKVNSList.Metadata})
	require.NoError(t, iMgr.AddToLists([]*IPSetMetadata{TestKVNSList.Metadata}, []*IPSetMetadata{TestKVPodSet.Metadata}))
	iMgr.CreateIPSets([]*IPSetMetadata{TestNestedLabelList.Metadata})
	toAddOrUpdateSetNames := []string{
		TestNSSet.PrefixName,
		TestKeyPodSet.PrefixName,
		TestKVPodSet.PrefixName,
		TestNamedportSet.PrefixName,
		TestCIDRSet.PrefixName,
		TestKeyNSList.PrefixName,
		TestKVNSList.PrefixName,
		TestNestedLabelList.PrefixName,
	}
	assertEqualContentsTestHelper(t, toAddOrUpdateSetNames, iMgr.toAddOrUpdateCache)
	err := iMgr.applyIPSets()
	require.NoError(t, err)

	for _, setName := range toAddOrUpdateSetNames {
		require.True(t, hns.Cache.SetPolicyExists(setName))
	}
}

func TestApplyDeletions(t *testing.T) {
	hns := GetHNSFake()
	io := common.NewMockIOShimWithFakeHNS([]testutils.TestCmd{}, hns)
	iMgr := NewIPSetManager(iMgrApplyAlwaysCfg, io)

	// Remove members and delete others
	iMgr.CreateIPSets([]*IPSetMetadata{TestNSSet.Metadata})
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.0", "a"))
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.1", "b"))
	iMgr.CreateIPSets([]*IPSetMetadata{TestKeyPodSet.Metadata})
	iMgr.CreateIPSets([]*IPSetMetadata{TestKeyNSList.Metadata})
	require.NoError(t, iMgr.AddToLists([]*IPSetMetadata{TestKeyNSList.Metadata}, []*IPSetMetadata{TestNSSet.Metadata, TestKeyPodSet.Metadata}))
	require.NoError(t, iMgr.RemoveFromSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.1", "b"))
	require.NoError(t, iMgr.RemoveFromList(TestKeyNSList.Metadata, []*IPSetMetadata{TestKeyPodSet.Metadata}))
	iMgr.CreateIPSets([]*IPSetMetadata{TestCIDRSet.Metadata})
	iMgr.DeleteIPSet(TestCIDRSet.PrefixName)
	iMgr.CreateIPSets([]*IPSetMetadata{TestNestedLabelList.Metadata})
	iMgr.DeleteIPSet(TestNestedLabelList.PrefixName)

	toDeleteSetNames := []string{TestCIDRSet.PrefixName, TestNestedLabelList.PrefixName}
	assertEqualContentsTestHelper(t, toDeleteSetNames, iMgr.toDeleteCache)
	toAddOrUpdateSetNames := []string{TestNSSet.PrefixName, TestKeyPodSet.PrefixName, TestKeyNSList.PrefixName}
	assertEqualContentsTestHelper(t, toAddOrUpdateSetNames, iMgr.toAddOrUpdateCache)

	err := iMgr.applyIPSets()
	require.NoError(t, err)

	for _, setName := range toDeleteSetNames {
		require.False(t, hns.Cache.SetPolicyExists(setName))
	}

	for _, setName := range toAddOrUpdateSetNames {
		require.True(t, hns.Cache.SetPolicyExists(setName))
	}
}

// TODO test that a reconcile list is updated
func TestFailureOnCreation(t *testing.T) {
	hns := GetHNSFake()
	io := common.NewMockIOShimWithFakeHNS([]testutils.TestCmd{}, hns)
	iMgr := NewIPSetManager(iMgrApplyAlwaysCfg, io)

	iMgr.CreateIPSets([]*IPSetMetadata{TestNSSet.Metadata})
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.0", "a"))
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.1", "b"))
	iMgr.CreateIPSets([]*IPSetMetadata{TestKeyPodSet.Metadata})
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestKeyPodSet.Metadata}, "10.0.0.5", "c"))
	iMgr.CreateIPSets([]*IPSetMetadata{TestCIDRSet.Metadata})
	iMgr.DeleteIPSet(TestCIDRSet.PrefixName)

	toAddOrUpdateSetNames := []string{TestNSSet.PrefixName, TestKeyPodSet.PrefixName}
	assertEqualContentsTestHelper(t, toAddOrUpdateSetNames, iMgr.toAddOrUpdateCache)
	toDeleteSetNames := []string{TestCIDRSet.PrefixName}
	assertEqualContentsTestHelper(t, toDeleteSetNames, iMgr.toDeleteCache)

	for _, setName := range toDeleteSetNames {
		require.False(t, hns.Cache.SetPolicyExists(setName))
	}

	for _, setName := range toAddOrUpdateSetNames {
		require.True(t, hns.Cache.SetPolicyExists(setName))
	}
}

// TODO test that a reconcile list is updated
func TestFailureOnAddToList(t *testing.T) {
	// This exact scenario wouldn't occur. This error happens when the cache is out of date with the kernel.
	hns := GetHNSFake()
	io := common.NewMockIOShimWithFakeHNS([]testutils.TestCmd{}, hns)
	iMgr := NewIPSetManager(iMgrApplyAlwaysCfg, io)

	iMgr.CreateIPSets([]*IPSetMetadata{TestNSSet.Metadata})
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.0", "a"))
	iMgr.CreateIPSets([]*IPSetMetadata{TestKeyPodSet.Metadata})
	iMgr.CreateIPSets([]*IPSetMetadata{TestKeyNSList.Metadata})
	require.NoError(t, iMgr.AddToLists([]*IPSetMetadata{TestKeyNSList.Metadata}, []*IPSetMetadata{TestNSSet.Metadata, TestKeyPodSet.Metadata}))
	iMgr.CreateIPSets([]*IPSetMetadata{TestKVNSList.Metadata})
	require.NoError(t, iMgr.AddToLists([]*IPSetMetadata{TestKVNSList.Metadata}, []*IPSetMetadata{TestNSSet.Metadata}))
	iMgr.CreateIPSets([]*IPSetMetadata{TestCIDRSet.Metadata})
	iMgr.DeleteIPSet(TestCIDRSet.PrefixName)

	toAddOrUpdateSetNames := []string{
		TestNSSet.PrefixName,
		TestKeyPodSet.PrefixName,
		TestKeyNSList.PrefixName,
		TestKVNSList.PrefixName,
	}
	assertEqualContentsTestHelper(t, toAddOrUpdateSetNames, iMgr.toAddOrUpdateCache)
	toDeleteSetNames := []string{TestCIDRSet.PrefixName}
	assertEqualContentsTestHelper(t, toDeleteSetNames, iMgr.toDeleteCache)

	for _, setName := range toDeleteSetNames {
		require.False(t, hns.Cache.SetPolicyExists(setName))
	}

	for _, setName := range toAddOrUpdateSetNames {
		require.True(t, hns.Cache.SetPolicyExists(setName))
	}
}

// TODO test that a reconcile list is updated
func TestFailureOnFlush(t *testing.T) {
	// This exact scenario wouldn't occur. This error happens when the cache is out of date with the kernel.
	hns := GetHNSFake()
	io := common.NewMockIOShimWithFakeHNS([]testutils.TestCmd{}, hns)
	iMgr := NewIPSetManager(iMgrApplyAlwaysCfg, io)

	iMgr.CreateIPSets([]*IPSetMetadata{TestNSSet.Metadata})
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.0", "a"))
	iMgr.CreateIPSets([]*IPSetMetadata{TestKVPodSet.Metadata})
	iMgr.DeleteIPSet(TestKVPodSet.PrefixName)
	iMgr.CreateIPSets([]*IPSetMetadata{TestCIDRSet.Metadata})
	iMgr.DeleteIPSet(TestCIDRSet.PrefixName)

	toAddOrUpdateSetNames := []string{TestNSSet.PrefixName}
	assertEqualContentsTestHelper(t, toAddOrUpdateSetNames, iMgr.toAddOrUpdateCache)
	toDeleteSetNames := []string{TestKVPodSet.PrefixName, TestCIDRSet.PrefixName}
	assertEqualContentsTestHelper(t, toDeleteSetNames, iMgr.toDeleteCache)

	for _, setName := range toDeleteSetNames {
		require.False(t, hns.Cache.SetPolicyExists(setName))
	}

	for _, setName := range toAddOrUpdateSetNames {
		require.True(t, hns.Cache.SetPolicyExists(setName))
	}
}

// TODO test that a reconcile list is updated
func TestFailureOnDeletion(t *testing.T) {
	hns := GetHNSFake()
	io := common.NewMockIOShimWithFakeHNS([]testutils.TestCmd{}, hns)
	iMgr := NewIPSetManager(iMgrApplyAlwaysCfg, io)

	iMgr.CreateIPSets([]*IPSetMetadata{TestNSSet.Metadata})
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.0", "a"))
	iMgr.CreateIPSets([]*IPSetMetadata{TestKVPodSet.Metadata})
	iMgr.DeleteIPSet(TestKVPodSet.PrefixName)
	iMgr.CreateIPSets([]*IPSetMetadata{TestCIDRSet.Metadata})
	iMgr.DeleteIPSet(TestCIDRSet.PrefixName)

	toAddOrUpdateSetNames := []string{TestNSSet.PrefixName}
	assertEqualContentsTestHelper(t, toAddOrUpdateSetNames, iMgr.toAddOrUpdateCache)
	toDeleteSetNames := []string{TestKVPodSet.PrefixName, TestCIDRSet.PrefixName}
	assertEqualContentsTestHelper(t, toDeleteSetNames, iMgr.toDeleteCache)

	for _, setName := range toDeleteSetNames {
		require.False(t, hns.Cache.SetPolicyExists(setName))
	}

	for _, setName := range toAddOrUpdateSetNames {
		require.True(t, hns.Cache.SetPolicyExists(setName))
	}
}
