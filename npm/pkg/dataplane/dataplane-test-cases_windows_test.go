package dataplane

import (
	"github.com/Azure/azure-container-networking/network/hnswrapper"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
	dptestutils "github.com/Azure/azure-container-networking/npm/pkg/dataplane/testutils"
	"github.com/Microsoft/hcsshim/hcn"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// tags
const (
	podCrudTag    Tag = "pod-crud"
	nsCrudTag     Tag = "namespace-crud"
	netpolCrudTag Tag = "netpol-crud"
)

const (
	thisNode  = "this-node"
	otherNode = "other-node"

	ip1 = "10.0.0.1"
	ip2 = "10.0.0.2"

	endpoint1 = "test1"
	endpoint2 = "test2"
)

// IPSet constants
var (
	podK1Set   = ipsets.NewIPSetMetadata("k1", ipsets.KeyLabelOfPod)
	podK1V1Set = ipsets.NewIPSetMetadata("k1:v1", ipsets.KeyValueLabelOfPod)
	podK2Set   = ipsets.NewIPSetMetadata("k2", ipsets.KeyLabelOfPod)
	podK2V2Set = ipsets.NewIPSetMetadata("k2:v2", ipsets.KeyValueLabelOfPod)

	// emptySet is a member of a list if enabled in the dp Config
	// in Windows, this Config option is actually forced to be enabled in NewDataPlane()
	emptySet      = ipsets.NewIPSetMetadata("emptyhashset", ipsets.EmptyHashSet)
	allNamespaces = ipsets.NewIPSetMetadata("all-namespaces", ipsets.KeyLabelOfNamespace)
	nsXSet        = ipsets.NewIPSetMetadata("x", ipsets.Namespace)
	nsYSet        = ipsets.NewIPSetMetadata("y", ipsets.Namespace)

	nsK1Set   = ipsets.NewIPSetMetadata("k1", ipsets.KeyLabelOfNamespace)
	nsK1V1Set = ipsets.NewIPSetMetadata("k1:v1", ipsets.KeyValueLabelOfNamespace)
	nsK2Set   = ipsets.NewIPSetMetadata("k2", ipsets.KeyLabelOfNamespace)
	nsK2V2Set = ipsets.NewIPSetMetadata("k2:v2", ipsets.KeyValueLabelOfNamespace)
)

// DP Configs
var (
	defaultWindowsDPCfg = &Config{
		IPSetManagerCfg: &ipsets.IPSetManagerCfg{
			IPSetMode:          ipsets.ApplyAllIPSets,
			AddEmptySetToLists: true,
		},
		PolicyManagerCfg: &policies.PolicyManagerCfg{
			PolicyMode: policies.IPSetPolicyMode,
		},
	}
)

func policyXBaseOnK1V1() *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "base",
			Namespace: "x",
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

func getAllSerialTests() []*SerialTestCase {
	return []*SerialTestCase{
		{
			Description: "pod created",
			Actions: []*Action{
				CreateEndpoint(endpoint1, ip1),
				CreatePod("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}),
				ApplyDP(),
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					podCrudTag,
				},
				DpCfg:            defaultWindowsDPCfg,
				InitialEndpoints: nil,
				ExpectedSetPolicies: []*hcn.SetPolicySetting{
					dptestutils.SetPolicy(emptySet),
					dptestutils.SetPolicy(allNamespaces, emptySet.GetHashedName(), nsXSet.GetHashedName()),
					dptestutils.SetPolicy(nsXSet, ip1),
					dptestutils.SetPolicy(podK1Set, ip1),
					dptestutils.SetPolicy(podK1V1Set, ip1),
				},
				ExpectedEnpdointACLs: map[string][]*hnswrapper.FakeEndpointPolicy{
					endpoint1: {},
				},
			},
		},
		{
			Description: "pod created, then pod deleted",
			Actions: []*Action{
				CreateEndpoint(endpoint1, ip1),
				CreatePod("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}),
				ApplyDP(),
				DeleteEndpoint(endpoint1),
				DeletePod("x", "a", ip1, map[string]string{"k1": "v1"}),
				ApplyDP(),
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					podCrudTag,
				},
				DpCfg:            defaultWindowsDPCfg,
				InitialEndpoints: nil,
				ExpectedSetPolicies: []*hcn.SetPolicySetting{
					dptestutils.SetPolicy(emptySet),
					dptestutils.SetPolicy(allNamespaces, emptySet.GetHashedName(), nsXSet.GetHashedName()),
					dptestutils.SetPolicy(nsXSet),
					dptestutils.SetPolicy(podK1Set),
					dptestutils.SetPolicy(podK1V1Set),
				},
				ExpectedEnpdointACLs: nil,
			},
		},
		{
			Description: "pod created, then pod deleted, then ipsets garbage collected",
			Actions: []*Action{
				CreateEndpoint(endpoint1, ip1),
				CreatePod("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}),
				ApplyDP(),
				DeleteEndpoint(endpoint1),
				DeletePod("x", "a", ip1, map[string]string{"k1": "v1"}),
				ApplyDP(),
				ReconcileDP(),
				ApplyDP(),
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					podCrudTag,
				},
				DpCfg:            defaultWindowsDPCfg,
				InitialEndpoints: nil,
				ExpectedSetPolicies: []*hcn.SetPolicySetting{
					dptestutils.SetPolicy(emptySet),
					dptestutils.SetPolicy(allNamespaces, emptySet.GetHashedName(), nsXSet.GetHashedName()),
					dptestutils.SetPolicy(nsXSet),
				},
				ExpectedEnpdointACLs: nil,
			},
		},
		{
			Description: "policy created with no pods",
			Actions: []*Action{
				UpdatePolicy(policyXBaseOnK1V1()),
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					netpolCrudTag,
				},
				DpCfg:            defaultWindowsDPCfg,
				InitialEndpoints: nil,
				ExpectedSetPolicies: []*hcn.SetPolicySetting{
					// will not be an all-namespaces IPSet unless there's a Pod/Namespace event
					dptestutils.SetPolicy(nsXSet),
					// Policies do not create the KeyLabelOfPod type IPSet if the selector has a key-value requirement
					dptestutils.SetPolicy(podK1V1Set),
				},
			},
		},
		{
			Description: "pod created on node, then relevant policy created",
			Actions: []*Action{
				CreateEndpoint(endpoint1, ip1),
				CreatePod("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}),
				// will apply dirty ipsets from CreatePod
				UpdatePolicy(policyXBaseOnK1V1()),
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					podCrudTag,
					netpolCrudTag,
				},
				DpCfg:            defaultWindowsDPCfg,
				InitialEndpoints: nil,
				ExpectedSetPolicies: []*hcn.SetPolicySetting{
					dptestutils.SetPolicy(emptySet),
					dptestutils.SetPolicy(allNamespaces, emptySet.GetHashedName(), nsXSet.GetHashedName()),
					dptestutils.SetPolicy(nsXSet, ip1),
					dptestutils.SetPolicy(podK1Set, ip1),
					dptestutils.SetPolicy(podK1V1Set, ip1),
				},
				ExpectedEnpdointACLs: map[string][]*hnswrapper.FakeEndpointPolicy{
					endpoint1: {
						{
							ID:              "azure-acl-x-base",
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
							ID:              "azure-acl-x-base",
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
		},
		{
			Description: "pod created on node, then relevant policy created, then policy deleted",
			Actions: []*Action{
				CreateEndpoint(endpoint1, ip1),
				CreatePod("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}),
				// will apply dirty ipsets from CreatePod
				UpdatePolicy(policyXBaseOnK1V1()),
				DeletePolicyByObject(policyXBaseOnK1V1()),
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					podCrudTag,
					netpolCrudTag,
				},
				DpCfg:            defaultWindowsDPCfg,
				InitialEndpoints: nil,
				ExpectedSetPolicies: []*hcn.SetPolicySetting{
					dptestutils.SetPolicy(emptySet),
					dptestutils.SetPolicy(allNamespaces, emptySet.GetHashedName(), nsXSet.GetHashedName()),
					dptestutils.SetPolicy(nsXSet, ip1),
					dptestutils.SetPolicy(podK1Set, ip1),
					dptestutils.SetPolicy(podK1V1Set, ip1),
				},
				ExpectedEnpdointACLs: map[string][]*hnswrapper.FakeEndpointPolicy{
					endpoint1: {},
				},
			},
		},
		{
			Description: "pod created off node (no endpoint), then relevant policy created",
			Actions: []*Action{
				CreatePod("x", "a", ip1, otherNode, map[string]string{"k1": "v1"}),
				// will apply dirty ipsets from CreatePod
				UpdatePolicy(policyXBaseOnK1V1()),
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					podCrudTag,
					netpolCrudTag,
				},
				DpCfg:            defaultWindowsDPCfg,
				InitialEndpoints: nil,
				ExpectedSetPolicies: []*hcn.SetPolicySetting{
					dptestutils.SetPolicy(emptySet),
					dptestutils.SetPolicy(allNamespaces, emptySet.GetHashedName(), nsXSet.GetHashedName()),
					dptestutils.SetPolicy(nsXSet, ip1),
					dptestutils.SetPolicy(podK1Set, ip1),
					dptestutils.SetPolicy(podK1V1Set, ip1),
				},
				ExpectedEnpdointACLs: nil,
			},
		},
		{
			Description: "pod created off node (remote endpoint), then relevant policy created",
			Actions: []*Action{
				CreateRemoteEndpoint(endpoint1, ip1),
				CreatePod("x", "a", ip1, otherNode, map[string]string{"k1": "v1"}),
				// will apply dirty ipsets from CreatePod
				UpdatePolicy(policyXBaseOnK1V1()),
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					podCrudTag,
					netpolCrudTag,
				},
				DpCfg:            defaultWindowsDPCfg,
				InitialEndpoints: nil,
				ExpectedSetPolicies: []*hcn.SetPolicySetting{
					dptestutils.SetPolicy(emptySet),
					dptestutils.SetPolicy(allNamespaces, emptySet.GetHashedName(), nsXSet.GetHashedName()),
					dptestutils.SetPolicy(nsXSet, ip1),
					dptestutils.SetPolicy(podK1Set, ip1),
					dptestutils.SetPolicy(podK1V1Set, ip1),
				},
				ExpectedEnpdointACLs: map[string][]*hnswrapper.FakeEndpointPolicy{
					endpoint1: {},
				},
			},
		},
		{
			Description: "policy created, then pod created which satisfies policy",
			Actions: []*Action{
				UpdatePolicy(policyXBaseOnK1V1()),
				CreateEndpoint(endpoint1, ip1),
				CreatePod("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}),
				ApplyDP(),
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					podCrudTag,
					netpolCrudTag,
				},
				DpCfg:            defaultWindowsDPCfg,
				InitialEndpoints: nil,
				ExpectedSetPolicies: []*hcn.SetPolicySetting{
					dptestutils.SetPolicy(emptySet),
					dptestutils.SetPolicy(allNamespaces, emptySet.GetHashedName(), nsXSet.GetHashedName()),
					dptestutils.SetPolicy(nsXSet, ip1),
					dptestutils.SetPolicy(podK1Set, ip1),
					dptestutils.SetPolicy(podK1V1Set, ip1),
				},
				ExpectedEnpdointACLs: map[string][]*hnswrapper.FakeEndpointPolicy{
					endpoint1: {
						{
							ID:              "azure-acl-x-base",
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
							ID:              "azure-acl-x-base",
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
		},
		{
			Description: "policy created, then pod created off node (no endpoint) which satisfies policy",
			Actions: []*Action{
				UpdatePolicy(policyXBaseOnK1V1()),
				CreatePod("x", "a", ip1, otherNode, map[string]string{"k1": "v1"}),
				ApplyDP(),
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					podCrudTag,
					netpolCrudTag,
				},
				DpCfg:            defaultWindowsDPCfg,
				InitialEndpoints: nil,
				ExpectedSetPolicies: []*hcn.SetPolicySetting{
					dptestutils.SetPolicy(emptySet),
					dptestutils.SetPolicy(allNamespaces, emptySet.GetHashedName(), nsXSet.GetHashedName()),
					dptestutils.SetPolicy(nsXSet, ip1),
					dptestutils.SetPolicy(podK1Set, ip1),
					dptestutils.SetPolicy(podK1V1Set, ip1),
				},
				ExpectedEnpdointACLs: nil,
			},
		},
		{
			Description: "policy created, then pod created off node (remote endpoint) which satisfies policy",
			Actions: []*Action{
				UpdatePolicy(policyXBaseOnK1V1()),
				CreateRemoteEndpoint(endpoint1, ip1),
				CreatePod("x", "a", ip1, otherNode, map[string]string{"k1": "v1"}),
				ApplyDP(),
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					podCrudTag,
					netpolCrudTag,
				},
				DpCfg:            defaultWindowsDPCfg,
				InitialEndpoints: nil,
				ExpectedSetPolicies: []*hcn.SetPolicySetting{
					dptestutils.SetPolicy(emptySet),
					dptestutils.SetPolicy(allNamespaces, emptySet.GetHashedName(), nsXSet.GetHashedName()),
					dptestutils.SetPolicy(nsXSet, ip1),
					dptestutils.SetPolicy(podK1Set, ip1),
					dptestutils.SetPolicy(podK1V1Set, ip1),
				},
				ExpectedEnpdointACLs: map[string][]*hnswrapper.FakeEndpointPolicy{
					endpoint1: {},
				},
			},
		},
		{
			Description: "policy created, then pod created which satisfies policy, then pod relabeled and no longer satisfies policy",
			Actions: []*Action{
				UpdatePolicy(policyXBaseOnK1V1()),
				CreateEndpoint(endpoint1, ip1),
				CreatePod("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}),
				ApplyDP(),
				UpdatePodLabels("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}, map[string]string{"k2": "v2"}),
				ApplyDP(),
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					podCrudTag,
					netpolCrudTag,
				},
				DpCfg:            defaultWindowsDPCfg,
				InitialEndpoints: nil,
				ExpectedSetPolicies: []*hcn.SetPolicySetting{
					dptestutils.SetPolicy(emptySet),
					dptestutils.SetPolicy(allNamespaces, emptySet.GetHashedName(), nsXSet.GetHashedName()),
					dptestutils.SetPolicy(nsXSet, ip1),
					// old labels (not yet garbage collected)
					dptestutils.SetPolicy(podK1Set),
					dptestutils.SetPolicy(podK1V1Set),
					// new labels
					dptestutils.SetPolicy(podK2Set, ip1),
					dptestutils.SetPolicy(podK2V2Set, ip1),
				},
				ExpectedEnpdointACLs: map[string][]*hnswrapper.FakeEndpointPolicy{
					endpoint1: {},
				},
			},
		},
		{
			Description: "Pod B replaces Pod A with same IP",
			Actions: []*Action{
				CreateEndpoint(endpoint1, ip1),
				CreatePod("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}),
				ApplyDP(),
				DeleteEndpoint(endpoint1),
				CreateEndpoint(endpoint2, ip1),
				// in case CreatePod("x", "b", ...) is somehow processed before DeletePod("x", "a", ...)
				CreatePod("x", "b", ip1, thisNode, map[string]string{"k2": "v2"}),
				// policy should not be applied to x/b since ipset manager should not consider pod x/b part of k1:v1 selector ipsets
				UpdatePolicy(policyXBaseOnK1V1()),
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					podCrudTag,
					netpolCrudTag,
				},
				DpCfg:            defaultWindowsDPCfg,
				InitialEndpoints: nil,
				ExpectedSetPolicies: []*hcn.SetPolicySetting{
					dptestutils.SetPolicy(emptySet),
					dptestutils.SetPolicy(allNamespaces, emptySet.GetHashedName(), nsXSet.GetHashedName()),
					dptestutils.SetPolicy(nsXSet, ip1),
					dptestutils.SetPolicy(podK1Set, ip1),
					dptestutils.SetPolicy(podK1V1Set, ip1),
					dptestutils.SetPolicy(podK2Set, ip1),
					dptestutils.SetPolicy(podK2V2Set, ip1),
				},
				ExpectedEnpdointACLs: map[string][]*hnswrapper.FakeEndpointPolicy{
					endpoint2: {},
				},
			},
		},
	}
}

func getAllMultiJobTests() []*MultiJobTestCase {
	return []*MultiJobTestCase{
		{
			Description: "create namespaces, pods, and a policy which applies to a pod",
			Jobs: map[string][]*Action{
				"namespace_controller": {
					CreateNamespace("x", map[string]string{"k1": "v1"}),
					CreateNamespace("y", map[string]string{"k2": "v2"}),
					ApplyDP(),
				},
				"pod_controller": {
					CreatePod("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}),
					CreatePod("y", "a", ip2, otherNode, map[string]string{"k1": "v1"}),
					ApplyDP(),
				},
				"policy_controller": {
					UpdatePolicy(policyXBaseOnK1V1()),
				},
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					nsCrudTag,
					podCrudTag,
					netpolCrudTag,
				},
				DpCfg: defaultWindowsDPCfg,
				InitialEndpoints: []*hcn.HostComputeEndpoint{
					// ends up being 2 identical endpoints (test2)??
					dptestutils.Endpoint(endpoint1, ip1),
					dptestutils.RemoteEndpoint(endpoint2, ip2),
				},
				ExpectedSetPolicies: []*hcn.SetPolicySetting{
					dptestutils.SetPolicy(emptySet),
					dptestutils.SetPolicy(allNamespaces, emptySet.GetHashedName(), nsXSet.GetHashedName(), nsYSet.GetHashedName()),
					dptestutils.SetPolicy(nsXSet, ip1),
					dptestutils.SetPolicy(nsYSet, ip2),
					dptestutils.SetPolicy(nsK1Set, emptySet.GetHashedName(), nsXSet.GetHashedName()),
					dptestutils.SetPolicy(nsK1V1Set, emptySet.GetHashedName(), nsXSet.GetHashedName()),
					dptestutils.SetPolicy(nsK2Set, emptySet.GetHashedName(), nsYSet.GetHashedName()),
					dptestutils.SetPolicy(nsK2V2Set, emptySet.GetHashedName(), nsYSet.GetHashedName()),
					dptestutils.SetPolicy(podK1Set, ip1, ip2),
					dptestutils.SetPolicy(podK1V1Set, ip1, ip2),
				},
				ExpectedEnpdointACLs: map[string][]*hnswrapper.FakeEndpointPolicy{
					endpoint1: {
						{
							ID:              "azure-acl-x-base",
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
							ID:              "azure-acl-x-base",
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
					endpoint2: {},
				},
			},
		},
	}
}
