package ipsets

import (
	"fmt"
	"testing"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/network/hnswrapper"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/util"
	testutils "github.com/Azure/azure-container-networking/test/utils"
	"github.com/Microsoft/hcsshim/hcn"
	"github.com/stretchr/testify/require"
)

type winPromVals struct {
	getNetworkLatencyCalls   int
	getNetworkFailures       int
	createIPSetLatencyCalls  int
	createIPSetFailures      int
	createNestedLatencyCalls int
	createNestedFailures     int
	updateIPSetLatencyCalls  int
	updateIPSetFailures      int
	updateNestedLatencyCalls int
	updateNestedFailures     int
	deleteIPSetLatencyCalls  int
	deleteIPSetFailures      int
	deleteNestedLatencyCalls int
	deleteNestedFailures     int
}

func (w winPromVals) test(t *testing.T) {
	t.Helper()

	count, err := metrics.TotalGetNetworkLatencyCalls()
	require.NoError(t, err, "failed to get metric")
	require.Equal(t, w.getNetworkLatencyCalls, count, "incorrect list network latency calls")

	count, err = metrics.TotalGetNetworkFailures()
	require.NoError(t, err, "failed to get metric")
	require.Equal(t, w.getNetworkFailures, count, "incorrect list network failures")

	count, err = metrics.TotalSetPolicyLatencyCalls(metrics.CreateOp, false)
	require.NoError(t, err, "failed to get metric")
	require.Equal(t, w.createIPSetLatencyCalls, count, "incorrect create ipset latency calls")

	count, err = metrics.TotalSetPolicyFailures(metrics.CreateOp, false)
	require.NoError(t, err, "failed to get metric")
	require.Equal(t, w.createIPSetFailures, count, "incorrect create ipset failures")

	count, err = metrics.TotalSetPolicyLatencyCalls(metrics.CreateOp, true)
	require.NoError(t, err, "failed to get metric")
	require.Equal(t, w.createNestedLatencyCalls, count, "incorrect create nested ipset latency calls")

	count, err = metrics.TotalSetPolicyFailures(metrics.CreateOp, true)
	require.NoError(t, err, "failed to get metric")
	require.Equal(t, w.createNestedFailures, count, "incorrect create nested ipset failures")

	count, err = metrics.TotalSetPolicyLatencyCalls(metrics.UpdateOp, false)
	require.NoError(t, err, "failed to get metric")
	require.Equal(t, w.updateIPSetLatencyCalls, count, "incorrect update ipset latency calls")

	count, err = metrics.TotalSetPolicyFailures(metrics.UpdateOp, false)
	require.NoError(t, err, "failed to get metric")
	require.Equal(t, w.updateIPSetFailures, count, "incorrect update ipset failures")

	count, err = metrics.TotalSetPolicyLatencyCalls(metrics.UpdateOp, true)
	require.NoError(t, err, "failed to get metric")
	require.Equal(t, w.updateNestedLatencyCalls, count, "incorrect update nested ipset latency calls")

	count, err = metrics.TotalSetPolicyFailures(metrics.UpdateOp, true)
	require.NoError(t, err, "failed to get metric")
	require.Equal(t, w.updateNestedFailures, count, "incorrect update nested ipset failures")

	count, err = metrics.TotalSetPolicyLatencyCalls(metrics.DeleteOp, false)
	require.NoError(t, err, "failed to get metric")
	require.Equal(t, w.deleteIPSetLatencyCalls, count, "incorrect delete ipset latency calls")

	count, err = metrics.TotalSetPolicyFailures(metrics.DeleteOp, false)
	require.NoError(t, err, "failed to get metric")
	require.Equal(t, w.deleteIPSetFailures, count, "incorrect delete ipset failures")

	count, err = metrics.TotalSetPolicyLatencyCalls(metrics.DeleteOp, true)
	require.NoError(t, err, "failed to get metric")
	require.Equal(t, w.deleteNestedLatencyCalls, count, "incorrect delete nested ipset latency calls")

	count, err = metrics.TotalSetPolicyFailures(metrics.DeleteOp, true)
	require.NoError(t, err, "failed to get metric")
	require.Equal(t, w.deleteNestedFailures, count, "incorrect delete nested ipset failures")
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

	err := iMgr.AddToSets(setsTocreate, "10.0.0.1", "test1")
	require.NoError(t, err)

	err = iMgr.AddToSets(setsTocreate, "10.0.0.2", "test2")
	require.NoError(t, err)

	err = iMgr.AddToSets([]*IPSetMetadata{setsTocreate[0], setsTocreate[2], setsTocreate[3]}, "10.0.0.3", "test3")
	require.NoError(t, err)

	kvl := []*IPSetMetadata{NewIPSetMetadata("kvl-1", KeyValueLabelOfPod)}
	require.NoError(t, iMgr.AddToLists([]*IPSetMetadata{nestedPodLabelList}, kvl))
	require.NoError(t, iMgr.AddToSets(kvl, "10.0.0.1", "test1"))
	require.NoError(t, iMgr.AddToSets(kvl, "10.0.0.2", "test2"))

	ipsetList := map[string]struct{}{}
	for _, v := range setsTocreate {
		ipsetList[v.GetPrefixName()] = struct{}{}
	}
	ipsetList[nestedPodLabelList.GetPrefixName()] = struct{}{}
	ips, err := iMgr.GetIPsFromSelectorIPSets(ipsetList)
	require.NoError(t, err)

	require.Equal(t, 2, len(ips))

	expectedintersection := map[string]string{
		"10.0.0.1": "test1",
		"10.0.0.2": "test2",
	}

	require.Equal(t, expectedintersection, ips)
}

func TestAddToSetWindows(t *testing.T) {
	metrics.InitializeWindowsMetrics()

	hns := GetHNSFake(t, "azure")
	io := common.NewMockIOShimWithFakeHNS(hns)
	iMgr := NewIPSetManager(applyAlwaysCfg, io)

	setMetadata := NewIPSetMetadata(testSetName, Namespace)
	iMgr.CreateIPSets([]*IPSetMetadata{setMetadata})

	err := iMgr.AddToSets([]*IPSetMetadata{setMetadata}, testPodIP, testPodKey)
	require.NoError(t, err)

	err = iMgr.AddToSets([]*IPSetMetadata{setMetadata}, "2001:db8:0:0:0:0:2:1", "newpod")
	require.Error(t, err)

	// same IP changed podkey
	err = iMgr.AddToSets([]*IPSetMetadata{setMetadata}, testPodIP, "newpod")
	require.NoError(t, err)

	listMetadata := NewIPSetMetadata("testipsetlist", KeyLabelOfNamespace)
	iMgr.CreateIPSets([]*IPSetMetadata{listMetadata})
	err = iMgr.AddToSets([]*IPSetMetadata{listMetadata}, testPodIP, testPodKey)
	require.Error(t, err)

	err = iMgr.ApplyIPSets()
	require.NoError(t, err)

	winPromVals{
		getNetworkLatencyCalls:   1,
		createIPSetLatencyCalls:  1,
		createNestedLatencyCalls: 1,
	}.test(t)
}

func TestDestroyNPMIPSets(t *testing.T) {
	metrics.InitializeWindowsMetrics()

	hns := GetHNSFake(t, "azure")
	io := common.NewMockIOShimWithFakeHNS(hns)
	iMgr := NewIPSetManager(applyAlwaysCfg, io)

	require.NoError(t, iMgr.resetIPSets())
	winPromVals{
		getNetworkLatencyCalls: 1,
	}.test(t)

	setMetadata := NewIPSetMetadata(testSetName, Namespace)
	iMgr.CreateIPSets([]*IPSetMetadata{setMetadata})

	err := iMgr.AddToSets([]*IPSetMetadata{setMetadata}, testPodIP, testPodKey)
	require.NoError(t, err)

	err = iMgr.ApplyIPSets()
	require.NoError(t, err)

	winPromVals{
		getNetworkLatencyCalls:  2,
		createIPSetLatencyCalls: 1,
	}.test(t)

	require.NoError(t, iMgr.resetIPSets())
	winPromVals{
		getNetworkLatencyCalls:  3,
		createIPSetLatencyCalls: 1,
		deleteIPSetLatencyCalls: 1,
	}.test(t)
}

// create all possible SetTypes
// FIXME because this can flake, commenting this out until we refactor with new windows testing framework
// func TestApplyCreationsAndAdds(t *testing.T) {
// 	hns := GetHNSFake(t, "azure")
// 	io := common.NewMockIOShimWithFakeHNS(hns)
// 	iMgr := NewIPSetManager(applyAlwaysCfg, io)

// 	iMgr.CreateIPSets([]*IPSetMetadata{TestNSSet.Metadata})
// 	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.0", "a"))
// 	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.1", "b"))
// 	iMgr.CreateIPSets([]*IPSetMetadata{TestKeyPodSet.Metadata})
// 	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestKeyPodSet.Metadata}, "10.0.0.5", "c"))
// 	iMgr.CreateIPSets([]*IPSetMetadata{TestKVPodSet.Metadata})
// 	iMgr.CreateIPSets([]*IPSetMetadata{TestNamedportSet.Metadata})
// 	iMgr.CreateIPSets([]*IPSetMetadata{TestCIDRSet.Metadata})
// 	iMgr.CreateIPSets([]*IPSetMetadata{TestKeyNSList.Metadata})
// 	require.NoError(t, iMgr.AddToLists([]*IPSetMetadata{TestKeyNSList.Metadata}, []*IPSetMetadata{TestNSSet.Metadata, TestKeyPodSet.Metadata}))
// 	iMgr.CreateIPSets([]*IPSetMetadata{TestKVNSList.Metadata})
// 	require.NoError(t, iMgr.AddToLists([]*IPSetMetadata{TestKVNSList.Metadata}, []*IPSetMetadata{TestKVPodSet.Metadata}))
// 	iMgr.CreateIPSets([]*IPSetMetadata{TestNestedLabelList.Metadata})
// 	toAddOrUpdateSetMap := map[string]hcn.SetPolicySetting{
// 		TestKeyPodSet.PrefixName: {
// 			Id:         TestKeyPodSet.HashedName,
// 			PolicyType: hcn.SetPolicyTypeIpSet,
// 			Name:       TestKeyPodSet.PrefixName,
// 			Values:     "10.0.0.5",
// 		},
// 		TestKVPodSet.PrefixName: {
// 			Id:         TestKVPodSet.HashedName,
// 			PolicyType: hcn.SetPolicyTypeIpSet,
// 			Name:       TestKVPodSet.PrefixName,
// 			Values:     "",
// 		},
// 		TestNamedportSet.PrefixName: {
// 			Id:         TestNamedportSet.HashedName,
// 			PolicyType: hcn.SetPolicyTypeIpSet,
// 			Name:       TestNamedportSet.PrefixName,
// 			Values:     "",
// 		},
// 		TestCIDRSet.PrefixName: {
// 			Id:         TestCIDRSet.HashedName,
// 			PolicyType: hcn.SetPolicyTypeIpSet,
// 			Name:       TestCIDRSet.PrefixName,
// 			Values:     "",
// 		},
// 		TestKeyNSList.PrefixName: {
// 			Id:         TestKeyNSList.HashedName,
// 			PolicyType: SetPolicyTypeNestedIPSet,
// 			Name:       TestKeyNSList.PrefixName,
// 			Values:     fmt.Sprintf("%s,%s", TestNSSet.HashedName, TestKeyPodSet.HashedName),
// 		},
// 		TestKVNSList.PrefixName: {
// 			Id:         TestKVNSList.HashedName,
// 			PolicyType: SetPolicyTypeNestedIPSet,
// 			Name:       TestKVNSList.PrefixName,
// 			Values:     TestKVPodSet.HashedName,
// 		},
// 		TestNestedLabelList.PrefixName: {
// 			Id:         TestNestedLabelList.HashedName,
// 			PolicyType: SetPolicyTypeNestedIPSet,
// 			Name:       TestNestedLabelList.PrefixName,
// 			Values:     "",
// 		},
// 	}
// 	err := iMgr.ApplyIPSets()
// 	require.NoError(t, err)
// 	verifyHNSCache(t, toAddOrUpdateSetMap, hns)

// 	// TODO change to use testutils instead
// 	// requires refactoring IPSEtmetadata, SetType, and SetKind to different types package
// 	// dptestutils.VerifySetPolicies(t, []*hcn.SetPolicySetting{
// 	// 	dptestutils.SetPolicy(TestNSSet.Metadata, "10.0.0.0", "10.0.0.1"),
// 	// 	dptestutils.SetPolicy(TestKeyPodSet.Metadata, "10.0.0.5"),
// 	// 	dptestutils.SetPolicy(TestKVPodSet.Metadata),
// 	// 	dptestutils.SetPolicy(TestNamedportSet.Metadata),
// 	// 	dptestutils.SetPolicy(TestCIDRSet.Metadata),
// 	// 	dptestutils.SetPolicy(TestKeyNSList.Metadata, TestNSSet.HashedName, TestKeyPodSet.HashedName),
// 	// 	dptestutils.SetPolicy(TestKVNSList.Metadata, TestKVPodSet.HashedName),
// 	// 	dptestutils.SetPolicy(TestNestedLabelList.Metadata),
// 	// })
// }

func TestApplyDeletions(t *testing.T) {
	metrics.InitializeWindowsMetrics()

	hns := GetHNSFake(t, "azure")
	io := common.NewMockIOShimWithFakeHNS(hns)
	iMgr := NewIPSetManager(applyAlwaysCfg, io)

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
	iMgr.DeleteIPSet(TestCIDRSet.PrefixName, util.SoftDelete)
	iMgr.CreateIPSets([]*IPSetMetadata{TestNestedLabelList.Metadata})
	iMgr.DeleteIPSet(TestNestedLabelList.PrefixName, util.SoftDelete)

	toDeleteSetNames := []string{TestCIDRSet.PrefixName, TestNestedLabelList.PrefixName}
	toAddOrUpdateSetMap := map[string]hcn.SetPolicySetting{
		TestNSSet.PrefixName: {
			Id:         TestNSSet.HashedName,
			PolicyType: hcn.SetPolicyTypeIpSet,
			Name:       TestNSSet.PrefixName,
			Values:     "10.0.0.0",
		},
		TestKeyPodSet.PrefixName: {
			Id:         TestKeyPodSet.HashedName,
			PolicyType: hcn.SetPolicyTypeIpSet,
			Name:       TestKeyPodSet.PrefixName,
			Values:     "",
		},
		TestKeyNSList.PrefixName: {
			Id:         TestKeyNSList.HashedName,
			PolicyType: SetPolicyTypeNestedIPSet,
			Name:       TestKeyNSList.PrefixName,
			Values:     TestNSSet.HashedName,
		},
	}

	err := iMgr.ApplyIPSets()
	require.NoError(t, err)
	verifyHNSCache(t, toAddOrUpdateSetMap, hns)
	verifyDeletedHNSCache(t, toDeleteSetNames, hns)

	winPromVals{
		getNetworkLatencyCalls:   1,
		createIPSetLatencyCalls:  1,
		createNestedLatencyCalls: 1,
	}.test(t)
}

// TODO test that a reconcile list is updated
func TestFailureOnCreation(t *testing.T) {
	metrics.InitializeWindowsMetrics()

	hns := GetHNSFake(t, "azure")
	io := common.NewMockIOShimWithFakeHNS(hns)
	iMgr := NewIPSetManager(applyAlwaysCfg, io)

	iMgr.CreateIPSets([]*IPSetMetadata{TestNSSet.Metadata})
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.0", "a"))
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.1", "b"))
	iMgr.CreateIPSets([]*IPSetMetadata{TestKeyPodSet.Metadata})
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestKeyPodSet.Metadata}, "10.0.0.5", "c"))
	iMgr.CreateIPSets([]*IPSetMetadata{TestCIDRSet.Metadata})
	iMgr.DeleteIPSet(TestCIDRSet.PrefixName, util.SoftDelete)

	toDeleteSetNames := []string{TestCIDRSet.PrefixName}
	toAddOrUpdateSetMap := map[string]hcn.SetPolicySetting{
		TestNSSet.PrefixName: {
			Id:         TestNSSet.HashedName,
			PolicyType: hcn.SetPolicyTypeIpSet,
			Name:       TestNSSet.PrefixName,
			Values:     "10.0.0.0,10.0.0.1",
		},
		TestKeyPodSet.PrefixName: {
			Id:         TestKeyPodSet.HashedName,
			PolicyType: hcn.SetPolicyTypeIpSet,
			Name:       TestKeyPodSet.PrefixName,
			Values:     "10.0.0.5",
		},
	}

	err := iMgr.ApplyIPSets()
	require.NoError(t, err)
	verifyHNSCache(t, toAddOrUpdateSetMap, hns)
	verifyDeletedHNSCache(t, toDeleteSetNames, hns)

	winPromVals{
		getNetworkLatencyCalls:  1,
		createIPSetLatencyCalls: 1,
	}.test(t)
}

// TODO test that a reconcile list is updated
// FIXME commenting this out until we refactor with new windows testing framework
// func TestFailureOnAddToList(t *testing.T) {
// 	// This exact scenario wouldn't occur. This error happens when the cache is out of date with the kernel.
// 	hns := GetHNSFake(t, "azure")
// 	io := common.NewMockIOShimWithFakeHNS(hns)
// 	iMgr := NewIPSetManager(applyAlwaysCfg, io)

// 	iMgr.CreateIPSets([]*IPSetMetadata{TestNSSet.Metadata})
// 	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.0", "a"))
// 	iMgr.CreateIPSets([]*IPSetMetadata{TestKeyPodSet.Metadata})
// 	iMgr.CreateIPSets([]*IPSetMetadata{TestKeyNSList.Metadata})
// 	require.NoError(t, iMgr.AddToLists([]*IPSetMetadata{TestKeyNSList.Metadata}, []*IPSetMetadata{TestNSSet.Metadata, TestKeyPodSet.Metadata}))
// 	iMgr.CreateIPSets([]*IPSetMetadata{TestKVNSList.Metadata})
// 	require.NoError(t, iMgr.AddToLists([]*IPSetMetadata{TestKVNSList.Metadata}, []*IPSetMetadata{TestNSSet.Metadata}))
// 	iMgr.CreateIPSets([]*IPSetMetadata{TestCIDRSet.Metadata})
// 	iMgr.DeleteIPSet(TestCIDRSet.PrefixName, util.SoftDelete)

// 	toDeleteSetNames := []string{TestCIDRSet.PrefixName}
// 	toAddOrUpdateSetMap := map[string]hcn.SetPolicySetting{
// 		TestNSSet.PrefixName: {
// 			Id:         TestNSSet.HashedName,
// 			PolicyType: hcn.SetPolicyTypeIpSet,
// 			Name:       TestNSSet.PrefixName,
// 			Values:     "10.0.0.0",
// 		},
// 		TestKeyPodSet.PrefixName: {
// 			Id:         TestKeyPodSet.HashedName,
// 			PolicyType: hcn.SetPolicyTypeIpSet,
// 			Name:       TestKeyPodSet.PrefixName,
// 			Values:     "",
// 		},
// 		TestKeyNSList.PrefixName: {
// 			Id:         TestKeyNSList.HashedName,
// 			PolicyType: SetPolicyTypeNestedIPSet,
// 			Name:       TestKeyNSList.PrefixName,
// 			Values:     fmt.Sprintf("%s,%s", TestNSSet.HashedName, TestKeyPodSet.HashedName),
// 		},
// 		TestKVNSList.PrefixName: {
// 			Id:         TestKVNSList.HashedName,
// 			PolicyType: SetPolicyTypeNestedIPSet,
// 			Name:       TestKVNSList.PrefixName,
// 			Values:     TestNSSet.HashedName,
// 		},
// 	}

// 	err := iMgr.ApplyIPSets()
// 	require.NoError(t, err)
// 	verifyHNSCache(t, toAddOrUpdateSetMap, hns)
// 	verifyDeletedHNSCache(t, toDeleteSetNames, hns)
// }

// TODO test that a reconcile list is updated
func TestFailureOnFlush(t *testing.T) {
	metrics.InitializeWindowsMetrics()

	// This exact scenario wouldn't occur. This error happens when the cache is out of date with the kernel.
	hns := GetHNSFake(t, "azure")
	io := common.NewMockIOShimWithFakeHNS(hns)
	iMgr := NewIPSetManager(applyAlwaysCfg, io)

	iMgr.CreateIPSets([]*IPSetMetadata{TestNSSet.Metadata})
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.0", "a"))
	iMgr.CreateIPSets([]*IPSetMetadata{TestKVPodSet.Metadata})
	iMgr.DeleteIPSet(TestKVPodSet.PrefixName, util.SoftDelete)
	iMgr.CreateIPSets([]*IPSetMetadata{TestCIDRSet.Metadata})
	iMgr.DeleteIPSet(TestCIDRSet.PrefixName, util.SoftDelete)

	toDeleteSetNames := []string{TestKVPodSet.PrefixName, TestCIDRSet.PrefixName}
	toAddOrUpdateSetMap := map[string]hcn.SetPolicySetting{
		TestNSSet.PrefixName: {
			Id:         TestNSSet.HashedName,
			PolicyType: hcn.SetPolicyTypeIpSet,
			Name:       TestNSSet.PrefixName,
			Values:     "10.0.0.0",
		},
	}

	err := iMgr.ApplyIPSets()
	require.NoError(t, err)
	verifyHNSCache(t, toAddOrUpdateSetMap, hns)
	verifyDeletedHNSCache(t, toDeleteSetNames, hns)

	winPromVals{
		getNetworkLatencyCalls:  1,
		createIPSetLatencyCalls: 1,
	}.test(t)
}

// TODO test that a reconcile list is updated
func TestFailureOnDeletion(t *testing.T) {
	metrics.InitializeWindowsMetrics()

	hns := GetHNSFake(t, "azure")
	io := common.NewMockIOShimWithFakeHNS(hns)
	iMgr := NewIPSetManager(applyAlwaysCfg, io)

	iMgr.CreateIPSets([]*IPSetMetadata{TestNSSet.Metadata})
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.0", "a"))
	iMgr.CreateIPSets([]*IPSetMetadata{TestKVPodSet.Metadata})
	iMgr.DeleteIPSet(TestKVPodSet.PrefixName, util.SoftDelete)
	iMgr.CreateIPSets([]*IPSetMetadata{TestCIDRSet.Metadata})
	iMgr.DeleteIPSet(TestCIDRSet.PrefixName, util.SoftDelete)

	toDeleteSetNames := []string{TestKVPodSet.PrefixName, TestCIDRSet.PrefixName}
	toAddOrUpdateSetMap := map[string]hcn.SetPolicySetting{
		TestNSSet.PrefixName: {
			Id:         TestNSSet.HashedName,
			PolicyType: hcn.SetPolicyTypeIpSet,
			Name:       TestNSSet.PrefixName,
			Values:     "10.0.0.0",
		},
	}

	err := iMgr.ApplyIPSets()
	require.NoError(t, err)
	verifyHNSCache(t, toAddOrUpdateSetMap, hns)
	verifyDeletedHNSCache(t, toDeleteSetNames, hns)

	winPromVals{
		getNetworkLatencyCalls:  1,
		createIPSetLatencyCalls: 1,
	}.test(t)
}

func TestMetrics(t *testing.T) {
	metrics.InitializeWindowsMetrics()
	metrics.ResetIPSetEntries()

	hns := GetHNSFake(t, "azure")
	io := common.NewMockIOShimWithFakeHNS(hns)
	iMgr := NewIPSetManager(applyAlwaysCfg, io)

	nsW := NewIPSetMetadata("w", Namespace)
	nsX := NewIPSetMetadata("x", Namespace)
	nsY := NewIPSetMetadata("y", Namespace)
	nsZ := NewIPSetMetadata("z", Namespace)
	nsList1 := NewIPSetMetadata("list1", KeyLabelOfNamespace)
	nsList2 := NewIPSetMetadata("list2", KeyLabelOfNamespace)
	nsList3 := NewIPSetMetadata("list3", KeyLabelOfNamespace)
	nsList4 := NewIPSetMetadata("list4", KeyLabelOfNamespace)

	iMgr.CreateIPSets([]*IPSetMetadata{nsW, nsX, nsY, nsList1, nsList2, nsList3})
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{nsW, nsX}, "1.2.3.4", "pod1"))
	require.NoError(t, iMgr.AddToLists([]*IPSetMetadata{nsList1, nsList2}, []*IPSetMetadata{nsW}))
	require.Nil(t, iMgr.ApplyIPSets())

	winPromVals{
		getNetworkLatencyCalls:   1,
		createIPSetLatencyCalls:  1,
		createNestedLatencyCalls: 1,
	}.test(t)

	iMgr.CreateIPSets([]*IPSetMetadata{nsZ, nsList4})
	require.Nil(t, iMgr.AddToSets([]*IPSetMetadata{nsY}, "5.6.7.8", "pod2"))
	require.Nil(t, iMgr.AddToLists([]*IPSetMetadata{nsList3}, []*IPSetMetadata{nsX}))
	require.Nil(t, iMgr.RemoveFromSets([]*IPSetMetadata{nsW, nsX}, "1.2.3.4", "pod1"))
	require.Nil(t, iMgr.RemoveFromList(nsList1, []*IPSetMetadata{nsW}))
	require.Nil(t, iMgr.RemoveFromList(nsList2, []*IPSetMetadata{nsW}))
	iMgr.DeleteIPSet(nsW.GetPrefixName(), util.SoftDelete)
	iMgr.DeleteIPSet(nsList1.GetPrefixName(), util.SoftDelete)
	require.Nil(t, iMgr.ApplyIPSets())

	winPromVals{
		getNetworkLatencyCalls:   2,
		createIPSetLatencyCalls:  2,
		createNestedLatencyCalls: 2,
		updateIPSetLatencyCalls:  1,
		updateNestedLatencyCalls: 1,
		deleteIPSetLatencyCalls:  1,
		deleteNestedLatencyCalls: 1,
	}.test(t)
}

func TestMetricsMaxMember(t *testing.T) {
	metrics.InitializeWindowsMetrics()
	metrics.ResetIPSetEntries()

	hns := GetHNSFake(t, "azure")
	io := common.NewMockIOShimWithFakeHNS(hns)
	iMgr := NewIPSetManager(applyAlwaysCfg, io)

	nsW := NewIPSetMetadata("w", Namespace)
	nsX := NewIPSetMetadata("x", Namespace)
	nsY := NewIPSetMetadata("y", Namespace)
	nsList1 := NewIPSetMetadata("list1", KeyLabelOfNamespace)

	iMgr.CreateIPSets([]*IPSetMetadata{nsW, nsX, nsY, nsList1})
	count, err := metrics.MaxIPSetMembers()
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 0, count)

	require.Nil(t, iMgr.AddToSets([]*IPSetMetadata{nsW, nsX}, "1.1.1.1", "pod1"))
	count, err = metrics.MaxIPSetMembers()
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 1, count)

	require.Nil(t, iMgr.AddToLists([]*IPSetMetadata{nsList1}, []*IPSetMetadata{nsW}))
	count, err = metrics.MaxIPSetMembers()
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 1, count)

	require.Nil(t, iMgr.AddToLists([]*IPSetMetadata{nsList1}, []*IPSetMetadata{nsX, nsY}))
	count, err = metrics.MaxIPSetMembers()
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 3, count)

	require.Nil(t, iMgr.AddToSets([]*IPSetMetadata{nsX}, "2.2.2.2", "pod2"))
	count, err = metrics.MaxIPSetMembers()
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 3, count)

	require.Nil(t, iMgr.RemoveFromList(nsList1, []*IPSetMetadata{nsX, nsY}))
	count, err = metrics.MaxIPSetMembers()
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 2, count)

	require.Nil(t, iMgr.AddToSets([]*IPSetMetadata{nsX}, "3.3.3.3", "pod3"))
	count, err = metrics.MaxIPSetMembers()
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 3, count)

	require.Nil(t, iMgr.AddToSets([]*IPSetMetadata{nsX}, "3.3.3.3", "pod3-changed"))
	count, err = metrics.MaxIPSetMembers()
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 3, count)

	require.Nil(t, iMgr.AddToSets([]*IPSetMetadata{nsX}, "4.4.4.4", "pod4"))
	count, err = metrics.MaxIPSetMembers()
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 4, count)

	require.Nil(t, iMgr.RemoveFromSets([]*IPSetMetadata{nsX}, "4.4.4.4", "pod4"))
	count, err = metrics.MaxIPSetMembers()
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 3, count)

	require.Nil(t, iMgr.RemoveFromSets([]*IPSetMetadata{nsX}, "3.3.3.3", "wrong-pod"))
	count, err = metrics.MaxIPSetMembers()
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 3, count)

	// can't delete this set
	iMgr.DeleteIPSet(nsX.GetPrefixName(), util.SoftDelete)
	count, err = metrics.MaxIPSetMembers()
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 3, count)

	iMgr.DeleteIPSet(nsX.GetPrefixName(), util.ForceDelete)
	count, err = metrics.MaxIPSetMembers()
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 1, count)
}

func verifyHNSCache(t *testing.T, expected map[string]hcn.SetPolicySetting, hns *hnswrapper.Hnsv2wrapperFake) {
	for setName, setObj := range expected {
		cacheObj := hns.Cache.SetPolicy(setObj.Id)
		require.NotNil(t, cacheObj)
		require.Equal(t, setObj, *cacheObj, fmt.Sprintf("%s mismatch in cache", setName))
	}
}

func verifyDeletedHNSCache(t *testing.T, deleted []string, hns *hnswrapper.Hnsv2wrapperFake) {
	for _, setName := range deleted {
		cacheObj := hns.Cache.SetPolicy(setName)
		require.Nil(t, cacheObj)
	}
}
