package dataplane

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/network/hnswrapper"
	"github.com/Azure/azure-container-networking/npm/pkg/controlplane/translation"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Microsoft/hcsshim/hcn"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

type dpEvent func(*DataPlane, *hnswrapper.Hnsv2wrapperFake) error
type thread []dpEvent
type concurrentSession []thread

// FIXME move this into the common code path along with the verification functions below
const azureNetworkID = "1234"

const (
	applyDP      bool = true
	doNotApplyDP bool = false

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

// IPSet constants
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

// POLICIES
// see translatePolicy_test.go for example rules

func policyNs1LabelPair1AllowAll() *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "labelPair1-allow-all",
			Namespace: "ns1",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"k1": "v1",
				},
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{},
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
		},
	}
}

// DP EVENTS

// podCreateEvent models a Pod CREATE in the PodController
func podCreateEvent(shouldApply bool, pod *PodMetadata, nsIPSet *ipsets.IPSetMetadata, labelIPSets ...*ipsets.IPSetMetadata) dpEvent {
	return func(dp *DataPlane, _ *hnswrapper.Hnsv2wrapperFake) error {
		context := fmt.Sprintf("create context: [pod: %+v. ns: %s. label sets: %+v]", pod, nsIPSet.Name, prefixNames(labelIPSets))

		// PodController might not call this if the namespace already existed
		if err := dp.AddToLists([]*ipsets.IPSetMetadata{allNamespaces}, []*ipsets.IPSetMetadata{nsIPSet}); err != nil {
			return errors.Wrapf(err, "[podCreateEvent] failed to add ns set to all namespaces list. %s", context)
		}

		if err := dp.AddToSets([]*ipsets.IPSetMetadata{nsIPSet}, pod); err != nil {
			return errors.Wrapf(err, "[podCreateEvent] failed to add pod ip to ns set. %s", context)
		}

		// technically, the Pod Controller would make this call two sets at a time (for each key-val pair)
		if err := dp.AddToSets(labelIPSets, pod); err != nil {
			return errors.Wrapf(err, "[podCreateEvent] failed to add pod ip to label sets. %s", context)
		}

		if shouldApply {
			if err := dp.ApplyDataPlane(); err != nil {
				return errors.Wrapf(err, "[podCreateEvent] failed to apply. %s", context)
			}
		}
		return nil
	}
}

// podUpdateEvent models a Pod UPDATE in the PodController
func podUpdateEvent(shouldApply bool, oldPod, newPod *PodMetadata, nsIPSet *ipsets.IPSetMetadata, toRemoveLabelSets, toAddLabelSets []*ipsets.IPSetMetadata) dpEvent {
	return func(dp *DataPlane, _ *hnswrapper.Hnsv2wrapperFake) error {
		context := fmt.Sprintf("update context: [old pod: %+v. new pod: %+v. ns: %s. label sets to remove: %+v. label sets to add: %+v]",
			oldPod, newPod, nsIPSet.Name, prefixNames(toRemoveLabelSets), prefixNames(toAddLabelSets))

		// think it's impossible for this to be called on an UPDATE
		// dp.AddToLists([]*ipsets.IPSetMetadata{allNamespaces}, []*ipsets.IPSetMetadata{nsIPSet})

		for _, toRemoveSet := range toRemoveLabelSets {
			if err := dp.RemoveFromSets([]*ipsets.IPSetMetadata{toRemoveSet}, oldPod); err != nil {
				return errors.Wrapf(err, "[podUpdateEvent] failed to remove old pod ip from set %s. %s", toRemoveSet.GetPrefixName(), context)
			}
		}

		for _, toAddSet := range toAddLabelSets {
			if err := dp.AddToSets([]*ipsets.IPSetMetadata{toAddSet}, newPod); err != nil {
				return errors.Wrapf(err, "[podUpdateEvent] failed to add new pod ip to set %s. %s", toAddSet.GetPrefixName(), context)
			}
		}

		if shouldApply {
			if err := dp.ApplyDataPlane(); err != nil {
				return errors.Wrapf(err, "[podUpdateEvent] failed to apply. %s", context)
			}
		}
		return nil
	}
}

// podUpdateEvent models a Pod UPDATE in the PodController where the Pod does not change IP/node.
func podUpdateEventSameIP(shouldApply bool, pod *PodMetadata, nsIPSet *ipsets.IPSetMetadata, toRemoveLabelSets, toAddLabelSets []*ipsets.IPSetMetadata) dpEvent {
	return podUpdateEvent(shouldApply, pod, pod, nsIPSet, toRemoveLabelSets, toAddLabelSets)
}

// podDeleteEvent models a Pod DELETE in the PodController
func podDeleteEvent(shouldApply bool, pod *PodMetadata, nsIPSet *ipsets.IPSetMetadata, labelIPSets ...*ipsets.IPSetMetadata) dpEvent {
	return func(dp *DataPlane, _ *hnswrapper.Hnsv2wrapperFake) error {
		context := fmt.Sprintf("delete context: [pod: %+v. ns: %s. label sets: %+v]", pod, nsIPSet.Name, prefixNames(labelIPSets))

		if err := dp.RemoveFromSets([]*ipsets.IPSetMetadata{nsIPSet}, pod); err != nil {
			return errors.Wrapf(err, "[podDeleteEvent] failed to remove pod ip from ns set. %s", context)
		}

		// technically, the Pod Controller would make this call two sets at a time (for each key-val pair)
		if err := dp.RemoveFromSets(labelIPSets, pod); err != nil {
			return errors.Wrapf(err, "[podDeleteEvent] failed to remove pod ip from label sets. %s", context)
		}

		if shouldApply {
			if err := dp.ApplyDataPlane(); err != nil {
				return errors.Wrapf(err, "[podDeleteEvent] failed to apply. %s", context)
			}
		}
		return nil
	}
}

// nsCreateEvent models a Namespace CREATE in the NamespaceController
func nsCreateEvent(shouldApply bool, nsIPSet *ipsets.IPSetMetadata, labelIPSets ...*ipsets.IPSetMetadata) dpEvent {
	return func(dp *DataPlane, _ *hnswrapper.Hnsv2wrapperFake) error {
		// TODO
		return nil
	}
}

// nsUpdateEvent models a Namespace UPDATE in the NamespaceController
func nsUpdateEvent(shouldApply bool, nsIPSet *ipsets.IPSetMetadata, labelIPSets ...*ipsets.IPSetMetadata) dpEvent {
	return func(dp *DataPlane, _ *hnswrapper.Hnsv2wrapperFake) error {
		// TODO
		return nil
	}
}

// nsDeleteEvent models a Namespace DELETE in the NamespaceController
func nsDeleteEvent(shouldApply bool, nsIPSet *ipsets.IPSetMetadata, labelIPSets ...*ipsets.IPSetMetadata) dpEvent {
	return func(dp *DataPlane, _ *hnswrapper.Hnsv2wrapperFake) error {
		// TODO
		return nil
	}
}

// policyUpdateEvent models a Network Policy CREATE/UPDATE in the PolicyController
func policyUpdateEvent(policy *networkingv1.NetworkPolicy) dpEvent {
	return func(dp *DataPlane, _ *hnswrapper.Hnsv2wrapperFake) error {
		npmNetPol, err := translation.TranslatePolicy(policy)
		if err != nil {
			return errors.Wrapf(err, "[policyUpdateEvent] failed to translate policy with key %s/%s", policy.Namespace, policy.Name)
		}

		if err := dp.UpdatePolicy(npmNetPol); err != nil {
			return errors.Wrapf(err, "[policyUpdateEvent] failed to update policy with key %s/%s", policy.Namespace, policy.Name)
		}
		return nil
	}
}

// policyDeleteEvent models a Network Policy DELETE in the PolicyController
func policyDeleteEvent(policyKey string) dpEvent {
	return func(dp *DataPlane, _ *hnswrapper.Hnsv2wrapperFake) error {
		if err := dp.RemovePolicy(policyKey); err != nil {
			return errors.Wrapf(err, "[policyDeleteEvent] failed to remove policy with key %s", policyKey)
		}
		return nil
	}
}

// endpointCreateEvent models an Endpoint being created within HNS
// With this Event, Endpoints can be created in between calls to dp
func endpointCreateEvent(epID, ip string) dpEvent {
	return func(_ *DataPlane, hns *hnswrapper.Hnsv2wrapperFake) error {
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
		if err != nil {
			return errors.Wrapf(err, "[endpointCreateEvent] failed to create hns endpoint. ep: %+v", ep)
		}
		return nil
	}
}

// endpointDeleteEvent models an Endpoint being deleted within HNS
// With this Event, Endpoints can be deleted in between calls to dp
func endpointDeleteEvent(epID string) dpEvent {
	return func(_ *DataPlane, hns *hnswrapper.Hnsv2wrapperFake) error {
		ep := &hcn.HostComputeEndpoint{
			Id: epID,
		}
		if err := hns.DeleteEndpoint(ep); err != nil {
			return errors.Wrapf(err, "[endpointDeleteEvent] failed to delete hns endpoint. ep: %+v", ep)
		}
		return nil
	}
}

// TestAllEventSequences can test any config with a sequence of events.
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
				func(dp *DataPlane, _ *hnswrapper.Hnsv2wrapperFake) error {
					pod1 := NewPodMetadata(podKey1, ip1, thisNode)
					if err := dp.AddToSets([]*ipsets.IPSetMetadata{ns1Set, podLabel1Set}, pod1); err != nil {
						return errors.Wrap(err, "[custom-add-apply] failed to add set")
					}

					if err := dp.ApplyDataPlane(); err != nil {
						return errors.Wrapf(err, "[custom-add-apply] failed to apply dp")
					}
					return nil
				},
			},
			expectedSetPolicies: []*hcn.SetPolicySetting{
				setPolicy(ns1Set, ip1),
				// setPolicy(podLabel1Set, ip1),
			},
			expectedEnpdointACLs: map[string][]*hnswrapper.FakeEndpointPolicy{
				endpoint1: {},
			},
		},
		{
			name:        "policy created with no pods",
			cfg:         dpCfg,
			ipEndpoints: nil,
			events: []dpEvent{
				// pre-defined dpEvent
				policyUpdateEvent(policyNs1LabelPair1AllowAll()),
			},
			expectedSetPolicies: []*hcn.SetPolicySetting{
				// will not be an all-namespaces IPSet unless there's a Pod/Namespace event
				setPolicy(ns1Set),
				// Policies do not create the KeyLabelOfPod type IPSet if the selector has a key-value requirement
				setPolicy(podLabelVal1Set),
			},
			expectedEnpdointACLs: nil,
		},
		{
			name:        "pod created on node -> policy created and applied to it",
			cfg:         dpCfg,
			ipEndpoints: nil,
			events: []dpEvent{
				endpointCreateEvent(endpoint1, ip1),
				podCreateEvent(applyDP, NewPodMetadata(podKey1, ip1, thisNode), ns1Set, podLabelSets1...),
				policyUpdateEvent(policyNs1LabelPair1AllowAll()),
			},
			expectedSetPolicies: []*hcn.SetPolicySetting{
				setPolicy(emptySet),
				setPolicy(allNamespaces, ns1Set.GetHashedName(), emptySet.GetHashedName()),
				setPolicy(ns1Set, ip1),
				setPolicy(podLabel1Set, ip1),
				setPolicy(podLabelVal1Set, ip1),
			},
			expectedEnpdointACLs: map[string][]*hnswrapper.FakeEndpointPolicy{
				endpoint1: {
					{
						ID:              "azure-acl-ns1-labelPair1-allow-all",
						Protocols:       "",
						Action:          "Allow",
						Direction:       "In",
						LocalAddresses:  "",
						RemoteAddresses: "",
						LocalPorts:      "",
						RemotePorts:     "",
						Priority:        222,
					},
					{
						ID:              "azure-acl-ns1-labelPair1-allow-all",
						Protocols:       "",
						Action:          "Allow",
						Direction:       "Out",
						LocalAddresses:  "",
						RemoteAddresses: "",
						LocalPorts:      "",
						RemotePorts:     "",
						Priority:        222,
					},
				},
			},
		},
		{
			name:        "pod created on node -> policy created and applied to it -> policy deleted",
			cfg:         dpCfg,
			ipEndpoints: nil,
			events: []dpEvent{
				endpointCreateEvent(endpoint1, ip1),
				podCreateEvent(applyDP, NewPodMetadata(podKey1, ip1, thisNode), ns1Set, podLabelSets1...),
				policyUpdateEvent(policyNs1LabelPair1AllowAll()),
				policyDeleteEvent("ns1/labelPair1-allow-all"),
			},
			expectedSetPolicies: []*hcn.SetPolicySetting{
				setPolicy(emptySet),
				setPolicy(allNamespaces, ns1Set.GetHashedName(), emptySet.GetHashedName()),
				setPolicy(ns1Set, ip1),
				setPolicy(podLabel1Set, ip1),
				setPolicy(podLabelVal1Set, ip1),
			},
			expectedEnpdointACLs: map[string][]*hnswrapper.FakeEndpointPolicy{
				endpoint1: {},
			},
		},
		{
			// NOTE: this fails right now. we incorrectly add a policy
			name:        "pod created off node -> relevant policy created but not applied (even though there's a local Endpoint with the same IP)",
			cfg:         dpCfg,
			ipEndpoints: nil,
			events: []dpEvent{
				endpointCreateEvent(endpoint1, ip1),
				podCreateEvent(applyDP, NewPodMetadata(podKey1, ip1, otherNode), ns1Set, podLabelSets1...),
				policyUpdateEvent(policyNs1LabelPair1AllowAll()),
			},
			expectedSetPolicies: []*hcn.SetPolicySetting{
				setPolicy(emptySet),
				setPolicy(allNamespaces, ns1Set.GetHashedName(), emptySet.GetHashedName()),
				setPolicy(ns1Set, ip1),
				setPolicy(podLabel1Set, ip1),
				setPolicy(podLabelVal1Set, ip1),
			},
			expectedEnpdointACLs: map[string][]*hnswrapper.FakeEndpointPolicy{
				endpoint1: {},
			},
		},
		{
			name:        "pod created off node -> relevant policy created but not applied (no local Endpoint)",
			cfg:         dpCfg,
			ipEndpoints: nil,
			events: []dpEvent{
				podCreateEvent(applyDP, NewPodMetadata(podKey1, ip1, otherNode), ns1Set, podLabelSets1...),
				policyUpdateEvent(policyNs1LabelPair1AllowAll()),
			},
			expectedSetPolicies: []*hcn.SetPolicySetting{
				setPolicy(emptySet),
				setPolicy(allNamespaces, ns1Set.GetHashedName(), emptySet.GetHashedName()),
				setPolicy(ns1Set, ip1),
				setPolicy(podLabel1Set, ip1),
				setPolicy(podLabelVal1Set, ip1),
			},
			expectedEnpdointACLs: nil,
		},
		{
			name:        "pod created -> pod deleted",
			cfg:         dpCfg,
			ipEndpoints: nil,
			events: []dpEvent{
				// mix of pre-defined dpEvents and a custom one
				endpointCreateEvent(endpoint1, ip1),
				podCreateEvent(applyDP, NewPodMetadata(podKey1, ip1, thisNode), ns1Set, podLabelSets1...),
				endpointDeleteEvent(endpoint1),
				podDeleteEvent(applyDP, NewPodMetadata(podKey1, ip1, thisNode), ns1Set, podLabelSets1...),
				// garbage collect IPSets
				func(dp *DataPlane, _ *hnswrapper.Hnsv2wrapperFake) error {
					dp.ipsetMgr.Reconcile()
					if err := dp.ApplyDataPlane(); err != nil {
						return errors.Wrap(err, "[custom-reconcile] failed to apply dp")
					}
					return nil
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
			name:        "policy created -> pod created which satisfies the policy pod selector",
			cfg:         dpCfg,
			ipEndpoints: nil,
			events: []dpEvent{
				policyUpdateEvent(policyNs1LabelPair1AllowAll()),
				endpointCreateEvent(endpoint1, ip1),
				podCreateEvent(applyDP, NewPodMetadata(podKey1, ip1, thisNode), ns1Set, podLabelSets1...),
			},
			expectedSetPolicies: []*hcn.SetPolicySetting{
				setPolicy(emptySet),
				setPolicy(allNamespaces, ns1Set.GetHashedName(), emptySet.GetHashedName()),
				setPolicy(ns1Set, ip1),
				setPolicy(podLabel1Set, ip1),
				setPolicy(podLabelVal1Set, ip1),
			},
			expectedEnpdointACLs: map[string][]*hnswrapper.FakeEndpointPolicy{
				endpoint1: {
					{
						ID:              "azure-acl-ns1-labelPair1-allow-all",
						Protocols:       "",
						Action:          "Allow",
						Direction:       "In",
						LocalAddresses:  "",
						RemoteAddresses: "",
						LocalPorts:      "",
						RemotePorts:     "",
						Priority:        222,
					},
					{
						ID:              "azure-acl-ns1-labelPair1-allow-all",
						Protocols:       "",
						Action:          "Allow",
						Direction:       "Out",
						LocalAddresses:  "",
						RemoteAddresses: "",
						LocalPorts:      "",
						RemotePorts:     "",
						Priority:        222,
					},
				},
			},
		},
		{
			// FIXME: debug why this case fails
			name:        "policy created -> pod created which satisfies the policy pod selector -> pod relabeled so policy removed",
			cfg:         dpCfg,
			ipEndpoints: nil,
			events: []dpEvent{
				policyUpdateEvent(policyNs1LabelPair1AllowAll()),
				endpointCreateEvent(endpoint1, ip1),
				podCreateEvent(applyDP, NewPodMetadata(podKey1, ip1, thisNode), ns1Set, podLabelSets1...),
				podUpdateEventSameIP(applyDP, NewPodMetadata(podKey1, ip1, thisNode), ns1Set, podLabelSets1, podLabelSets2),
			},
			expectedSetPolicies: []*hcn.SetPolicySetting{
				setPolicy(emptySet),
				setPolicy(allNamespaces, ns1Set.GetHashedName(), emptySet.GetHashedName()),
				setPolicy(ns1Set, ip1),
				setPolicy(podLabel1Set, ip1),
				setPolicy(podLabelVal1Set, ip1),
			},
			expectedEnpdointACLs: map[string][]*hnswrapper.FakeEndpointPolicy{
				endpoint1: {},
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
				require.Nil(t, event(nil, hns), "failed to create endpoint. ep ID: %s. ip: %s", epID, ip)
			}

			dp, err := NewDataPlane(thisNode, io, tt.cfg, nil)
			require.NoError(t, err, "failed to initialize dp")

			for k, event := range tt.events {
				require.Nil(t, event(dp, hns), "failed while running event number %d", k)
			}

			verifyHNSCache(t, hns, tt.expectedSetPolicies, tt.expectedEnpdointACLs)

			// uncomment to see output even for succeeding test cases
			// require.Fail(t, "DEBUGME: successful")
		})
	}
}

func TestConcurrentEvents(t *testing.T) {
	tests := []struct {
		name                 string
		cfg                  *Config
		ipEndpoints          map[string]string
		sessions             []concurrentSession
		expectedSetPolicies  []*hcn.SetPolicySetting
		expectedEnpdointACLs map[string][]*hnswrapper.FakeEndpointPolicy
	}{
		{
			name:        "pod created on node -> concurrent: 1) creating relevant policy <=> 2) updating pod so policy no longer relevant",
			cfg:         dpCfg,
			ipEndpoints: nil,
			sessions: []concurrentSession{
				{
					thread{
						endpointCreateEvent(endpoint1, ip1),
						podCreateEvent(applyDP, NewPodMetadata(podKey1, ip1, thisNode), ns1Set, podLabelSets1...),
					},
				},
				{
					thread{
						policyUpdateEvent(policyNs1LabelPair1AllowAll()),
					},
					thread{
						podUpdateEventSameIP(applyDP, NewPodMetadata(podKey1, ip1, thisNode), ns1Set, podLabelSets1, podLabelSets2),
					},
				},
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
				endpoint1: {},
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
				require.Nil(t, event(nil, hns), "failed to create endpoint. ep ID: %s. ip: %s", epID, ip)
			}

			dp, err := NewDataPlane(thisNode, io, tt.cfg, nil)
			require.NoError(t, err, "failed to initialize dp")

			for i, session := range tt.sessions {
				// concurrent case with multiple go routines
				wg := new(sync.WaitGroup)
				wg.Add(len(session))
				threadErrors := make(chan error, len(session))
				for j, th := range session {
					go func(threadNum int, th thread) {
						defer wg.Done()
						for k, event := range th {
							if err := event(dp, hns); err != nil {
								threadErrors <- errors.Wrapf(err, "failed to run thread %d, event %d", threadNum, k)
								return
							}
						}
						threadErrors <- nil
					}(j, th)
				}

				wg.Wait()
				close(threadErrors)
				for err := range threadErrors {
					assert.Nil(t, err, "failed during concurrency for sequence %d", i)
				}
			}

			verifyHNSCache(t, hns, tt.expectedSetPolicies, tt.expectedEnpdointACLs)

			// uncomment to see output even for succeeding test cases
			// require.Fail(t, "DEBUGME: successful")
		})
	}
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
	t.Helper()

	printGetAllOutput(hns)

	// we want to evaluate both verify functions even if one fails, so don't write as verifySetPolicies() && verifyACLs() in case of short-circuiting
	success := verifySetPolicies(t, hns, expectedSetPolicies)
	success = verifyACLs(t, hns, expectedEndpointACLs) && success

	if !success {
		require.FailNow(t, fmt.Sprintf("hns cache had unexpected state. printing hns cache...\n%s", hns.Cache.PrettyString()))
	}
}

// verifySetPolicies is true if HNS strictly has the expected SetPolicies.
func verifySetPolicies(t *testing.T, hns *hnswrapper.Hnsv2wrapperFake, expectedSetPolicies []*hcn.SetPolicySetting) bool {
	t.Helper()

	cachedSetPolicies := hns.Cache.AllSetPolicies(azureNetworkID)

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

		success = assert.Equal(t, expectedSetPolicy, &copyOfCachedSetPolicy, fmt.Sprintf("SetPolicy has unexpected contents. ID %s, name: %s", expectedSetPolicy.Id, expectedSetPolicy.Name)) && success
	}

	return success
}

// verifyACLs is true if HNS strictly has the expected Endpoints and ACLs.
func verifyACLs(t *testing.T, hns *hnswrapper.Hnsv2wrapperFake, expectedEndpointACLs map[string][]*hnswrapper.FakeEndpointPolicy) bool {
	t.Helper()

	cachedEndpointACLs := hns.Cache.GetAllACLs()

	success := assert.Equal(t, len(expectedEndpointACLs), len(cachedEndpointACLs), "unexpected number of Endpoints")
	for epID, expectedACLs := range expectedEndpointACLs {
		cachedACLs, ok := cachedEndpointACLs[epID]
		success = assert.True(t, ok, fmt.Sprintf("expected ACL not found for endpoint %s", epID)) && success
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
func printGetAllOutput(hns *hnswrapper.Hnsv2wrapperFake) {
	klog.Info("SETPOLICIES...")
	for _, setPol := range hns.Cache.AllSetPolicies(azureNetworkID) {
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

func prefixNames(sets []*ipsets.IPSetMetadata) []string {
	a := make([]string, len(sets))
	for k, s := range sets {
		a[k] = s.GetPrefixName()
	}
	return a
}
