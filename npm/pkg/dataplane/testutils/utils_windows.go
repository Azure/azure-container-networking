package dptestutils

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/network/hnswrapper"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Microsoft/hcsshim/hcn"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/klog"
)

func PrefixNames(sets []*ipsets.IPSetMetadata) []string {
	a := make([]string, len(sets))
	for k, s := range sets {
		a[k] = s.GetPrefixName()
	}
	return a
}

func Endpoint(epID, ip string) *hcn.HostComputeEndpoint {
	return &hcn.HostComputeEndpoint{
		Id:                 epID,
		Name:               epID,
		HostComputeNetwork: common.FakeHNSNetworkID,
		IpConfigurations: []hcn.IpConfig{
			{
				IpAddress: ip,
			},
		},
	}
}

func RemoteEndpoint(epID, ip string) *hcn.HostComputeEndpoint {
	e := Endpoint(epID, ip)
	e.Flags = hcn.EndpointFlagsRemoteEndpoint
	return e
}

func SetPolicy(setMetadata *ipsets.IPSetMetadata, members ...string) *hcn.SetPolicySetting {
	pType := hcn.SetPolicyType("")
	switch setMetadata.GetSetKind() {
	case ipsets.ListSet:
		pType = hcn.SetPolicyTypeNestedIpSet
	case ipsets.HashSet:
		pType = hcn.SetPolicyTypeIpSet
	case ipsets.UnknownKind:
		pType = hcn.SetPolicyType("")
	}

	// sort for easier comparison
	sort.Strings(members)

	return &hcn.SetPolicySetting{
		Id:         setMetadata.GetHashedName(),
		Name:       setMetadata.GetPrefixName(),
		PolicyType: pType,
		Values:     strings.Join(members, ","),
	}
}

// VerifyHNSCache asserts that HNS has the correct state.
func VerifyHNSCache(t *testing.T, hns *hnswrapper.Hnsv2wrapperFake, expectedSetPolicies []*hcn.SetPolicySetting, expectedEndpointACLs map[string][]*hnswrapper.FakeEndpointPolicy) {
	t.Helper()

	PrintGetAllOutput(hns)

	// we want to evaluate both verify functions even if one fails, so don't write as verifySetPolicies() && verifyACLs() in case of short-circuiting
	success := VerifySetPolicies(t, hns, expectedSetPolicies)
	success = VerifyACLs(t, hns, expectedEndpointACLs) && success

	if !success {
		require.FailNow(t, fmt.Sprintf("hns cache had unexpected state. printing hns cache...\n%s", hns.Cache.PrettyString()))
	}
}

// VerifySetPolicies is true if HNS strictly has the expected SetPolicies.
func VerifySetPolicies(t *testing.T, hns *hnswrapper.Hnsv2wrapperFake, expectedSetPolicies []*hcn.SetPolicySetting) bool {
	t.Helper()

	cachedSetPolicies := hns.Cache.AllSetPolicies(common.FakeHNSNetworkID)

	success := assert.Equal(t, len(expectedSetPolicies), len(cachedSetPolicies), "unexpected number of SetPolicies")
	for _, expectedSetPolicy := range expectedSetPolicies {
		cachedSetPolicy, ok := cachedSetPolicies[expectedSetPolicy.Id]
		success = assert.True(t, ok, fmt.Sprintf("expected SetPolicy not found. ID %s, name: %s", expectedSetPolicy.Id, expectedSetPolicy.Name)) && success
		if !ok {
			continue
		}

		members := strings.Split(cachedSetPolicy.Values, ",")
		sort.Strings(members)
		copyOfCachedSetPolicy := *cachedSetPolicy
		copyOfCachedSetPolicy.Values = strings.Join(members, ",")

		// required that the expectedSetPolicy already has sorted members
		success = assert.Equal(t, expectedSetPolicy, &copyOfCachedSetPolicy, fmt.Sprintf("SetPolicy has unexpected contents. ID %s, name: %s", expectedSetPolicy.Id, expectedSetPolicy.Name)) && success
	}

	return success
}

// verifyACLs is true if HNS strictly has the expected Endpoints and ACLs.
func VerifyACLs(t *testing.T, hns *hnswrapper.Hnsv2wrapperFake, expectedEndpointACLs map[string][]*hnswrapper.FakeEndpointPolicy) bool {
	t.Helper()

	cachedEndpointACLs := hns.Cache.GetAllACLs()

	success := assert.Equal(t, len(expectedEndpointACLs), len(cachedEndpointACLs), "unexpected number of Endpoints")
	for epID, expectedACLs := range expectedEndpointACLs {
		cachedACLs, ok := cachedEndpointACLs[epID]
		success = assert.True(t, ok, fmt.Sprintf("expected endpoint not found: %s", epID)) && success
		if !ok {
			continue
		}

		success = assert.Equal(t, len(expectedACLs), len(cachedACLs), fmt.Sprintf("unexpected number of ACLs for Endpoint with ID: %s", epID)) && success
		for _, expectedACL := range expectedACLs {
			foundACL := false
			for _, cacheACL := range cachedACLs {
				if expectedACL.ID == cacheACL.ID {
					if cmp.Equal(expectedACL, cacheACL) {
						foundACL = true
						break
					}
				}
			}
			success = assert.True(t, foundACL, fmt.Sprintf("missing expected ACL. ID: %s, full ACL: %+v", expectedACL.ID, expectedACL)) && success
		}
	}
	return success
}

// helpful for debugging if there's a discrepancy between GetAll functions and the HNS PrettyString
func PrintGetAllOutput(hns *hnswrapper.Hnsv2wrapperFake) {
	klog.Info("SETPOLICIES...")
	for _, setPol := range hns.Cache.AllSetPolicies(common.FakeHNSNetworkID) {
		klog.Infof("%+v", setPol)
	}
	klog.Info("Endpoint ACLs...")
	for id, acls := range hns.Cache.GetAllACLs() {
		a := make([]string, len(acls))
		for k, v := range acls {
			a[k] = fmt.Sprintf("%+v", v)
		}
		klog.Infof("%s: %s", id, strings.Join(a, ","))
	}
}
