package e2e

import (
	"github.com/Azure/azure-container-networking/network/hnswrapper"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
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

func getAllTests() []*TestCaseMetadata {
	return []*TestCaseMetadata{
		{
			Description: "test case 1",
			Tags: []Tag{
				podCrudTag,
				netpolCrudTag,
				backgroundTag,
			},
			InitialEndpoints: []*hcn.HostComputeEndpoint{
				dptestutils.Endpoint(endpoint1, ip1),
			},
			TestCase: NewTestCase([]*TestStep{
				{
					ID:           "pod1",
					InBackground: false,
					Action:       CreatePod("x", "a", ip1, thisNode, map[string]string{"k1": "v1"}),
				},
				{
					ID:           "netpol1",
					InBackground: true,
					Action:       UpdatePolicy(policyNs1LabelPair1AllowAll()),
				},
				{
					ID:           "netpol2",
					InBackground: true,
					Action:       UpdatePolicy(policyNs1LabelPair1AllowAll()),
				},
				{
					ID:           "pod2",
					InBackground: true,
					Action:       CreatePod("x", "b", ip2, otherNode, map[string]string{"k2": "v2"}),
				},
				{
					ID:           "pod3",
					InBackground: true,
					Action:       CreatePod("y", "a", ip2, otherNode, map[string]string{"k1": "v1"}),
				},
				{
					ID:           "netpol3",
					InBackground: false,
					Action:       DeletePolicyByObject(policyNs1LabelPair1AllowAll()),
				},
			}, map[string][]string{
				// netpol2 won't run until netpol1 is complete
				"netpol2": {"netpol1"},
				// pod3 won't run until pod2 is complete
				"pod3": {"pod2"},
				// netpol3 won't run until all background threads have complete
				"netpol3": {
					"netpol2",
					"pod3",
				},
			}),
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
	}
}

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
