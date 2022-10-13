package dataplane

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/network/hnswrapper"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Microsoft/hcsshim/hcn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/klog"
)

type dpEvent func(*testing.T, *DataPlane, *hnswrapper.Hnsv2wrapperFake)

// FIXME move this into the common code path along with the verification functions below
const azureNetworkID = "1234"

const (
	thisNode  = "this-node"
	otherNode = "other-node"

	podKey1 = "pod1"
	podKey2 = "pod2"

	ip1 = "10.0.0.1"
	ip2 = "10.0.0.2"

	endpoint1 = "test1"
	endpoint2 = "test2"

	defaultHNSLatency = time.Duration(0)
)

var (
	podLabel1Set    = ipsets.NewIPSetMetadata("k1", ipsets.KeyLabelOfPod)
	podLabelVal1Set = ipsets.NewIPSetMetadata("k1:v1", ipsets.KeyValueLabelOfPod)
	podLabel2Set    = ipsets.NewIPSetMetadata("k2", ipsets.KeyLabelOfPod)
	podLabelVal2Set = ipsets.NewIPSetMetadata("k2:v2", ipsets.KeyValueLabelOfPod)

	podLabelSets1 = []*ipsets.IPSetMetadata{podLabel1Set, podLabelVal1Set}
	podLabelSets2 = []*ipsets.IPSetMetadata{podLabel2Set, podLabelVal2Set}

	// emptySet is a member of a list if enabled in the dp Config
	// in Windows, this Config option is actually forced to be enabled in NewDataPlane()
	emptySet      = ipsets.NewIPSetMetadata("emptyhashset", ipsets.EmptyHashSet)
	allNamespaces = ipsets.NewIPSetMetadata("all-namespaces", ipsets.KeyLabelOfNamespace)
	ns1Set        = ipsets.NewIPSetMetadata("ns1", ipsets.Namespace)
	ns2Set        = ipsets.NewIPSetMetadata("ns2", ipsets.Namespace)

	nsLabel1Set    = ipsets.NewIPSetMetadata("k1", ipsets.KeyLabelOfNamespace)
	nsLabelVal1Set = ipsets.NewIPSetMetadata("k1:v1", ipsets.KeyValueLabelOfNamespace)
	nsLabel2Set    = ipsets.NewIPSetMetadata("k1", ipsets.KeyLabelOfNamespace)
	nsLabelVal2Set = ipsets.NewIPSetMetadata("k1:v1", ipsets.KeyValueLabelOfNamespace)
)

// podCreateEvent models a Pod CREATE in the PodController
func podCreateEvent(pod *PodMetadata, nsIPSet *ipsets.IPSetMetadata, labelIPSets ...*ipsets.IPSetMetadata) dpEvent {
	return func(t *testing.T, dp *DataPlane, _ *hnswrapper.Hnsv2wrapperFake) {
		// PodController might not call this if the namespace already existed
		require.Nil(t, dp.AddToLists([]*ipsets.IPSetMetadata{allNamespaces}, []*ipsets.IPSetMetadata{nsIPSet}))
		require.Nil(t, dp.AddToSets([]*ipsets.IPSetMetadata{nsIPSet}, pod))
		// technically, the Pod Controller would make this call two sets at a time (for each key-val pair)
		require.Nil(t, dp.AddToSets(labelIPSets, pod))

		require.Nil(t, dp.ApplyDataPlane())
	}
}

// podUpdateEvent models a Pod UPDATE in the PodController
func podUpdateEvent(oldPod, newPod *PodMetadata, nsIPSet *ipsets.IPSetMetadata, toRemoveLabelSets, toAddLabelSets []*ipsets.IPSetMetadata) dpEvent {
	return func(t *testing.T, dp *DataPlane, _ *hnswrapper.Hnsv2wrapperFake) {
		// think it's impossible for this to be called on an UPDATE
		// dp.AddToLists([]*ipsets.IPSetMetadata{allNamespaces}, []*ipsets.IPSetMetadata{nsIPSet})

		for _, toRemoveSet := range toRemoveLabelSets {
			require.Nil(t, dp.RemoveFromSets([]*ipsets.IPSetMetadata{toRemoveSet}, oldPod))
		}

		for _, toAddSet := range toAddLabelSets {
			require.Nil(t, dp.AddToSets([]*ipsets.IPSetMetadata{toAddSet}, newPod))
		}

		require.Nil(t, dp.ApplyDataPlane())
	}
}

// podUpdateEvent models a Pod UPDATE in the PodController where the Pod does not change IP/node.
func podUpdateEventSameIP(pod *PodMetadata, nsIPSet *ipsets.IPSetMetadata, toRemoveLabelSets, toAddLabelSets []*ipsets.IPSetMetadata) dpEvent {
	return podUpdateEvent(pod, pod, nsIPSet, toRemoveLabelSets, toAddLabelSets)
}

// podDeleteEvent models a Pod DELETE in the PodController
func podDeleteEvent(pod *PodMetadata, nsIPSet *ipsets.IPSetMetadata, labelIPSets ...*ipsets.IPSetMetadata) dpEvent {
	return func(t *testing.T, dp *DataPlane, _ *hnswrapper.Hnsv2wrapperFake) {
		require.Nil(t, dp.RemoveFromSets([]*ipsets.IPSetMetadata{nsIPSet}, pod))
		// technically, the Pod Controller would make this call two sets at a time (for each key-val pair)
		require.Nil(t, dp.RemoveFromSets(labelIPSets, pod))

		require.Nil(t, dp.ApplyDataPlane())
	}
}

// nsCreateEvent models a Namespace CREATE in the NamespaceController
func nsCreateEvent(nsIPSet *ipsets.IPSetMetadata, labelIPSets ...*ipsets.IPSetMetadata) dpEvent {
	return func(t *testing.T, dp *DataPlane, _ *hnswrapper.Hnsv2wrapperFake) {
		// TODO
	}
}

// nsUpdateEvent models a Namespace UPDATE in the NamespaceController
func nsUpdateEvent(nsIPSet *ipsets.IPSetMetadata, labelIPSets ...*ipsets.IPSetMetadata) dpEvent {
	return func(t *testing.T, dp *DataPlane, _ *hnswrapper.Hnsv2wrapperFake) {
		// TODO
	}
}

// nsDeleteEvent models a Namespace DELETE in the NamespaceController
func nsDeleteEvent(nsIPSet *ipsets.IPSetMetadata, labelIPSets ...*ipsets.IPSetMetadata) dpEvent {
	return func(t *testing.T, dp *DataPlane, _ *hnswrapper.Hnsv2wrapperFake) {
		// TODO
	}
}

// policyUpdateEvent models a Network Policy CREATE/UPDATE in the PolicyController
// FIXME have NPMNetworkPolicy as input
func policyUpdateEvent(policyKey string) dpEvent {
	return func(t *testing.T, dp *DataPlane, _ *hnswrapper.Hnsv2wrapperFake) {
		// TODO
	}
}

// policyDeleteEvent models a Network Policy DELETE in the PolicyController
func policyDeleteEvent(policyKey string) dpEvent {
	return func(t *testing.T, dp *DataPlane, _ *hnswrapper.Hnsv2wrapperFake) {
		// TODO
	}
}

// endpointCreateEvent models an Endpoint being created within HNS
// With this Event, Endpoints can be created in between calls to dp
func endpointCreateEvent(epID, ip string) dpEvent {
	return func(t *testing.T, _ *DataPlane, hns *hnswrapper.Hnsv2wrapperFake) {
		ep := &hcn.HostComputeEndpoint{
			Id:                 epID,
			Name:               epID,
			HostComputeNetwork: azureNetworkID,
			IpConfigurations: []hcn.IpConfig{
				{
					IpAddress: ip,
				},
			},
		}
		_, err := hns.CreateEndpoint(ep)
		require.Nil(t, err, "failed to create hns endpoint in mock: %+v", ep)
	}
}

// endpointDeleteEvent models an Endpoint being deleted within HNS
// With this Event, Endpoints can be deleted in between calls to dp
func endpointDeleteEvent(epID string) dpEvent {
	return func(t *testing.T, _ *DataPlane, hns *hnswrapper.Hnsv2wrapperFake) {
		ep := &hcn.HostComputeEndpoint{
			Id: epID,
		}
		err := hns.DeleteEndpoint(ep)
		require.Nil(t, err, "failed to create hns endpoint in mock: %+v", ep)
	}
}

// backgroundEvent will run the dpEvents in the background when called
func backgroundEvent(event1 dpEvent, otherEvents ...dpEvent) dpEvent {
	allEvents := make([]dpEvent, 1)
	allEvents[0] = event1
	allEvents = append(allEvents, otherEvents...)

	return func(t *testing.T, dp *DataPlane, hns *hnswrapper.Hnsv2wrapperFake) {
		go func() {
			// delay would impact other threads as well
			for _, event := range allEvents {
				event(t, dp, hns)
			}
		}()
	}
}

// TestAllEventSequences can test any config with a sequence of events.
// TODO double check HNS mock is working as planned
func TestAllEventSequences(t *testing.T) {
	tests := []struct {
		name                 string
		cfg                  *Config
		ipEndpoints          map[string]string
		events               []dpEvent
		expectedSetPolicies  []*hcn.SetPolicySetting
		expectedEnpdointACLs map[string][]*hnswrapper.FakeEndpointPolicy
	}{
		{
			name: "add set for pod on node",
			cfg:  dpCfg,
			ipEndpoints: map[string]string{
				ip1: endpoint1,
			},
			events: []dpEvent{
				// custom dpEvent
				func(t *testing.T, dp *DataPlane, _ *hnswrapper.Hnsv2wrapperFake) {
					pod1 := NewPodMetadata(podKey1, ip1, thisNode)
					require.Nil(t, dp.AddToSets([]*ipsets.IPSetMetadata{ns1Set, podLabel1Set}, pod1))
					require.Nil(t, dp.ApplyDataPlane())
				},
			},
			expectedSetPolicies: []*hcn.SetPolicySetting{
				setPolicy(ns1Set, ip1),
				setPolicy(podLabel1Set, ip1),
			},
			expectedEnpdointACLs: map[string][]*hnswrapper.FakeEndpointPolicy{
				endpoint1: {},
			},
		},
		{
			name:        "pod created then deleted",
			cfg:         dpCfg,
			ipEndpoints: nil,
			events: []dpEvent{
				// mix of pre-defined dpEvents and a custom one
				endpointCreateEvent(endpoint1, ip1),
				podCreateEvent(NewPodMetadata(podKey1, ip1, thisNode), ns1Set, podLabelSets1...),
				endpointDeleteEvent(endpoint1),
				podDeleteEvent(NewPodMetadata(podKey1, ip1, thisNode), ns1Set, podLabelSets1...),
				func(t *testing.T, dp *DataPlane, _ *hnswrapper.Hnsv2wrapperFake) {
					dp.ipsetMgr.Reconcile()
					require.Nil(t, dp.ApplyDataPlane())
				},
			},
			expectedSetPolicies: []*hcn.SetPolicySetting{
				setPolicy(emptySet),
				setPolicy(allNamespaces, ns1Set.GetHashedName(), emptySet.GetHashedName()),
				setPolicy(ns1Set),
				// should be garbage collected: podLabel1Set and podLabel1Set
			},
			expectedEnpdointACLs: nil,
		},
		{
			name:        "pod created then updated, with policy add in background",
			cfg:         dpCfg,
			ipEndpoints: nil,
			events: []dpEvent{
				// pre-defined dpEvents
				endpointCreateEvent(endpoint1, ip1),
				podCreateEvent(NewPodMetadata(podKey1, ip1, thisNode), ns1Set, podLabelSets1...),
				backgroundEvent(policyUpdateEvent("x/base")),
				podUpdateEventSameIP(NewPodMetadata(podKey1, ip1, thisNode), ns1Set, podLabelSets1, podLabelSets2),
			},
			expectedSetPolicies: []*hcn.SetPolicySetting{
				setPolicy(emptySet),
				setPolicy(allNamespaces, ns1Set.GetHashedName(), emptySet.GetHashedName()),
				setPolicy(ns1Set, ip1),
				setPolicy(podLabel2Set, ip1),
				setPolicy(podLabelVal2Set, ip1),
				// the rest are not garbage collected yet
				setPolicy(podLabel1Set),
				setPolicy(podLabel1Set),
			},
			expectedEnpdointACLs: map[string][]*hnswrapper.FakeEndpointPolicy{
				endpoint1: {
					// {
					// 	ID:              "",
					// 	Protocols:       "",
					// 	Action:          "",
					// 	Direction:       "",
					// 	LocalAddresses:  "",
					// 	RemoteAddresses: "",
					// 	LocalPorts:      "",
					// 	RemotePorts:     "",
					// 	Priority:        0,
					// },
				},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			hns := ipsets.GetHNSFake(t)
			hns.Delay = defaultHNSLatency
			io := common.NewMockIOShimWithFakeHNS(hns)
			for ip, epID := range tt.ipEndpoints {
				event := endpointCreateEvent(epID, ip)
				event(t, nil, hns)
			}

			dp, err := NewDataPlane(thisNode, io, tt.cfg, nil)
			require.NoError(t, err, "failed to initialize dp")

			for _, event := range tt.events {
				event(t, dp, hns)
			}

			verifyHNSCache(t, hns, tt.expectedSetPolicies, tt.expectedEnpdointACLs)

			// uncomment to see output even for succeeding test cases
			// require.Fail(t, "DEBUGME")
		})
	}

	// 	I1013 15:07:01.118147   12452 ipsetmanager_windows.go:226] [IPSetManager Windows] Done applying IPSets.
	// I1013 15:07:01.118147   12452 dataplane_windows.go:279] Getting all endpoints for Network ID 1234
	// I1013 15:07:01.118147   12452 dataplane_windows.go:318] Endpoint ID test1 has no IPAddreses
	// I1013 15:07:01.118147   12452 dataplane_windows.go:105] [DataPlane] updatePod called for Pod Key pod1
	// W1013 15:07:01.118147   12452 dataplane_windows.go:121] [DataPlane] did not find endpoint with IPaddress 10.0.0.1
}

func setPolicy(setMetadata *ipsets.IPSetMetadata, members ...string) *hcn.SetPolicySetting {
	pType := hcn.SetPolicyType("")
	switch setMetadata.GetSetKind() {
	case ipsets.ListSet:
		pType = hcn.SetPolicyTypeNestedIpSet
	case ipsets.HashSet:
		pType = hcn.SetPolicyTypeIpSet
	}

	sort.Strings(members)

	return &hcn.SetPolicySetting{
		Id:         setMetadata.GetHashedName(),
		Name:       setMetadata.GetPrefixName(),
		PolicyType: pType,
		Values:     strings.Join(members, ","),
	}
}

// verifyHNSCache asserts that HNS has the correct state.
// TODO: move all these functions to common location used by windows test files in pkg ipsets and policies.
func verifyHNSCache(t *testing.T, hns *hnswrapper.Hnsv2wrapperFake, expectedSetPolicies []*hcn.SetPolicySetting, expectedEndpointACLs map[string][]*hnswrapper.FakeEndpointPolicy) {
	// we want to evaluate both verify functions even if one fails, so don't write as verifySetPolicies() && verifyACLs() in case of short-circuiting
	success := verifySetPolicies(t, hns, expectedSetPolicies)
	success = success && verifyACLs(t, hns, expectedEndpointACLs)

	printGetAllOutput(hns)
	if !success {
		require.FailNow(t, fmt.Sprintf("hns cache had unexpected state. printing hns cache...\n%s", hns.Cache.PrettyString()))
	}
}

// verifySetPolicies is true if HNS strictly has the expected SetPolicies.
func verifySetPolicies(t *testing.T, hns *hnswrapper.Hnsv2wrapperFake, expectedSetPolicies []*hcn.SetPolicySetting) bool {
	cachedSetPolicies := hns.Cache.AllSetPolicies(azureNetworkID)

	success := assert.Equal(t, len(expectedSetPolicies), len(cachedSetPolicies), "unexpected number of SetPolicies")
	for _, expectedSetPolicy := range expectedSetPolicies {
		cachedSetPolicy, ok := cachedSetPolicies[expectedSetPolicy.Id]
		success = success && assert.True(t, ok, fmt.Sprintf("expected SetPolicy not found. ID %s, name: %s", expectedSetPolicy.Id, expectedSetPolicy.Name))
		if !ok {
			continue
		}

		members := strings.Split(cachedSetPolicy.Values, ",")
		sort.Strings(members)
		copyOfCachedSetPolicy := *cachedSetPolicy
		copyOfCachedSetPolicy.Values = strings.Join(members, ",")

		success = success && assert.Equal(t, expectedSetPolicy, &copyOfCachedSetPolicy, fmt.Sprintf("SetPolicy has unexpected contents. ID %s, name: %s", expectedSetPolicy.Id, expectedSetPolicy.Name))
	}

	return success
}

// verifyACLs is true if HNS strictly has the expected Endpoints and ACLs.
func verifyACLs(t *testing.T, hns *hnswrapper.Hnsv2wrapperFake, expectedEndpointACLs map[string][]*hnswrapper.FakeEndpointPolicy) bool {
	cachedEndpointACLs := hns.Cache.GetAllACLs()

	success := assert.Equal(t, len(expectedEndpointACLs), len(cachedEndpointACLs), "unexpected number of Endpoints")
	for epID, expectedACLs := range expectedEndpointACLs {
		cachedACLs, ok := cachedEndpointACLs[epID]
		success = success && assert.True(t, ok, fmt.Sprintf("expected ACL not found for endpoint %s", epID))
		if !ok {
			continue
		}

		for _, expectedACL := range expectedACLs {
			success = success && assert.Equal(t, len(expectedACLs), len(cachedACLs), fmt.Sprintf("unexpected number of ACLs for Endpoint with ID: %s", epID))

			foundACL := false
			for _, cacheACL := range cachedACLs {
				if expectedACL.ID == cacheACL.ID {
					if reflect.DeepEqual(expectedACL, cacheACL) {
						foundACL = true
						break
					}
				}
			}
			success = success && assert.True(t, foundACL, fmt.Sprintf("missing expected ACL. ID: %s, full ACL: %+v", expectedACL.ID, expectedACL))
		}
	}
	return success
}

// helpful for debugging if there's a discrepancy between GetAll functions and the HNS PrettyString
func printGetAllOutput(hns *hnswrapper.Hnsv2wrapperFake) {
	klog.Info("SETPOLICIES...")
	for _, setPol := range hns.Cache.AllSetPolicies(azureNetworkID) {
		klog.Infof("%+v\n", setPol)
	}
	klog.Info("Endpoint ACLs...")
	for id, acls := range hns.Cache.GetAllACLs() {
		a := make([]string, len(acls))
		for k, v := range acls {
			a[k] = fmt.Sprintf("%+v", v)
		}
		klog.Infof("%s:\n%s\n", id, strings.Join(a, "\n"))
	}
}
