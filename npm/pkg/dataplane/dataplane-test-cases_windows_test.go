package dataplane

import (
	"github.com/Azure/azure-container-networking/network/hnswrapper"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
	"github.com/Microsoft/hcsshim/hcn"

	dptestutils "github.com/Azure/azure-container-networking/npm/pkg/dataplane/testutils"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// tags
const (
	podCrudTag    Tag = "pod-crud"
	nsCrudTag     Tag = "namespace-crud"
	netpolCrudTag Tag = "netpol-crud"
	backgroundTag Tag = "has-background-steps"
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
	nsXSet        = ipsets.NewIPSetMetadata("ns1", ipsets.Namespace)
	nsYSet        = ipsets.NewIPSetMetadata("ns2", ipsets.Namespace)

	nsK1Set   = ipsets.NewIPSetMetadata("k1", ipsets.KeyLabelOfNamespace)
	nsK1V1Set = ipsets.NewIPSetMetadata("k1:v1", ipsets.KeyValueLabelOfNamespace)
	nsK2Set   = ipsets.NewIPSetMetadata("k1", ipsets.KeyLabelOfNamespace)
	nsK2V2Set = ipsets.NewIPSetMetadata("k1:v1", ipsets.KeyValueLabelOfNamespace)
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

func getAllSerialTests() []*SerialTestCase {
	return []*SerialTestCase{
		{
			Description: "pod x/a created, then relevant network policy created",
			Actions: []*Action{
				CreatePod("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}),
				UpdatePolicy(policyNs1LabelPair1AllowAll()),
			},
			TestCaseMetadata: &TestCaseMetadata{
				Tags: []Tag{
					podCrudTag,
					netpolCrudTag,
					backgroundTag,
				},
				DpCfg: defaultWindowsDPCfg,
				InitialEndpoints: []*hcn.HostComputeEndpoint{
					dptestutils.Endpoint(endpoint1, ip1),
				},
				ExpectedSetPolicies: []*hcn.SetPolicySetting{
					dptestutils.SetPolicy(emptySet),
					dptestutils.SetPolicy(allNamespaces, nsXSet.GetHashedName(), emptySet.GetHashedName()),
					dptestutils.SetPolicy(nsXSet, ip1),
					dptestutils.SetPolicy(podK1Set, ip1),
					dptestutils.SetPolicy(podK1V1Set, ip1),
				},
				ExpectedEnpdointACLs: map[string][]*hnswrapper.FakeEndpointPolicy{
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
		},
	}
}

func getAllMultiRoutineTests() []*MultiRoutineTestCase {
	return []*MultiRoutineTestCase{
		{
			Description: "pod x/a created, then relevant network policy created",
			Routines: map[string][]*Action{
				"pod_controller": {
					CreatePod("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}),
					CreatePod("y", "a", ip2, otherNode, map[string]string{"k2": "v2"}),
				},
				"policy_controller": {
					UpdatePolicy(policyNs1LabelPair1AllowAll()),
				},
				"namespace_controller": {},
			},
			// would fill metadata out for actual test case
			TestCaseMetadata: nil,
		},
	}
}
