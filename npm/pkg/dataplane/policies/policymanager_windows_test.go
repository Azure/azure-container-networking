package policies

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/network/hnswrapper"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Microsoft/hcsshim/hcn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	expectedACLs = []*hnswrapper.FakeEndpointPolicy{
		{
			ID:              TestNetworkPolicies[0].ACLs[0].PolicyID,
			Protocols:       "6",
			Direction:       "In",
			Action:          "Block",
			LocalAddresses:  "azure-npm-3216600258",
			RemoteAddresses: "azure-npm-2031808719",
			RemotePorts:     getPortStr(222, 333),
			LocalPorts:      "",
			Priority:        blockRulePriotity,
		},
		{
			ID:              TestNetworkPolicies[0].ACLs[0].PolicyID,
			Protocols:       "17",
			Direction:       "In",
			Action:          "Allow",
			LocalAddresses:  "azure-npm-3216600258",
			RemoteAddresses: "",
			LocalPorts:      "",
			RemotePorts:     "",
			Priority:        allowRulePriotity,
		},
		{
			ID:              TestNetworkPolicies[0].ACLs[0].PolicyID,
			Protocols:       "17",
			Direction:       "Out",
			Action:          "Block",
			LocalAddresses:  "",
			RemoteAddresses: "azure-npm-3216600258",
			LocalPorts:      "144",
			RemotePorts:     "",
			Priority:        blockRulePriotity,
		},
		{
			ID:              TestNetworkPolicies[0].ACLs[0].PolicyID,
			Protocols:       "256",
			Direction:       "Out",
			Action:          "Allow",
			LocalAddresses:  "",
			RemoteAddresses: "azure-npm-3216600258",
			LocalPorts:      "",
			RemotePorts:     "",
			Priority:        allowRulePriotity,
		},
	}

	endPointIDList = map[string]string{
		"10.0.0.1": "test1",
		"10.0.0.2": "test2",
	}
)

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
	err := pMgr.AddPolicy(TestNetworkPolicies[0], endPointIDList)
	require.NoError(t, err)

	aclID := TestNetworkPolicies[0].ACLs[0].PolicyID

	aclPolicies, err := hns.Cache.ACLPolicies(endPointIDList, aclID)
	require.NoError(t, err)
	for _, id := range endPointIDList {
		acls, ok := aclPolicies[id]
		if !ok {
			t.Errorf("Expected %s to be in ACLs", id)
		}
		verifyFakeHNSCacheACLs(t, expectedACLs, acls)
	}
}

func TestRemovePolicies(t *testing.T) {
	pMgr, hns := getPMgr(t)
	err := pMgr.AddPolicy(TestNetworkPolicies[0], endPointIDList)
	require.NoError(t, err)

	aclID := TestNetworkPolicies[0].ACLs[0].PolicyID

	aclPolicies, err := hns.Cache.ACLPolicies(endPointIDList, aclID)
	require.NoError(t, err)
	for _, id := range endPointIDList {
		acls, ok := aclPolicies[id]
		if !ok {
			t.Errorf("Expected %s to be in ACLs", id)
		}
		verifyFakeHNSCacheACLs(t, expectedACLs, acls)
	}

	err = pMgr.RemovePolicy(TestNetworkPolicies[0].Name, nil)
	require.NoError(t, err)
	verifyACLCacheIsCleaned(t, hns, len(endPointIDList))
}

// Helper functions for UTS

func getPMgr(t *testing.T) (*PolicyManager, *hnswrapper.Hnsv2wrapperFake) {
	hns := ipsets.GetHNSFake(t)
	io := common.NewMockIOShimWithFakeHNS(hns)

	for ip, epID := range endPointIDList {
		ep := &hcn.HostComputeEndpoint{
			Id:   epID,
			Name: epID,
			IpConfigurations: []hcn.IpConfig{
				{
					IpAddress: ip,
				},
			},
		}
		_, err := hns.CreateEndpoint(ep)
		require.NoError(t, err)
	}
	cfg := &PolicyManagerCfg{
		PolicyMode: IPSetPolicyMode,
	}
	return NewPolicyManager(io, cfg), hns
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
		for j, cacheACL := range actual {
			assert.Equal(t,
				expectedACL.ID,
				actual[i].ID,
				fmt.Sprintf("Expected %s, got %s", expectedACL.ID, actual[i].ID),
			)
			if reflect.DeepEqual(expectedACL, cacheACL) {
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
