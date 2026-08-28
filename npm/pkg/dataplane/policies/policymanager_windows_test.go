package policies

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/network/hnswrapper"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	dptestutils "github.com/Azure/azure-container-networking/npm/pkg/dataplane/testutils"
	"github.com/Microsoft/hcsshim/hcn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type winPromVals struct {
	getEndpointLatencyCalls int
	getEndpointFailures     int
	createLatencyCalls      int
	createFailures          int
	updateLatencyCalls      int
	updateFailures          int
}

func (w winPromVals) test(t *testing.T) {
	t.Helper()

	count, err := metrics.TotalGetEndpointLatencyCalls()
	require.NoError(t, err, "failed to get metric")
	require.Equal(t, w.getEndpointLatencyCalls, count, "incorrect get endpoint latency calls")

	count, err = metrics.TotalGetEndpointFailures()
	require.NoError(t, err, "failed to get metric")
	require.Equal(t, w.getEndpointFailures, count, "incorrect get endpoint failures")

	count, err = metrics.TotalACLLatencyCalls(metrics.CreateOp)
	require.NoError(t, err, "failed to get metric")
	require.Equal(t, w.createLatencyCalls, count, "incorrect create latency calls")

	count, err = metrics.TotalACLFailures(metrics.CreateOp)
	require.NoError(t, err, "failed to get metric")
	require.Equal(t, w.createFailures, count, "incorrect create failures")

	count, err = metrics.TotalACLLatencyCalls(metrics.UpdateOp)
	require.NoError(t, err, "failed to get metric")
	require.Equal(t, w.updateLatencyCalls, count, "incorrect update latency calls")

	count, err = metrics.TotalACLFailures(metrics.UpdateOp)
	require.NoError(t, err, "failed to get metric")
	require.Equal(t, w.updateFailures, count, "incorrect update failures")
}

var (
	// TODO fix these expected ACLs (e.g. local/remote addresses and ports are off)
	expectedACLs = []*hnswrapper.FakeEndpointPolicy{
		{
			ID:              TestNetworkPolicies[0].ACLPolicyID,
			Protocols:       "6",
			Direction:       "In",
			Action:          "Block",
			LocalAddresses:  ipsets.TestCIDRSet.HashedName,
			RemoteAddresses: ipsets.TestKeyPodSet.HashedName,
			RemotePorts:     "",
			LocalPorts:      getPortStr(222, 333),
			Priority:        blockRulePriotity,
		},
		{
			ID:              TestNetworkPolicies[0].ACLPolicyID,
			Protocols:       "17",
			Direction:       "In",
			Action:          "Allow",
			LocalAddresses:  ipsets.TestCIDRSet.HashedName,
			RemoteAddresses: "",
			LocalPorts:      "",
			RemotePorts:     "",
			Priority:        allowRulePriotity,
		},
		{
			ID:              TestNetworkPolicies[0].ACLPolicyID,
			Protocols:       "17",
			Direction:       "Out",
			Action:          "Block",
			LocalAddresses:  ipsets.TestCIDRSet.HashedName,
			RemoteAddresses: "",
			LocalPorts:      "",
			RemotePorts:     "144",
			Priority:        blockRulePriotity,
		},
		{
			ID:              TestNetworkPolicies[0].ACLPolicyID,
			Protocols:       "", // any protocol
			Direction:       "Out",
			Action:          "Allow",
			LocalAddresses:  ipsets.TestCIDRSet.HashedName,
			RemoteAddresses: "",
			LocalPorts:      "",
			RemotePorts:     "",
			Priority:        allowRulePriotity,
		},
		// readiness probe ACL
		{
			ID:              TestNetworkPolicies[0].ACLPolicyID,
			Direction:       "In",
			Action:          "Allow",
			RemoteAddresses: "6.7.8.9",
			Priority:        201,
		},
	}

	endPointIDList = map[string]string{
		"10.0.0.1": "test1",
		"10.0.0.2": "test2",
	}
)

func endpointIDListCopy() map[string]string {
	m := make(map[string]string, len(endPointIDList))
	for k, v := range endPointIDList {
		m[k] = v
	}
	return m
}

func TestCompareAndRemovePolicies(t *testing.T) {
	epbuilder := newEndpointPolicyBuilder()

	testPol := &NPMACLPolSettings{
		Id:        "test1",
		Protocols: string(TCP),
	}
	testPol2 := &NPMACLPolSettings{
		Id:        "test1",
		Protocols: string(UDP),
	}

	epbuilder.aclPolicies = append(epbuilder.aclPolicies, []*NPMACLPolSettings{testPol, testPol2}...)

	epbuilder.compareAndRemovePolicies("test1", 2)

	if len(epbuilder.aclPolicies) != 0 {
		t.Errorf("Expected 0 policies, got %d", len(epbuilder.aclPolicies))
	}
}

func TestNumACLRulesProducedInKernelWithNodeEgressPorts(t *testing.T) {
	egressACL := NewACLPolicy(Allowed, Egress)

	base := &NPMNetworkPolicy{ACLs: []*ACLPolicy{egressACL}}
	// egress ACL (1) + extra egress rule (1)
	require.Equal(t, 2, base.numACLRulesProducedInKernel())

	withPorts := &NPMNetworkPolicy{
		ACLs:            []*ACLPolicy{egressACL},
		NodeEgressPorts: []int32{5005, 2500},
	}
	// same as base plus one TCP and one UDP ACL per node egress port
	require.Equal(t, 2+2*len(withPorts.NodeEgressPorts), withPorts.numACLRulesProducedInKernel())
}

func TestAddPolicies(t *testing.T) {
	metrics.InitializeWindowsMetrics()

	pMgr, hns := getPMgr(t)

	// AddPolicy may modify the endpointIDList, so we need to pass a copy
	err := pMgr.AddPolicies([]*NPMNetworkPolicy{TestNetworkPolicies[0]}, endpointIDListCopy())
	require.NoError(t, err)

	aclID := TestNetworkPolicies[0].ACLPolicyID

	aclPolicies, err := hns.Cache.ACLPolicies(endPointIDList, aclID)
	require.NoError(t, err)
	for _, id := range endPointIDList {
		acls, ok := aclPolicies[id]
		if !ok {
			t.Errorf("Expected endpoint ID %s to have ACLs", id)
		}
		fmt.Printf("verifying ACLs on endpoint ID %s\n", id)
		verifyFakeHNSCacheACLs(t, expectedACLs, acls)
	}

	winPromVals{
		getEndpointLatencyCalls: 2,
		getEndpointFailures:     0,
		createLatencyCalls:      2,
		createFailures:          0,
		updateLatencyCalls:      0,
		updateFailures:          0,
	}.test(t)
}

func TestRemovePolicies(t *testing.T) {
	metrics.InitializeWindowsMetrics()

	pMgr, hns := getPMgr(t)

	// AddPolicy may modify the endpointIDList, so we need to pass a copy
	err := pMgr.AddPolicies([]*NPMNetworkPolicy{TestNetworkPolicies[0]}, endpointIDListCopy())
	require.NoError(t, err)

	aclID := TestNetworkPolicies[0].ACLPolicyID

	aclPolicies, err := hns.Cache.ACLPolicies(endPointIDList, aclID)
	require.NoError(t, err)
	for _, id := range endPointIDList {
		acls, ok := aclPolicies[id]
		if !ok {
			t.Errorf("Expected %s to be in ACLs", id)
		}
		verifyFakeHNSCacheACLs(t, expectedACLs, acls)
	}

	err = pMgr.RemovePolicy(TestNetworkPolicies[0].PolicyKey)
	require.NoError(t, err)
	verifyACLCacheIsCleaned(t, hns, len(endPointIDList))

	winPromVals{
		getEndpointLatencyCalls: 4,
		getEndpointFailures:     0,
		createLatencyCalls:      2,
		createFailures:          0,
		updateLatencyCalls:      2,
		updateFailures:          0,
	}.test(t)
}

func TestApplyPoliciesEndpointNotFound(t *testing.T) {
	metrics.InitializeWindowsMetrics()

	pMgr, hns := getPMgr(t)
	testendPointIDList := map[string]string{
		"10.0.0.5": "test10",
	}
	err := pMgr.AddPolicies([]*NPMNetworkPolicy{TestNetworkPolicies[0]}, testendPointIDList)
	require.NoError(t, err)
	verifyACLCacheIsCleaned(t, hns, len(endPointIDList))

	winPromVals{
		getEndpointLatencyCalls: 1,
		getEndpointFailures:     0,
		createLatencyCalls:      0,
		createFailures:          0,
		updateLatencyCalls:      0,
		updateFailures:          0,
	}.test(t)
}

func TestRemovePoliciesEndpointNotFound(t *testing.T) {
	metrics.InitializeWindowsMetrics()

	pMgr, hns := getPMgr(t)

	// AddPolicy may modify the endpointIDList, so we need to pass a copy
	err := pMgr.AddPolicies([]*NPMNetworkPolicy{TestNetworkPolicies[0]}, endpointIDListCopy())
	require.NoError(t, err)

	aclID := TestNetworkPolicies[0].ACLPolicyID

	aclPolicies, err := hns.Cache.ACLPolicies(endPointIDList, aclID)
	require.NoError(t, err)

	testendPointIDList := map[string]string{
		"10.0.0.5": "test10",
	}
	err = pMgr.RemovePolicyForEndpoints(TestNetworkPolicies[0].PolicyKey, testendPointIDList)
	require.NoError(t, err, err)

	for _, id := range endPointIDList {
		acls, ok := aclPolicies[id]
		if !ok {
			t.Errorf("Expected endpoint ID %s to have ACLs", id)
		}
		fmt.Printf("verifying ACLs on endpoint ID %s\n", id)
		verifyFakeHNSCacheACLs(t, expectedACLs, acls)
	}

	winPromVals{
		getEndpointLatencyCalls: 3,
		getEndpointFailures:     0,
		createLatencyCalls:      2,
		createFailures:          0,
		updateLatencyCalls:      0,
		updateFailures:          0,
	}.test(t)
}

func TestGetSettingsFromACLNodeEgressPorts(t *testing.T) {
	pMgr, _ := getPMgr(t)

	policy := &NPMNetworkPolicy{
		PolicyKey:       "test/node-egress",
		ACLPolicyID:     "azure-acl-test-node-egress",
		NodeEgressPorts: []int32{5005, 2500},
	}

	settings, err := pMgr.getSettingsFromACL(policy)
	require.NoError(t, err)

	// no policy ACLs, so: 1 readiness probe ACL + one ACL per node egress port per protocol (TCP+UDP)
	require.Len(t, settings, 1+len(policy.NodeEgressPorts)*2)

	// expect a TCP and UDP allow ACL to the node IP for each port
	expected := map[string]struct{}{
		"5005/6": {}, "5005/17": {},
		"2500/6": {}, "2500/17": {},
	}
	nodeEgressCount := 0
	for _, s := range settings {
		if s.Direction != hcn.DirectionTypeOut {
			continue
		}
		nodeEgressCount++
		assert.Equal(t, policy.ACLPolicyID, s.Id)
		assert.Equal(t, hcn.ActionTypeAllow, s.Action)
		assert.Equal(t, ipsetConfig.NodeIP, s.RemoteAddresses)
		assert.NotEmpty(t, s.Protocols, "port-scoped ACL must specify a protocol for HNS")
		assert.Equal(t, "", s.LocalPorts)
		assert.Equal(t, uint16(allowRulePriotity), s.Priority)
		key := s.RemotePorts + "/" + s.Protocols
		_, ok := expected[key]
		assert.True(t, ok, "unexpected port/protocol combo %q", key)
		delete(expected, key)
	}
	assert.Equal(t, len(policy.NodeEgressPorts)*2, nodeEgressCount, "wrong number of node egress ACLs")
	assert.Empty(t, expected, "not all expected port/protocol combos were emitted")
}

// Helper functions for UTS

func getPMgr(t *testing.T) (*PolicyManager, *hnswrapper.Hnsv2wrapperFake) {
	hns := ipsets.GetHNSFake(t, "azure")
	io := common.NewMockIOShimWithFakeHNS(hns)

	dptestutils.AddIPsToHNS(t, hns, endPointIDList)

	// reset all policy PodEndpoints
	for k := range TestNetworkPolicies {
		TestNetworkPolicies[k].PodEndpoints = nil
	}

	return NewPolicyManager(io, ipsetConfig), hns
}

func verifyFakeHNSCacheACLs(t *testing.T, expected, actual []*hnswrapper.FakeEndpointPolicy) bool {
	assert.Equal(t,
		len(expected),
		len(actual),
		fmt.Sprintf("Expected %d ACL, got %d", len(TestNetworkPolicies[0].ACLs), len(actual)),
	)
	for i, expectedACL := range expected {
		foundACL := false
		// While printing actual with %+v it only prints the pointers and it is hard to debug.
		// So creating this errStr to print the actual values.
		errStr := ""
		fmt.Printf("verifying expected ACL at index %d exists\n", i)
		for j, cacheACL := range actual {
			assert.Equal(t,
				expectedACL.ID,
				actual[i].ID,
				fmt.Sprintf("Expected ID %s, got %s", expectedACL.ID, actual[i].ID),
			)
			// for some reason, this only works if we make a copy
			expectedACLCopy := *expectedACL
			if reflect.DeepEqual(&expectedACLCopy, cacheACL) {
				foundACL = true
				break
			}
			errStr += fmt.Sprintf("\n%d: %+v", j, cacheACL)
		}
		require.True(t, foundACL, fmt.Sprintf("Expected %+v to be in ACLs \n Got: %s ", expectedACL, errStr))
	}
	return true
}

func verifyACLCacheIsCleaned(t *testing.T, hns *hnswrapper.Hnsv2wrapperFake, lenOfEPs int) {
	epACLs := hns.Cache.GetAllACLs()
	assert.Equal(t, lenOfEPs, len(epACLs))
	fmt.Printf("ACLs: %+v\n", epACLs)
	for _, acls := range epACLs {
		assert.Equal(t, 0, len(acls))
	}
}

func getPortStr(start, end int32) string {
	portStr := fmt.Sprintf("%d", start)
	if start == end || end == 0 {
		return portStr
	}

	for i := start + 1; i <= end; i++ {
		portStr += fmt.Sprintf(",%d", i)
	}

	return portStr
}
