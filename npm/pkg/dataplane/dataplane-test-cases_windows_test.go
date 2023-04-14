package dataplane

import (
	"time"

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
	podCrudTag           Tag = "pod-crud"
	nsCrudTag            Tag = "namespace-crud"
	netpolCrudTag        Tag = "netpol-crud"
	reconcileTag         Tag = "reconcile"
	calicoTag            Tag = "calico"
	applyInBackgroundTag Tag = "apply-in-background"
)

const (
	testNodeIP = "6.7.8.9"
	thisNode   = "this-node"
	otherNode  = "other-node"

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
	podK3Set   = ipsets.NewIPSetMetadata("k3", ipsets.KeyLabelOfPod)
	podK3V3Set = ipsets.NewIPSetMetadata("k3:v3", ipsets.KeyValueLabelOfPod)

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
			NetworkName:        "azure",
			IPSetMode:          ipsets.ApplyAllIPSets,
			AddEmptySetToLists: true,
		},
		PolicyManagerCfg: &policies.PolicyManagerCfg{
			NodeIP:     testNodeIP,
			PolicyMode: policies.IPSetPolicyMode,
		},
	}

	windowsCalicoDPCfg = &Config{
		IPSetManagerCfg: &ipsets.IPSetManagerCfg{
			NetworkName:        "Calico",
			IPSetMode:          ipsets.ApplyAllIPSets,
			AddEmptySetToLists: true,
		},
		PolicyManagerCfg: &policies.PolicyManagerCfg{
			NodeIP:     testNodeIP,
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

func policyXBase2OnK2V2() *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "base2",
			Namespace: "x",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"k2": "v2",
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

func policyXBase3OnK3V3() *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "base3",
			Namespace: "x",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"k3": "v3",
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

func basicTests() []*SerialTestCase {
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
					reconcileTag,
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
						{
							ID:              "azure-acl-x-base",
							Action:          "Allow",
							Direction:       "In",
							RemoteAddresses: testNodeIP,
							Priority:        201,
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
						{
							ID:              "azure-acl-x-base",
							Action:          "Allow",
							Direction:       "In",
							RemoteAddresses: testNodeIP,
							Priority:        201,
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
		{
			Description: "issue 1613: remove last instance of label, then reconcile IPSets, then apply DP",
			Actions: []*Action{
				CreateEndpoint(endpoint1, ip1),
				CreatePod("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}),
				ApplyDP(),
				UpdatePodLabels("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}, nil),
				ReconcileDP(),
				ApplyDP(),
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					podCrudTag,
					reconcileTag,
				},
				DpCfg:            defaultWindowsDPCfg,
				InitialEndpoints: nil,
				ExpectedSetPolicies: []*hcn.SetPolicySetting{
					dptestutils.SetPolicy(emptySet),
					dptestutils.SetPolicy(allNamespaces, emptySet.GetHashedName(), nsXSet.GetHashedName()),
					dptestutils.SetPolicy(nsXSet, ip1),
				},
				ExpectedEnpdointACLs: map[string][]*hnswrapper.FakeEndpointPolicy{
					endpoint1: {},
				},
			},
		},
		{
			Description: "pod created to satisfy policy, then policy deleted, then pod relabeled to no longer satisfy policy, then policy re-created and pod relabeled to satisfy policy",
			Actions: []*Action{
				CreateEndpoint(endpoint1, ip1),
				CreatePod("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}),
				// will apply dirty ipsets from CreatePod
				UpdatePolicy(policyXBaseOnK1V1()),
				DeletePolicyByObject(policyXBaseOnK1V1()),
				UpdatePodLabels("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}, map[string]string{"k2": "v2"}),
				ApplyDP(),
				UpdatePolicy(policyXBaseOnK1V1()),
				ApplyDP(),
				UpdatePodLabels("x", "a", ip1, thisNode, map[string]string{"k2": "v2"}, map[string]string{"k1": "v1"}),
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
					dptestutils.SetPolicy(podK2Set),
					dptestutils.SetPolicy(podK2V2Set),
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
						{
							ID:              "azure-acl-x-base",
							Action:          "Allow",
							Direction:       "In",
							RemoteAddresses: testNodeIP,
							Priority:        201,
						},
					},
				},
			},
		},
	}
}

func capzCalicoTests() []*SerialTestCase {
	return []*SerialTestCase{
		{
			Description: "Calico Network: base ACLs",
			Actions: []*Action{
				CreateEndpoint(endpoint1, ip1),
				CreatePod("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}),
				ApplyDP(),
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					calicoTag,
					podCrudTag,
				},
				DpCfg:            windowsCalicoDPCfg,
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
							ID:              "azure-acl-baseazurewireserver",
							Action:          "Block",
							Direction:       "Out",
							Priority:        200,
							RemoteAddresses: "168.63.129.16/32",
							RemotePorts:     "80",
							Protocols:       "6",
						},
						{
							ID:        "azure-acl-baseallowinswitch",
							Action:    "Allow",
							Direction: "In",
							Priority:  65499,
						},
						{
							ID:        "azure-acl-baseallowoutswitch",
							Action:    "Allow",
							Direction: "Out",
							Priority:  65499,
						},
						{
							ID:              "azure-acl-baseallowinhost",
							Action:          "Allow",
							Direction:       "In",
							LocalAddresses:  "",
							Priority:        0,
							RemoteAddresses: "",
							// RuleType is unsupported in FakeEndpointPolicy
							// RuleType: "Host",
						},
						{
							ID:              "azure-acl-baseallowouthost",
							Action:          "Allow",
							Direction:       "Out",
							LocalAddresses:  "",
							Priority:        0,
							RemoteAddresses: "",
							// RuleType is unsupported in FakeEndpointPolicy
							// RuleType: "Host",
						},
					},
				},
			},
		},
		{
			Description: "Calico Network: add netpol",
			Actions: []*Action{
				CreateEndpoint(endpoint1, ip1),
				CreatePod("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}),
				ApplyDP(),
				UpdatePolicy(policyXBaseOnK1V1()),
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					calicoTag,
					podCrudTag,
					netpolCrudTag,
				},
				DpCfg:            windowsCalicoDPCfg,
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
							ID:              "azure-acl-baseazurewireserver",
							Action:          "Block",
							Direction:       "Out",
							Priority:        200,
							RemoteAddresses: "168.63.129.16/32",
							RemotePorts:     "80",
							Protocols:       "6",
						},
						{
							ID:        "azure-acl-baseallowinswitch",
							Action:    "Allow",
							Direction: "In",
							Priority:  65499,
						},
						{
							ID:        "azure-acl-baseallowoutswitch",
							Action:    "Allow",
							Direction: "Out",
							Priority:  65499,
						},
						{
							ID:              "azure-acl-baseallowinhost",
							Action:          "Allow",
							Direction:       "In",
							LocalAddresses:  "",
							Priority:        0,
							RemoteAddresses: "",
							// RuleType is unsupported in FakeEndpointPolicy
							// RuleType: "Host",
						},
						{
							ID:              "azure-acl-baseallowouthost",
							Action:          "Allow",
							Direction:       "Out",
							LocalAddresses:  "",
							Priority:        0,
							RemoteAddresses: "",
							// RuleType is unsupported in FakeEndpointPolicy
							// RuleType: "Host",
						},
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
						{
							ID:              "azure-acl-x-base",
							Action:          "Allow",
							Direction:       "In",
							RemoteAddresses: testNodeIP,
							Priority:        201,
						},
					},
				},
			},
		},
		{
			Description: "Calico Network: add then remove netpol",
			Actions: []*Action{
				CreateEndpoint(endpoint1, ip1),
				CreatePod("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}),
				ApplyDP(),
				UpdatePolicy(policyXBaseOnK1V1()),
				DeletePolicyByObject(policyXBaseOnK1V1()),
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					calicoTag,
					podCrudTag,
					netpolCrudTag,
				},
				DpCfg: windowsCalicoDPCfg,

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
							ID:              "azure-acl-baseazurewireserver",
							Action:          "Block",
							Direction:       "Out",
							Priority:        200,
							RemoteAddresses: "168.63.129.16/32",
							RemotePorts:     "80",
							Protocols:       "6",
						},
						{
							ID:        "azure-acl-baseallowinswitch",
							Action:    "Allow",
							Direction: "In",
							Priority:  65499,
						},
						{
							ID:        "azure-acl-baseallowoutswitch",
							Action:    "Allow",
							Direction: "Out",
							Priority:  65499,
						},
						{
							ID:              "azure-acl-baseallowinhost",
							Action:          "Allow",
							Direction:       "In",
							LocalAddresses:  "",
							Priority:        0,
							RemoteAddresses: "",
							// RuleType is unsupported in FakeEndpointPolicy
							// RuleType: "Host",
						},
						{
							ID:              "azure-acl-baseallowouthost",
							Action:          "Allow",
							Direction:       "Out",
							LocalAddresses:  "",
							Priority:        0,
							RemoteAddresses: "",
							// RuleType is unsupported in FakeEndpointPolicy
							// RuleType: "Host",
						},
					},
				},
			},
		},
	}
}

// see issue #1729 for context on sequences 1, 2, 3
func updatePodTests() []*SerialTestCase {
	sequence1Tests := []*SerialTestCase{
		{
			Description: "Sequence 1: Pod A create --> Policy create --> Pod A cleanup --> Pod B create",
			Actions: []*Action{
				CreatePod("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}),
				ApplyDP(),
				UpdatePolicy(policyXBaseOnK1V1()),
				UpdatePolicy(policyXBase2OnK2V2()),
				DeletePod("x", "a", ip1, map[string]string{"k1": "v1"}),
				ApplyDP(),
				CreatePod("x", "b", ip1, thisNode, map[string]string{"k2": "v2"}),
				ApplyDP(),
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					podCrudTag,
					netpolCrudTag,
				},
				DpCfg: defaultWindowsDPCfg,
				InitialEndpoints: []*hcn.HostComputeEndpoint{
					dptestutils.Endpoint(endpoint1, ip1),
				},
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
					endpoint1: {
						{
							ID:              "azure-acl-x-base2",
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
							ID:              "azure-acl-x-base2",
							Protocols:       "",
							Action:          "Allow",
							Direction:       "Out",
							LocalAddresses:  "",
							RemoteAddresses: "",
							LocalPorts:      "",
							RemotePorts:     "",
							Priority:        222,
						},
						{
							ID:              "azure-acl-x-base2",
							Action:          "Allow",
							Direction:       "In",
							RemoteAddresses: testNodeIP,
							Priority:        201,
						},
					},
				},
			},
		},
		{
			Description: "Sequence 1: Policy create --> Pod A create --> Pod A cleanup --> Pod B create",
			Actions: []*Action{
				UpdatePolicy(policyXBaseOnK1V1()),
				UpdatePolicy(policyXBase2OnK2V2()),
				CreatePod("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}),
				ApplyDP(),
				DeletePod("x", "a", ip1, map[string]string{"k1": "v1"}),
				ApplyDP(),
				CreatePod("x", "b", ip1, thisNode, map[string]string{"k2": "v2"}),
				ApplyDP(),
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					podCrudTag,
					netpolCrudTag,
				},
				DpCfg: defaultWindowsDPCfg,
				InitialEndpoints: []*hcn.HostComputeEndpoint{
					dptestutils.Endpoint(endpoint1, ip1),
				},
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
					endpoint1: {
						{
							ID:              "azure-acl-x-base2",
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
							ID:              "azure-acl-x-base2",
							Protocols:       "",
							Action:          "Allow",
							Direction:       "Out",
							LocalAddresses:  "",
							RemoteAddresses: "",
							LocalPorts:      "",
							RemotePorts:     "",
							Priority:        222,
						},
						{
							ID:              "azure-acl-x-base2",
							Action:          "Allow",
							Direction:       "In",
							RemoteAddresses: testNodeIP,
							Priority:        201,
						},
					},
				},
			},
		},
		{
			Description: "Sequence 1: Policy create --> Pod A create --> Pod A cleanup --> Pod B create (skip first apply DP)",
			Actions: []*Action{
				UpdatePolicy(policyXBaseOnK1V1()),
				UpdatePolicy(policyXBase2OnK2V2()),
				CreatePod("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}),
				DeletePod("x", "a", ip1, map[string]string{"k1": "v1"}),
				ApplyDP(),
				CreatePod("x", "b", ip1, thisNode, map[string]string{"k2": "v2"}),
				ApplyDP(),
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					podCrudTag,
					netpolCrudTag,
				},
				DpCfg: defaultWindowsDPCfg,
				InitialEndpoints: []*hcn.HostComputeEndpoint{
					dptestutils.Endpoint(endpoint1, ip1),
				},
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
					endpoint1: {
						{
							ID:              "azure-acl-x-base2",
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
							ID:              "azure-acl-x-base2",
							Protocols:       "",
							Action:          "Allow",
							Direction:       "Out",
							LocalAddresses:  "",
							RemoteAddresses: "",
							LocalPorts:      "",
							RemotePorts:     "",
							Priority:        222,
						},
						{
							ID:              "azure-acl-x-base2",
							Action:          "Allow",
							Direction:       "In",
							RemoteAddresses: testNodeIP,
							Priority:        201,
						},
					},
				},
			},
		},
		{
			Description: "Sequence 1: Policy create --> Pod A create --> Pod A cleanup --> Pod B create (skip first two apply DP)",
			Actions: []*Action{
				UpdatePolicy(policyXBaseOnK1V1()),
				UpdatePolicy(policyXBase2OnK2V2()),
				CreatePod("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}),
				DeletePod("x", "a", ip1, map[string]string{"k1": "v1"}),
				CreatePod("x", "b", ip1, thisNode, map[string]string{"k2": "v2"}),
				ApplyDP(),
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					podCrudTag,
					netpolCrudTag,
				},
				DpCfg: defaultWindowsDPCfg,
				InitialEndpoints: []*hcn.HostComputeEndpoint{
					dptestutils.Endpoint(endpoint1, ip1),
				},
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
					endpoint1: {
						{
							ID:              "azure-acl-x-base2",
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
							ID:              "azure-acl-x-base2",
							Protocols:       "",
							Action:          "Allow",
							Direction:       "Out",
							LocalAddresses:  "",
							RemoteAddresses: "",
							LocalPorts:      "",
							RemotePorts:     "",
							Priority:        222,
						},
						{
							ID:              "azure-acl-x-base2",
							Action:          "Allow",
							Direction:       "In",
							RemoteAddresses: testNodeIP,
							Priority:        201,
						},
					},
				},
			},
		},
	}

	sequence2Tests := []*SerialTestCase{
		{
			Description: "Sequence 2 with Calico network",
			Actions: []*Action{
				UpdatePolicy(policyXBaseOnK1V1()),
				UpdatePolicy(policyXBase2OnK2V2()),
				CreatePod("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}),
				ApplyDP(),
				CreatePod("x", "b", ip1, thisNode, map[string]string{"k2": "v2"}),
				ApplyDP(),
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					podCrudTag,
					netpolCrudTag,
				},
				DpCfg: windowsCalicoDPCfg,
				InitialEndpoints: []*hcn.HostComputeEndpoint{
					dptestutils.Endpoint(endpoint1, ip1),
				},
				ExpectedSetPolicies: []*hcn.SetPolicySetting{
					dptestutils.SetPolicy(emptySet),
					dptestutils.SetPolicy(allNamespaces, emptySet.GetHashedName(), nsXSet.GetHashedName()),
					dptestutils.SetPolicy(nsXSet, ip1),
					// IP temporarily associated with IPSets of both pod A and pod B
					// Pod A sets
					dptestutils.SetPolicy(podK1Set, ip1),
					dptestutils.SetPolicy(podK1V1Set, ip1),
					// Pod B sets
					dptestutils.SetPolicy(podK2Set, ip1),
					dptestutils.SetPolicy(podK2V2Set, ip1),
				},
				ExpectedEnpdointACLs: map[string][]*hnswrapper.FakeEndpointPolicy{
					endpoint1: {
						{
							ID:              "azure-acl-x-base2",
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
							ID:              "azure-acl-x-base2",
							Protocols:       "",
							Action:          "Allow",
							Direction:       "Out",
							LocalAddresses:  "",
							RemoteAddresses: "",
							LocalPorts:      "",
							RemotePorts:     "",
							Priority:        222,
						},
						{
							ID:              "azure-acl-x-base2",
							Action:          "Allow",
							Direction:       "In",
							RemoteAddresses: testNodeIP,
							Priority:        201,
						},
						{
							ID:              "azure-acl-baseazurewireserver",
							Action:          "Block",
							Direction:       "Out",
							Priority:        200,
							RemoteAddresses: "168.63.129.16/32",
							RemotePorts:     "80",
							Protocols:       "6",
						},
						{
							ID:        "azure-acl-baseallowinswitch",
							Action:    "Allow",
							Direction: "In",
							Priority:  65499,
						},
						{
							ID:        "azure-acl-baseallowoutswitch",
							Action:    "Allow",
							Direction: "Out",
							Priority:  65499,
						},
						{
							ID:              "azure-acl-baseallowinhost",
							Action:          "Allow",
							Direction:       "In",
							LocalAddresses:  "",
							Priority:        0,
							RemoteAddresses: "",
							// RuleType is unsupported in FakeEndpointPolicy
							// RuleType: "Host",
						},
						{
							ID:              "azure-acl-baseallowouthost",
							Action:          "Allow",
							Direction:       "Out",
							LocalAddresses:  "",
							Priority:        0,
							RemoteAddresses: "",
							// RuleType is unsupported in FakeEndpointPolicy
							// RuleType: "Host",
						},
					},
				},
			},
		},
		{
			Description: "Sequence 2: Policy create --> Pod A Create --> Pod B create",
			Actions: []*Action{
				UpdatePolicy(policyXBaseOnK1V1()),
				UpdatePolicy(policyXBase2OnK2V2()),
				CreatePod("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}),
				ApplyDP(),
				CreatePod("x", "b", ip1, thisNode, map[string]string{"k2": "v2"}),
				ApplyDP(),
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					podCrudTag,
					netpolCrudTag,
				},
				DpCfg: defaultWindowsDPCfg,
				InitialEndpoints: []*hcn.HostComputeEndpoint{
					dptestutils.Endpoint(endpoint1, ip1),
				},
				ExpectedSetPolicies: []*hcn.SetPolicySetting{
					dptestutils.SetPolicy(emptySet),
					dptestutils.SetPolicy(allNamespaces, emptySet.GetHashedName(), nsXSet.GetHashedName()),
					dptestutils.SetPolicy(nsXSet, ip1),
					// IP temporarily associated with IPSets of both pod A and pod B
					// Pod A sets
					dptestutils.SetPolicy(podK1Set, ip1),
					dptestutils.SetPolicy(podK1V1Set, ip1),
					// Pod B sets
					dptestutils.SetPolicy(podK2Set, ip1),
					dptestutils.SetPolicy(podK2V2Set, ip1),
				},
				ExpectedEnpdointACLs: map[string][]*hnswrapper.FakeEndpointPolicy{
					endpoint1: {
						{
							ID:              "azure-acl-x-base2",
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
							ID:              "azure-acl-x-base2",
							Protocols:       "",
							Action:          "Allow",
							Direction:       "Out",
							LocalAddresses:  "",
							RemoteAddresses: "",
							LocalPorts:      "",
							RemotePorts:     "",
							Priority:        222,
						},
						{
							ID:              "azure-acl-x-base2",
							Action:          "Allow",
							Direction:       "In",
							RemoteAddresses: testNodeIP,
							Priority:        201,
						},
					},
				},
			},
		},
		{
			Description: "Sequence 2: Policy create --> Pod A Create --> Pod B create --> Pod A cleanup",
			Actions: []*Action{
				UpdatePolicy(policyXBaseOnK1V1()),
				UpdatePolicy(policyXBase2OnK2V2()),
				CreatePod("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}),
				ApplyDP(),
				CreatePod("x", "b", ip1, thisNode, map[string]string{"k2": "v2"}),
				ApplyDP(),
				DeletePod("x", "a", ip1, map[string]string{"k1": "v1"}),
				ApplyDP(),
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					podCrudTag,
					netpolCrudTag,
				},
				DpCfg: defaultWindowsDPCfg,
				InitialEndpoints: []*hcn.HostComputeEndpoint{
					dptestutils.Endpoint(endpoint1, ip1),
				},
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
					endpoint1: {
						{
							ID:              "azure-acl-x-base2",
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
							ID:              "azure-acl-x-base2",
							Protocols:       "",
							Action:          "Allow",
							Direction:       "Out",
							LocalAddresses:  "",
							RemoteAddresses: "",
							LocalPorts:      "",
							RemotePorts:     "",
							Priority:        222,
						},
						{
							ID:              "azure-acl-x-base2",
							Action:          "Allow",
							Direction:       "In",
							RemoteAddresses: testNodeIP,
							Priority:        201,
						},
					},
				},
			},
		},
		{
			// skipping this test. See PR #1856
			Description: "Sequence 2: Policy create --> Pod A Create --> Pod B create (skip first ApplyDP())",
			Actions: []*Action{
				UpdatePolicy(policyXBaseOnK1V1()),
				UpdatePolicy(policyXBase2OnK2V2()),
				CreatePod("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}),
				CreatePod("x", "b", ip1, thisNode, map[string]string{"k2": "v2"}),
				ApplyDP(),
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					podCrudTag,
					netpolCrudTag,
				},
				DpCfg: defaultWindowsDPCfg,
				InitialEndpoints: []*hcn.HostComputeEndpoint{
					dptestutils.Endpoint(endpoint1, ip1),
				},
				ExpectedSetPolicies: []*hcn.SetPolicySetting{
					dptestutils.SetPolicy(emptySet),
					dptestutils.SetPolicy(allNamespaces, emptySet.GetHashedName(), nsXSet.GetHashedName()),
					dptestutils.SetPolicy(nsXSet, ip1),
					// IP temporarily associated with IPSets of both pod A and pod B
					// Pod A sets
					dptestutils.SetPolicy(podK1Set, ip1),
					dptestutils.SetPolicy(podK1V1Set, ip1),
					// Pod B sets
					dptestutils.SetPolicy(podK2Set, ip1),
					dptestutils.SetPolicy(podK2V2Set, ip1),
				},
				ExpectedEnpdointACLs: map[string][]*hnswrapper.FakeEndpointPolicy{
					endpoint1: {
						{
							ID:              "azure-acl-x-base2",
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
							ID:              "azure-acl-x-base2",
							Protocols:       "",
							Action:          "Allow",
							Direction:       "Out",
							LocalAddresses:  "",
							RemoteAddresses: "",
							LocalPorts:      "",
							RemotePorts:     "",
							Priority:        222,
						},
						{
							ID:              "azure-acl-x-base2",
							Action:          "Allow",
							Direction:       "In",
							RemoteAddresses: testNodeIP,
							Priority:        201,
						},
					},
				},
			},
		},
		{
			// skipping this test. See PR #1856
			Description: "Sequence 2: Policy create --> Pod A Create --> Pod B create --> Pod A cleanup (skip first two ApplyDP())",
			Actions: []*Action{
				UpdatePolicy(policyXBaseOnK1V1()),
				UpdatePolicy(policyXBase2OnK2V2()),
				CreatePod("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}),
				CreatePod("x", "b", ip1, thisNode, map[string]string{"k2": "v2"}),
				DeletePod("x", "a", ip1, map[string]string{"k1": "v1"}),
				ApplyDP(),
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					podCrudTag,
					netpolCrudTag,
				},
				DpCfg: defaultWindowsDPCfg,
				InitialEndpoints: []*hcn.HostComputeEndpoint{
					dptestutils.Endpoint(endpoint1, ip1),
				},
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
					endpoint1: {
						{
							ID:              "azure-acl-x-base2",
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
							ID:              "azure-acl-x-base2",
							Protocols:       "",
							Action:          "Allow",
							Direction:       "Out",
							LocalAddresses:  "",
							RemoteAddresses: "",
							LocalPorts:      "",
							RemotePorts:     "",
							Priority:        222,
						},
						{
							ID:              "azure-acl-x-base2",
							Action:          "Allow",
							Direction:       "In",
							RemoteAddresses: testNodeIP,
							Priority:        201,
						},
					},
				},
			},
		},
	}

	otherTests := []*SerialTestCase{
		{
			Description: "ignore Pod update if added then deleted before ApplyDP()",
			Actions: []*Action{
				UpdatePolicy(policyXBaseOnK1V1()),
				CreatePod("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}),
				DeletePod("x", "a", ip1, map[string]string{"k1": "v1"}),
				ApplyDP(),
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					podCrudTag,
					netpolCrudTag,
				},
				DpCfg: defaultWindowsDPCfg,
				InitialEndpoints: []*hcn.HostComputeEndpoint{
					dptestutils.Endpoint(endpoint1, ip1),
				},
				ExpectedSetPolicies: []*hcn.SetPolicySetting{
					dptestutils.SetPolicy(emptySet),
					dptestutils.SetPolicy(allNamespaces, emptySet.GetHashedName(), nsXSet.GetHashedName()),
					dptestutils.SetPolicy(nsXSet),
					dptestutils.SetPolicy(podK1Set),
					dptestutils.SetPolicy(podK1V1Set),
				},
				ExpectedEnpdointACLs: map[string][]*hnswrapper.FakeEndpointPolicy{
					endpoint1: {},
				},
			},
		},
		{
			// doesn't really enforce behavior in DP, but one could look at logs to make sure we don't make a reset ACL SysCall into HNS
			Description: "ignore Pod delete for deleted endpoint",
			Actions: []*Action{
				UpdatePolicy(policyXBaseOnK1V1()),
				CreatePod("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}),
				ApplyDP(),
				DeleteEndpoint(endpoint1),
				DeletePod("x", "a", ip1, map[string]string{"k1": "v1"}),
				ApplyDP(),
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					podCrudTag,
					netpolCrudTag,
				},
				DpCfg: defaultWindowsDPCfg,
				InitialEndpoints: []*hcn.HostComputeEndpoint{
					dptestutils.Endpoint(endpoint1, ip1),
				},
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
			// doesn't really enforce behavior in DP, but one could look at logs to make sure we don't make a reset ACL SysCall into HNS
			Description: "ignore Pod delete for deleted endpoint (skip first ApplyDP())",
			Actions: []*Action{
				UpdatePolicy(policyXBaseOnK1V1()),
				CreatePod("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}),
				DeleteEndpoint(endpoint1),
				DeletePod("x", "a", ip1, map[string]string{"k1": "v1"}),
				ApplyDP(),
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					podCrudTag,
					netpolCrudTag,
				},
				DpCfg: defaultWindowsDPCfg,
				InitialEndpoints: []*hcn.HostComputeEndpoint{
					dptestutils.Endpoint(endpoint1, ip1),
				},
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
			// doesn't really enforce behavior in DP, but one could look at logs to make sure we don't make an add ACL SysCall into HNS"
			Description: "ignore Pod update when there's no corresponding endpoint",
			Actions: []*Action{
				UpdatePolicy(policyXBaseOnK1V1()),
				CreatePod("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}),
				DeleteEndpoint(endpoint1),
				ApplyDP(),
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					podCrudTag,
					netpolCrudTag,
				},
				DpCfg: defaultWindowsDPCfg,
				InitialEndpoints: []*hcn.HostComputeEndpoint{
					dptestutils.Endpoint(endpoint1, ip1),
				},
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
			Description: "two endpoints, one with policy, one without",
			Actions: []*Action{
				UpdatePolicy(policyXBase2OnK2V2()),
				CreatePod("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}),
				CreateEndpoint(endpoint2, ip2),
				CreatePod("x", "b", ip2, thisNode, map[string]string{"k2": "v2"}),
				ApplyDP(),
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					podCrudTag,
					netpolCrudTag,
				},
				DpCfg: defaultWindowsDPCfg,
				InitialEndpoints: []*hcn.HostComputeEndpoint{
					dptestutils.Endpoint(endpoint1, ip1),
				},
				ExpectedSetPolicies: []*hcn.SetPolicySetting{
					dptestutils.SetPolicy(emptySet),
					dptestutils.SetPolicy(allNamespaces, emptySet.GetHashedName(), nsXSet.GetHashedName()),
					dptestutils.SetPolicy(nsXSet, ip1, ip2),
					dptestutils.SetPolicy(podK1Set, ip1),
					dptestutils.SetPolicy(podK1V1Set, ip1),
					dptestutils.SetPolicy(podK2Set, ip2),
					dptestutils.SetPolicy(podK2V2Set, ip2),
				},
				ExpectedEnpdointACLs: map[string][]*hnswrapper.FakeEndpointPolicy{
					endpoint1: {},
					endpoint2: {
						{
							ID:              "azure-acl-x-base2",
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
							ID:              "azure-acl-x-base2",
							Protocols:       "",
							Action:          "Allow",
							Direction:       "Out",
							LocalAddresses:  "",
							RemoteAddresses: "",
							LocalPorts:      "",
							RemotePorts:     "",
							Priority:        222,
						},
						{
							ID:              "azure-acl-x-base2",
							Action:          "Allow",
							Direction:       "In",
							RemoteAddresses: testNodeIP,
							Priority:        201,
						},
					},
				},
			},
		},
	}

	allTests := sequence1Tests
	allTests = append(allTests, sequence2Tests...)
	// allTests = append(allTests, podAssignmentSequence3Tests()...)
	// make golint happy
	_ = podAssignmentSequence3Tests()
	allTests = append(allTests, otherTests...)
	return allTests
}

// sequence 3 of issue 1729
// seems like this sequence is impossible
// if it ever occurred, would need modifications in updatePod() and ipsetmanager
func podAssignmentSequence3Tests() []*SerialTestCase {
	return []*SerialTestCase{
		{
			Description: "Sequence 3: Pod B Create --> Pod A create --> Pod A Cleanup (ensure correct IPSets)",
			Actions: []*Action{
				CreatePod("x", "b", ip1, thisNode, map[string]string{"k2": "v2"}),
				ApplyDP(),
				CreatePod("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}),
				// UpdatePod() will fail for both x/a and x/b
				ApplyDP(),
				DeletePod("x", "a", ip1, map[string]string{"k1": "v1"}),
				ApplyDP(),
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					podCrudTag,
					netpolCrudTag,
				},
				DpCfg: defaultWindowsDPCfg,
				InitialEndpoints: []*hcn.HostComputeEndpoint{
					dptestutils.Endpoint(endpoint1, ip1),
				},
				ExpectedSetPolicies: []*hcn.SetPolicySetting{
					dptestutils.SetPolicy(emptySet),
					dptestutils.SetPolicy(allNamespaces, emptySet.GetHashedName(), nsXSet.GetHashedName()),
					dptestutils.SetPolicy(nsXSet, ip1),
					dptestutils.SetPolicy(podK2Set, ip1),
					dptestutils.SetPolicy(podK2V2Set, ip1),
					// not yet garbage-collected
					dptestutils.SetPolicy(podK1Set),
					dptestutils.SetPolicy(podK1V1Set),
				},
				ExpectedEnpdointACLs: map[string][]*hnswrapper.FakeEndpointPolicy{
					endpoint1: {},
				},
			},
		},
		{
			Description: "Sequence 3: Policy create --> Pod B Create --> Pod A create",
			Actions: []*Action{
				UpdatePolicy(policyXBaseOnK1V1()),
				UpdatePolicy(policyXBase2OnK2V2()),
				CreatePod("x", "b", ip1, thisNode, map[string]string{"k2": "v2"}),
				ApplyDP(),
				// UpdatePod() will fail for x/a since x/b is associated with the IP/Endpoint
				CreatePod("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}),
				ApplyDP(),
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					podCrudTag,
					netpolCrudTag,
				},
				DpCfg: defaultWindowsDPCfg,
				InitialEndpoints: []*hcn.HostComputeEndpoint{
					dptestutils.Endpoint(endpoint1, ip1),
				},
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
					endpoint1: {
						{
							ID:              "azure-acl-x-base2",
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
							ID:              "azure-acl-x-base2",
							Protocols:       "",
							Action:          "Allow",
							Direction:       "Out",
							LocalAddresses:  "",
							RemoteAddresses: "",
							LocalPorts:      "",
							RemotePorts:     "",
							Priority:        222,
						},
						{
							ID:              "azure-acl-x-base2",
							Action:          "Allow",
							Direction:       "In",
							RemoteAddresses: testNodeIP,
							Priority:        201,
						},
					},
				},
			},
		},
		{
			Description: "Sequence 3: Policy create --> Pod B Create --> Pod A create (skip first ApplyDP())",
			Actions: []*Action{
				UpdatePolicy(policyXBaseOnK1V1()),
				UpdatePolicy(policyXBase2OnK2V2()),
				CreatePod("x", "b", ip1, thisNode, map[string]string{"k2": "v2"}),
				CreatePod("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}),
				// UpdatePod() will fail for both x/a and x/b
				ApplyDP(),
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					podCrudTag,
					netpolCrudTag,
				},
				DpCfg: defaultWindowsDPCfg,
				InitialEndpoints: []*hcn.HostComputeEndpoint{
					dptestutils.Endpoint(endpoint1, ip1),
				},
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
					endpoint1: {},
				},
			},
		},
		{
			Description: "Sequence 3: Policy create --> Pod B Create --> Pod A create --> Pod A Cleanup",
			Actions: []*Action{
				UpdatePolicy(policyXBaseOnK1V1()),
				UpdatePolicy(policyXBase2OnK2V2()),
				CreatePod("x", "b", ip1, thisNode, map[string]string{"k2": "v2"}),
				CreatePod("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}),
				// UpdatePod() will fail for both x/a and x/b
				ApplyDP(),
				DeletePod("x", "a", ip1, map[string]string{"k1": "v1"}),
				ApplyDP(),
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					podCrudTag,
					netpolCrudTag,
				},
				DpCfg: defaultWindowsDPCfg,
				InitialEndpoints: []*hcn.HostComputeEndpoint{
					dptestutils.Endpoint(endpoint1, ip1),
				},
				ExpectedSetPolicies: []*hcn.SetPolicySetting{
					dptestutils.SetPolicy(emptySet),
					dptestutils.SetPolicy(allNamespaces, emptySet.GetHashedName(), nsXSet.GetHashedName()),
					dptestutils.SetPolicy(nsXSet, ip1),
					dptestutils.SetPolicy(podK2Set, ip1),
					dptestutils.SetPolicy(podK2V2Set, ip1),
					// not yet garbage-collected
					dptestutils.SetPolicy(podK1Set),
					dptestutils.SetPolicy(podK1V1Set),
				},
				ExpectedEnpdointACLs: map[string][]*hnswrapper.FakeEndpointPolicy{
					endpoint1: {},
				},
			},
		},
		{
			Description: "Sequence 3: Policy create --> Pod B Create --> Pod A create --> Pod B Update (unable to add second policy to endpoint until A cleanup)",
			Actions: []*Action{
				UpdatePolicy(policyXBaseOnK1V1()),
				UpdatePolicy(policyXBase2OnK2V2()),
				UpdatePolicy(policyXBase3OnK3V3()),
				CreatePod("x", "b", ip1, thisNode, map[string]string{"k2": "v2"}),
				ApplyDP(),
				// UpdatePod() will fail for x/a since x/b is associated with the IP/Endpoint
				CreatePod("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}),
				ApplyDP(),
				UpdatePodLabels("x", "b", ip1, thisNode, nil, map[string]string{"k3": "v3"}),
				ApplyDP(),
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					podCrudTag,
					netpolCrudTag,
				},
				DpCfg: defaultWindowsDPCfg,
				InitialEndpoints: []*hcn.HostComputeEndpoint{
					dptestutils.Endpoint(endpoint1, ip1),
				},
				ExpectedSetPolicies: []*hcn.SetPolicySetting{
					dptestutils.SetPolicy(emptySet),
					dptestutils.SetPolicy(allNamespaces, emptySet.GetHashedName(), nsXSet.GetHashedName()),
					dptestutils.SetPolicy(nsXSet, ip1),
					dptestutils.SetPolicy(podK1Set, ip1),
					dptestutils.SetPolicy(podK1V1Set, ip1),
					dptestutils.SetPolicy(podK2Set, ip1),
					dptestutils.SetPolicy(podK2V2Set, ip1),
					dptestutils.SetPolicy(podK3Set, ip1),
					dptestutils.SetPolicy(podK3V3Set, ip1),
				},
				ExpectedEnpdointACLs: map[string][]*hnswrapper.FakeEndpointPolicy{
					endpoint1: {
						{
							ID:              "azure-acl-x-base2",
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
							ID:              "azure-acl-x-base2",
							Protocols:       "",
							Action:          "Allow",
							Direction:       "Out",
							LocalAddresses:  "",
							RemoteAddresses: "",
							LocalPorts:      "",
							RemotePorts:     "",
							Priority:        222,
						},
						{
							ID:              "azure-acl-x-base2",
							Action:          "Allow",
							Direction:       "In",
							RemoteAddresses: testNodeIP,
							Priority:        201,
						},
					},
				},
			},
		},
		{
			Description: "Sequence 3: Policy create --> Pod B Create --> Pod A create --> Pod B Update --> Pod A cleanup (able to add second policy)",
			Actions: []*Action{
				UpdatePolicy(policyXBaseOnK1V1()),
				UpdatePolicy(policyXBase2OnK2V2()),
				UpdatePolicy(policyXBase3OnK3V3()),
				CreatePod("x", "b", ip1, thisNode, map[string]string{"k2": "v2"}),
				ApplyDP(),
				// UpdatePod() will fail for x/a since x/b is associated with the IP/Endpoint
				CreatePod("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}),
				ApplyDP(),
				UpdatePodLabels("x", "b", ip1, thisNode, nil, map[string]string{"k3": "v3"}),
				ApplyDP(),
				DeletePod("x", "a", ip1, map[string]string{"k1": "v1"}),
				ApplyDP(),
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					podCrudTag,
					netpolCrudTag,
				},
				DpCfg: defaultWindowsDPCfg,
				InitialEndpoints: []*hcn.HostComputeEndpoint{
					dptestutils.Endpoint(endpoint1, ip1),
				},
				ExpectedSetPolicies: []*hcn.SetPolicySetting{
					dptestutils.SetPolicy(emptySet),
					dptestutils.SetPolicy(allNamespaces, emptySet.GetHashedName(), nsXSet.GetHashedName()),
					dptestutils.SetPolicy(nsXSet, ip1),
					dptestutils.SetPolicy(podK2Set, ip1),
					dptestutils.SetPolicy(podK2V2Set, ip1),
					dptestutils.SetPolicy(podK3Set, ip1),
					dptestutils.SetPolicy(podK3V3Set, ip1),
					// not garbage-collected yet
					dptestutils.SetPolicy(podK1Set),
					dptestutils.SetPolicy(podK1V1Set),
				},
				ExpectedEnpdointACLs: map[string][]*hnswrapper.FakeEndpointPolicy{
					endpoint1: {
						{
							ID:              "azure-acl-x-base2",
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
							ID:              "azure-acl-x-base2",
							Protocols:       "",
							Action:          "Allow",
							Direction:       "Out",
							LocalAddresses:  "",
							RemoteAddresses: "",
							LocalPorts:      "",
							RemotePorts:     "",
							Priority:        222,
						},
						{
							ID:              "azure-acl-x-base2",
							Action:          "Allow",
							Direction:       "In",
							RemoteAddresses: testNodeIP,
							Priority:        201,
						},
						{
							ID:              "azure-acl-x-base3",
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
							ID:              "azure-acl-x-base3",
							Protocols:       "",
							Action:          "Allow",
							Direction:       "Out",
							LocalAddresses:  "",
							RemoteAddresses: "",
							LocalPorts:      "",
							RemotePorts:     "",
							Priority:        222,
						},
						{
							ID:              "azure-acl-x-base3",
							Action:          "Allow",
							Direction:       "In",
							RemoteAddresses: testNodeIP,
							Priority:        201,
						},
					},
				},
			},
		},
	}
}

func remoteEndpointTests() []*SerialTestCase {
	return []*SerialTestCase{
		{
			// updatePod cache will not be updated for a Pod off-node
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
				ExpectedEnpdointACLs: nil,
			},
		},
		{
			// updatePod cache will not be updated for a Pod off-node
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
				ExpectedEnpdointACLs: nil,
			},
		},
		{
			Description: "don't track remote endpoint",
			Actions: []*Action{
				CreatePod("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}),
				UpdatePolicy(policyXBaseOnK1V1()),
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					podCrudTag,
					netpolCrudTag,
				},
				DpCfg: defaultWindowsDPCfg,
				InitialEndpoints: []*hcn.HostComputeEndpoint{
					dptestutils.RemoteEndpoint(endpoint1, ip1),
				},
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
			Description: "add policy to correct endpoint e.g. when an old endpoint isn't deleted",
			Actions: []*Action{
				CreatePod("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}),
				UpdatePolicy(policyXBaseOnK1V1()),
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					podCrudTag,
					netpolCrudTag,
				},
				DpCfg: defaultWindowsDPCfg,
				InitialEndpoints: []*hcn.HostComputeEndpoint{
					dptestutils.RemoteEndpoint(endpoint1, ip1),
					dptestutils.Endpoint(endpoint2, ip1),
				},
				ExpectedSetPolicies: []*hcn.SetPolicySetting{
					dptestutils.SetPolicy(emptySet),
					dptestutils.SetPolicy(allNamespaces, emptySet.GetHashedName(), nsXSet.GetHashedName()),
					dptestutils.SetPolicy(nsXSet, ip1),
					dptestutils.SetPolicy(podK1Set, ip1),
					dptestutils.SetPolicy(podK1V1Set, ip1),
				},
				ExpectedEnpdointACLs: map[string][]*hnswrapper.FakeEndpointPolicy{
					endpoint2: {
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
						{
							ID:              "azure-acl-x-base",
							Action:          "Allow",
							Direction:       "In",
							RemoteAddresses: testNodeIP,
							Priority:        201,
						},
					},
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
						{
							ID:              "azure-acl-x-base",
							Action:          "Allow",
							Direction:       "In",
							RemoteAddresses: testNodeIP,
							Priority:        201,
						},
					},
				},
			},
		},
	}
}

func applyInBackgroundTests() []*SerialTestCase {
	allTests := make([]*SerialTestCase, 0)
	allTests = append(allTests, basicTests()...)
	allTests = append(allTests, capzCalicoTests()...)
	allTests = append(allTests, updatePodTests()...)

	for _, test := range allTests {
		test.TestCaseMetadata.Tags = append(test.TestCaseMetadata.Tags, applyInBackgroundTag)
		cfg := *test.DpCfg
		cfg.ApplyInBackground = true
		cfg.ApplyMaxBatches = 3
		cfg.ApplyInterval = time.Duration(50 * time.Millisecond)
		test.DpCfg = &cfg
	}

	return allTests
}

func multiJobApplyInBackgroundTests() []*MultiJobTestCase {
	allTests := make([]*MultiJobTestCase, 0)
	allTests = append(allTests, getAllMultiJobTests()...)

	for _, test := range allTests {
		test.TestCaseMetadata.Tags = append(test.TestCaseMetadata.Tags, applyInBackgroundTag)
		cfg := *test.DpCfg
		cfg.ApplyInBackground = true
		cfg.ApplyMaxBatches = 3
		cfg.ApplyInterval = time.Duration(50 * time.Millisecond)
		test.DpCfg = &cfg
	}

	return allTests
}
