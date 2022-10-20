package e2e

import (
	"github.com/Azure/azure-container-networking/network/hnswrapper"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane"
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
	policyCrudTag Tag = "policy-crud"
	backgroundTag Tag = "has-background-steps"
)

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
)

// IPSet constants
var (
	podLabel1Set    = ipsets.NewIPSetMetadata("k1", ipsets.KeyLabelOfPod)
	podLabelVal1Set = ipsets.NewIPSetMetadata("k1:v1", ipsets.KeyValueLabelOfPod)
	podLabel2Set    = ipsets.NewIPSetMetadata("k2", ipsets.KeyLabelOfPod)
	podLabelVal2Set = ipsets.NewIPSetMetadata("k2:v2", ipsets.KeyValueLabelOfPod)

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

func getAllTests() []*TestCaseMetadata {
	return []*TestCaseMetadata{
		{
			Description: "test case 1",
			Tags: []Tag{
				podCrudTag,
				backgroundTag,
			},
			InitialEndpoints: []*hcn.HostComputeEndpoint{
				dptestutils.Endpoint(endpoint1, ip1),
			},
			TestCase: NewTestCase([]*TestStep{
				{
					ID:           "pod controller 1",
					InBackground: false,
					Action: &PodCreateAction{
						Pod:       dataplane.NewPodMetadata(podKey1, ip1, thisNode),
						Namespace: "ns1",
						Labels: map[string]string{
							"k1": "v1",
						},
					},
				},
				{
					ID:           "policy controller 1",
					InBackground: true,
					Action: &PolicyUpdateAction{
						Policy: policyNs1LabelPair1AllowAll(),
					},
				},
				{
					ID:           "pod controller 2",
					InBackground: true,
					Action: &PodCreateAction{
						Pod:       dataplane.NewPodMetadata(podKey1, ip1, thisNode),
						Namespace: "ns1",
						Labels: map[string]string{
							"k1": "v1",
						},
					},
				},
				{
					ID:           "policy controller 2",
					InBackground: false,
					Action: &PolicyDeleteAction{
						Namespace: policyNs1LabelPair1AllowAll().Namespace,
						Name:      policyNs1LabelPair1AllowAll().Name,
					},
				},
			}, map[string][]string{
				// "policy controller 2" action won't run until both "policy controller 1" and "pod controller 2" are done
				"policy controller 2": {
					"policy controller 1",
					"pod controller 2",
				},
			}),
			ExpectedSetPolicies: []*hcn.SetPolicySetting{
				dptestutils.SetPolicy(emptySet),
				dptestutils.SetPolicy(allNamespaces, ns1Set.GetHashedName(), emptySet.GetHashedName()),
				dptestutils.SetPolicy(ns1Set, ip1),
				dptestutils.SetPolicy(podLabel1Set, ip1),
				dptestutils.SetPolicy(podLabelVal1Set, ip1),
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
