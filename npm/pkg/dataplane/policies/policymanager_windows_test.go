package policies

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/network/hnswrapper"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	dptestutils "github.com/Azure/azure-container-networking/npm/pkg/dataplane/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestAddPolicies(t *testing.T) {
	pMgr, hns := getPMgr(t)

	// AddPolicy may modify the endpointIDList, so we need to pass a copy
	err := pMgr.AddPolicy(TestNetworkPolicies[0], endpointIDListCopy())
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
}

func TestRemovePolicies(t *testing.T) {
	pMgr, hns := getPMgr(t)

	// AddPolicy may modify the endpointIDList, so we need to pass a copy
	err := pMgr.AddPolicy(TestNetworkPolicies[0], endpointIDListCopy())
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
}

func TestApplyPoliciesEndpointNotFound(t *testing.T) {
	pMgr, hns := getPMgr(t)
	testendPointIDList := map[string]string{
		"10.0.0.5": "test10",
	}
	err := pMgr.AddPolicy(TestNetworkPolicies[0], testendPointIDList)
	require.NoError(t, err)
	verifyACLCacheIsCleaned(t, hns, len(endPointIDList))
}

func TestRemovePoliciesEndpointNotFound(t *testing.T) {
	pMgr, hns := getPMgr(t)

	// AddPolicy may modify the endpointIDList, so we need to pass a copy
	err := pMgr.AddPolicy(TestNetworkPolicies[0], endpointIDListCopy())
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
