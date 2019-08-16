package npm

import (
	"testing"
	"reflect"
	"encoding/json"

	"github.com/Azure/azure-container-networking/npm/iptm"
	"github.com/Azure/azure-container-networking/npm/util"
	"k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestCraftPartialIptEntrySpecFromOpAndLabel(t *testing.T) {
	srcOp, srcLabel := "", "src"
	iptEntry := craftPartialIptEntrySpecFromOpAndLabel(srcOp, srcLabel, util.IptablesSrcFlag, false)
	expectedIptEntry := []string{
		util.IptablesModuleFlag,
		util.IptablesSetModuleFlag,
		util.IptablesMatchSetFlag,
		util.GetHashedName(srcLabel),
		util.IptablesSrcFlag,
	}
	
	if !reflect.DeepEqual(iptEntry, expectedIptEntry) {
		t.Errorf("TestCraftIptEntrySpecFromOpAndLabel failed @ src iptEntry comparison")
	}

	dstOp, dstLabel := "!", "dst"
	iptEntry = craftPartialIptEntrySpecFromOpAndLabel(dstOp, dstLabel, util.IptablesDstFlag, false)
	expectedIptEntry = []string{
		util.IptablesModuleFlag,
		util.IptablesSetModuleFlag,
		util.IptablesNotFlag,
		util.IptablesMatchSetFlag,
		util.GetHashedName(dstLabel),
		util.IptablesDstFlag,
	}
	
	if !reflect.DeepEqual(iptEntry, expectedIptEntry) {
		t.Errorf("TestCraftIptEntrySpecFromOpAndLabel failed @ dst iptEntry comparison")
	}
}

func TestCraftPartialIptEntrySpecFromOpsAndLabels(t *testing.T) {
	srcOps := []string{
		"",
		"",
		"!",
	}
	srcLabels := []string{
		"src",
		"src:firstLabel",
		"src:secondLabel",
	}

	dstOps := []string{
		"!",
		"!",
		"",
	}
	dstLabels := []string{
		"dst",
		"dst:firstLabel",
		"dst:secondLabel",
	}


	srcIptEntry := craftPartialIptEntrySpecFromOpsAndLabels(srcOps, srcLabels, util.IptablesSrcFlag, false)
	dstIptEntry := craftPartialIptEntrySpecFromOpsAndLabels(dstOps, dstLabels, util.IptablesDstFlag, false)
	iptEntrySpec := append(srcIptEntry, dstIptEntry...)
	expectedIptEntrySpec := []string{
		util.IptablesModuleFlag,
		util.IptablesSetModuleFlag,
		util.IptablesMatchSetFlag,
		util.GetHashedName("src"),
		util.IptablesSrcFlag,
		util.IptablesModuleFlag,
		util.IptablesSetModuleFlag,
		util.IptablesMatchSetFlag,
		util.GetHashedName("src:firstLabel"),
		util.IptablesSrcFlag,
		util.IptablesModuleFlag,
		util.IptablesSetModuleFlag,
		util.IptablesNotFlag,
		util.IptablesMatchSetFlag,
		util.GetHashedName("src:secondLabel"),
		util.IptablesSrcFlag,
		util.IptablesModuleFlag,
		util.IptablesSetModuleFlag,
		util.IptablesNotFlag,
		util.IptablesMatchSetFlag,
		util.GetHashedName("dst"),
		util.IptablesDstFlag,
		util.IptablesModuleFlag,
		util.IptablesSetModuleFlag,
		util.IptablesNotFlag,
		util.IptablesMatchSetFlag,
		util.GetHashedName("dst:firstLabel"),
		util.IptablesDstFlag,
		util.IptablesModuleFlag,
		util.IptablesSetModuleFlag,
		util.IptablesMatchSetFlag,
		util.GetHashedName("dst:secondLabel"),
		util.IptablesDstFlag,
	}

	if !reflect.DeepEqual(iptEntrySpec, expectedIptEntrySpec) {
		t.Errorf("TestCraftIptEntrySpecFromOpsAndLabels failed @ iptEntrySpec comparison")
		t.Errorf("iptEntrySpec:\n%v", iptEntrySpec)
		t.Errorf("expectedIptEntrySpec:\n%v", expectedIptEntrySpec)
	}
}

func TestCraftPartialIptEntryFromSelector(t *testing.T) {
	srcSelector := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"label": "src",
		},
		MatchExpressions: []metav1.LabelSelectorRequirement{
			metav1.LabelSelectorRequirement{
				Key:      "labelNotIn",
				Operator: metav1.LabelSelectorOpNotIn,
				Values: []string{
					"src",
				},
			},
		},
	}

	iptEntrySpec := craftPartialIptEntrySpecFromSelector(srcSelector, util.IptablesSrcFlag, false)
	expectedIptEntrySpec := []string{
		util.IptablesModuleFlag,
		util.IptablesSetModuleFlag,
		util.IptablesMatchSetFlag,
		util.GetHashedName("label:src"),
		util.IptablesSrcFlag,
		util.IptablesModuleFlag,
		util.IptablesSetModuleFlag,
		util.IptablesNotFlag,
		util.IptablesMatchSetFlag,
		util.GetHashedName("labelNotIn:src"),
		util.IptablesSrcFlag,
	}

	if !reflect.DeepEqual(iptEntrySpec, expectedIptEntrySpec) {
		t.Errorf("TestCraftPartialIptEntryFromSelector failed @ iptEntrySpec comparison")
		t.Errorf("iptEntrySpec:\n%v", iptEntrySpec)
		t.Errorf("expectedIptEntrySpec:\n%v", expectedIptEntrySpec)
	}
}

func TestCraftPartialIptablesCommentFromSelector(t *testing.T) {
	var selector *metav1.LabelSelector
	selector = nil
	comment := craftPartialIptablesCommentFromSelector(selector, false)
	expectedComment := "none"
	if comment != expectedComment {
		t.Errorf("TestCraftPartialIptablesCommentFromSelector failed @ nil selector comparison")
		t.Errorf("comment:\n%v", comment)
		t.Errorf("expectedComment:\n%v", expectedComment)
	}

	selector = &metav1.LabelSelector{}
	comment = craftPartialIptablesCommentFromSelector(selector, false)
	expectedComment = util.KubeAllNamespacesFlag
	if comment != expectedComment {
		t.Errorf("TestCraftPartialIptablesCommentFromSelector failed @ empty selector comparison")
		t.Errorf("comment:\n%v", comment)
		t.Errorf("expectedComment:\n%v", expectedComment)
	}

	selector = &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"k0": "v0",
		},
		MatchExpressions: []metav1.LabelSelectorRequirement{
			metav1.LabelSelectorRequirement{
				Key: "k1",
				Operator: metav1.LabelSelectorOpIn,
				Values: []string{
					"v10",
					"v11",
				},
			},
			metav1.LabelSelectorRequirement{
				Key: "k2",
				Operator: metav1.LabelSelectorOpDoesNotExist,
				Values: []string{},
			},
		},
	}
	comment = craftPartialIptablesCommentFromSelector(selector, false)
	expectedComment = "k0:v0-AND-k1:v10-AND-k1:v11-AND-!k2"
	if comment != expectedComment {
		t.Errorf("TestCraftPartialIptablesCommentFromSelector failed @ normal selector comparison")
		t.Errorf("comment:\n%v", comment)
		t.Errorf("expectedComment:\n%v", expectedComment)
	}

	nsSelector := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"k0": "v0",
		},
		MatchExpressions: []metav1.LabelSelectorRequirement{
			metav1.LabelSelectorRequirement{
				Key: "k1",
				Operator: metav1.LabelSelectorOpIn,
				Values: []string{
					"v10",
					"v11",
				},
			},
			metav1.LabelSelectorRequirement{
				Key: "k2",
				Operator: metav1.LabelSelectorOpDoesNotExist,
				Values: []string{},
			},
		},
	}
	comment = craftPartialIptablesCommentFromSelector(nsSelector, true)
	expectedComment = "ns-k0:v0-AND-ns-k1:v10-AND-ns-k1:v11-AND-ns-!k2"
	if comment != expectedComment {
		t.Errorf("TestCraftPartialIptablesCommentFromSelector failed @ namespace selector comparison")
		t.Errorf("comment:\n%v", comment)
		t.Errorf("expectedComment:\n%v", expectedComment)
	}

}

func TestTranslateIngress(t *testing.T) {
	ns := "testnamespace"

	targetSelector := metav1.LabelSelector{
		MatchLabels: map[string]string{
			"context": "dev",
		},
		MatchExpressions: []metav1.LabelSelectorRequirement{
			metav1.LabelSelectorRequirement{
				Key:      "testNotIn",
				Operator: metav1.LabelSelectorOpNotIn,
				Values: []string{
				"frontend",
				},
			},
		},
	}

	tcp := v1.ProtocolTCP
	port6783 := intstr.FromInt(6783)
	ingressPodSelector := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app": "db",
		},
		MatchExpressions: []metav1.LabelSelectorRequirement{
			metav1.LabelSelectorRequirement{
				Key:      "testIn",
				Operator: metav1.LabelSelectorOpIn,
				Values: []string{
					"frontend",
				},
			},
		},
	}
	ingressNamespaceSelector := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"ns": "dev",
		},
		MatchExpressions: []metav1.LabelSelectorRequirement{
			metav1.LabelSelectorRequirement{
				Key:      "testIn",
				Operator: metav1.LabelSelectorOpIn,
				Values: []string{
					"frontendns",
				},
			},
		},
	}

	compositeNetworkPolicyPeer := networkingv1.NetworkPolicyPeer{
		PodSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"region": "northpole",
			},
			MatchExpressions: []metav1.LabelSelectorRequirement{
				metav1.LabelSelectorRequirement{
					Key:      "k",
					Operator: metav1.LabelSelectorOpDoesNotExist,
				},
			},
		},
		NamespaceSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"planet": "earth",
			},
			MatchExpressions: []metav1.LabelSelectorRequirement{
				metav1.LabelSelectorRequirement{
					Key:      "keyExists",
					Operator: metav1.LabelSelectorOpExists,
				},
			},
		},
	}

	rules := []networkingv1.NetworkPolicyIngressRule{
		networkingv1.NetworkPolicyIngressRule{
			Ports: []networkingv1.NetworkPolicyPort{
				networkingv1.NetworkPolicyPort{
					Protocol: &tcp,
					Port:     &port6783,
				},
			},
			From: []networkingv1.NetworkPolicyPeer{
				networkingv1.NetworkPolicyPeer{
					PodSelector: ingressPodSelector,
				},
				networkingv1.NetworkPolicyPeer{
					NamespaceSelector: ingressNamespaceSelector,
				},
				compositeNetworkPolicyPeer,
			},
		},
	}

	util.IsNewNwPolicyVerFlag = true
	sets, lists, iptEntries := translateIngress(ns, targetSelector, rules)
	expectedSets := []string{
		"context:dev",
		"testNotIn:frontend",
		"app:db",
		"testIn:frontend",
		"region:northpole",
		"k",
	}

	if !reflect.DeepEqual(sets, expectedSets) {
		t.Errorf("translatedIngress failed @ sets comparison")
		t.Errorf("sets: %v", sets)
		t.Errorf("expectedSets: %v", expectedSets)
	}

	expectedLists := []string{
		"ns-ns:dev",
		"ns-testIn:frontendns",
		"ns-planet:earth",
		"ns-keyExists",
	}

	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedIngress failed @ lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	expectedIptEntries := []*iptm.IptEntry{
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressPortChain,
			Specs: []string{
				util.IptablesProtFlag,
				string(v1.ProtocolTCP),
				util.IptablesDstPortFlag,
				"6783",
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("context:dev"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesNotFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("testNotIn:frontend"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAzureIngressFromChain,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ALL-TO-6783-PORT-OF-context:dev-AND-!testNotIn:frontend-TO-JUMP-TO-" +
				util.IptablesAzureIngressFromChain,
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressFromChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:db"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("testIn:frontend"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("context:dev"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesNotFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("testNotIn:frontend"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAccept,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-app:db-AND-testIn:frontend-TO-context:dev-AND-!testNotIn:frontend",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressFromChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-ns:dev"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-testIn:frontendns"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("context:dev"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesNotFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("testNotIn:frontend"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAccept,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ns-ns:dev-AND-ns-testIn:frontendns-TO-context:dev-AND-!testNotIn:frontend",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressFromChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-planet:earth"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-keyExists"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("region:northpole"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesNotFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("k"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("context:dev"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesNotFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("testNotIn:frontend"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAccept,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ns-planet:earth-AND-ns-keyExists-AND-region:northpole-AND-!k-TO-context:dev-AND-!testNotIn:frontend",
			},
		},
	}

	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("translatedIngress failed @ composite ingress rule comparison")
		marshalledIptEntries, _ := json.Marshal(iptEntries)
		marshalledExpectedIptEntries, _ := json.Marshal(expectedIptEntries)
		t.Errorf("iptEntries: %s", marshalledIptEntries)
		t.Errorf("expectedIptEntries: %s", marshalledExpectedIptEntries)
	}
}

func TestTranslateEgress(t *testing.T) {
	ns := "testnamespace"

	targetSelector := metav1.LabelSelector{
		MatchLabels: map[string]string{
			"context": "dev",
		},
		MatchExpressions: []metav1.LabelSelectorRequirement{
			metav1.LabelSelectorRequirement{
				Key:      "testNotIn",
				Operator: metav1.LabelSelectorOpNotIn,
				Values: []string{
				"frontend",
				},
			},
		},
	}

	tcp := v1.ProtocolTCP
	port6783 := intstr.FromInt(6783)
	egressPodSelector := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app": "db",
		},
		MatchExpressions: []metav1.LabelSelectorRequirement{
			metav1.LabelSelectorRequirement{
				Key:      "testIn",
				Operator: metav1.LabelSelectorOpIn,
				Values: []string{
					"frontend",
				},
			},
		},
	}
	egressNamespaceSelector := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"ns": "dev",
		},
		MatchExpressions: []metav1.LabelSelectorRequirement{
			metav1.LabelSelectorRequirement{
				Key:      "testIn",
				Operator: metav1.LabelSelectorOpIn,
				Values: []string{
					"frontendns",
				},
			},
		},
	}

	compositeNetworkPolicyPeer := networkingv1.NetworkPolicyPeer{
		PodSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"region": "northpole",
			},
			MatchExpressions: []metav1.LabelSelectorRequirement{
				metav1.LabelSelectorRequirement{
					Key:      "k",
					Operator: metav1.LabelSelectorOpDoesNotExist,
				},
			},
		},
		NamespaceSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"planet": "earth",
			},
			MatchExpressions: []metav1.LabelSelectorRequirement{
				metav1.LabelSelectorRequirement{
					Key:      "keyExists",
					Operator: metav1.LabelSelectorOpExists,
				},
			},
		},
	}

	rules := []networkingv1.NetworkPolicyEgressRule{
		networkingv1.NetworkPolicyEgressRule{
			Ports: []networkingv1.NetworkPolicyPort{
				networkingv1.NetworkPolicyPort{
					Protocol: &tcp,
					Port:     &port6783,
				},
			},
			To: []networkingv1.NetworkPolicyPeer{
				networkingv1.NetworkPolicyPeer{
					PodSelector: egressPodSelector,
				},
				networkingv1.NetworkPolicyPeer{
					NamespaceSelector: egressNamespaceSelector,
				},
				compositeNetworkPolicyPeer,
			},
		},
	}

	util.IsNewNwPolicyVerFlag = true
	sets, lists, iptEntries := translateEgress(ns, targetSelector, rules)
	expectedSets := []string{
		"context:dev",
		"testNotIn:frontend",
		"app:db",
		"testIn:frontend",
		"region:northpole",
		"k",
	}

	if !reflect.DeepEqual(sets, expectedSets) {
		t.Errorf("translatedEgress failed @ sets comparison")
		t.Errorf("sets: %v", sets)
		t.Errorf("expectedSets: %v", expectedSets)
	}

	expectedLists := []string{
		"ns-ns:dev",
		"ns-testIn:frontendns",
		"ns-planet:earth",
		"ns-keyExists",
	}

	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedEgress failed @ lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	expectedIptEntries := []*iptm.IptEntry{
		&iptm.IptEntry{
			Chain: util.IptablesAzureEgressPortChain,
			Specs: []string{
				util.IptablesProtFlag,
				string(v1.ProtocolTCP),
				util.IptablesDstPortFlag,
				"6783",
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("context:dev"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesNotFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("testNotIn:frontend"),
				util.IptablesSrcFlag,
				util.IptablesJumpFlag,
				util.IptablesAzureEgressToChain,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ALL-FROM-6783-PORT-OF-context:dev-AND-!testNotIn:frontend-TO-JUMP-TO-" +
				util.IptablesAzureEgressToChain,
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureEgressToChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("context:dev"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesNotFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("testNotIn:frontend"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:db"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("testIn:frontend"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAccept,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-context:dev-AND-!testNotIn:frontend-TO-app:db-AND-testIn:frontend",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureEgressToChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("context:dev"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesNotFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("testNotIn:frontend"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-ns:dev"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-testIn:frontendns"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAccept,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-context:dev-AND-!testNotIn:frontend-TO-ns-ns:dev-AND-ns-testIn:frontendns",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureEgressToChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("context:dev"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesNotFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("testNotIn:frontend"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-planet:earth"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-keyExists"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("region:northpole"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesNotFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("k"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAccept,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-context:dev-AND-!testNotIn:frontend-TO-ns-planet:earth-AND-ns-keyExists-AND-region:northpole-AND-!k",
			},
		},
	}

	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("translatedEgress failed @ composite egress rule comparison")
		marshalledIptEntries, _ := json.Marshal(iptEntries)
		marshalledExpectedIptEntries, _ := json.Marshal(expectedIptEntries)
		t.Errorf("iptEntries: %s", marshalledIptEntries)
		t.Errorf("expectedIptEntries: %s", marshalledExpectedIptEntries)
	}
}

func TestTranslatePolicy(t *testing.T) {
	targetSelector := metav1.LabelSelector{}
	denyAllPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "deny-all-policy",
			Namespace: "testnamespace",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: targetSelector,
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{},
		},
	}

	sets, lists, iptEntries := translatePolicy(denyAllPolicy)

	expectedSets := []string{"ns-testnamespace"}
	if !reflect.DeepEqual(sets, expectedSets) {
		t.Errorf("translatedPolicy failed @ deny-all-policy sets comparison")
		t.Errorf("sets: %v", sets)
		t.Errorf("expectedSets: %v", expectedSets)
	}

	expectedLists := []string{}
	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedPolicy failed @ deny-all-policy lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	/*
	expectedIptEntries = []*iptm.IptEntry{
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressPortChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-testnamespace"),
				util.IptablesSrcFlag,
				util.IptablesJumpFlag,
				util.IptablesAccept,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ALL-TO-ns-testnamespace",
			},
		},
	}
	*/
	expectedIptEntries := []*iptm.IptEntry{}
	expectedIptEntries = append(
		expectedIptEntries,
		getAllowKubeSystemEntries("testnamespace", targetSelector)...,
	)
	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("translatedPolicy failed @ deny-all-policy policy comparison")
		marshalledIptEntries, _ := json.Marshal(iptEntries)
		marshalledExpectedIptEntries, _ := json.Marshal(expectedIptEntries)
		t.Errorf("iptEntries: %s", marshalledIptEntries)
		t.Errorf("expectedIptEntries: %s", marshalledExpectedIptEntries)
	}

	targetSelector = metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app": "backend",
		},
	}
	allowFromPodsPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "allow-from-pods-policy",
			Namespace: "testnamespace",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: targetSelector,
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				networkingv1.NetworkPolicyIngressRule{
					From: []networkingv1.NetworkPolicyPeer{
						networkingv1.NetworkPolicyPeer{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "frontend",
								},
							},
						},
					},
				},
			},
		},
	}

	sets, lists, iptEntries = translatePolicy(allowFromPodsPolicy)

	expectedSets = []string{
		"app:backend",
		"app:frontend",
	}
	if !reflect.DeepEqual(sets, expectedSets) {
		t.Errorf("translatedPolicy failed @ deny-all-policy sets comparison")
		t.Errorf("sets: %v", sets)
		t.Errorf("expectedSets: %v", expectedSets)
	}

	expectedLists = []string{}
	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedPolicy failed @ deny-all-policy lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	expectedIptEntries = []*iptm.IptEntry{}
	expectedIptEntries = append(
		expectedIptEntries,
		getAllowKubeSystemEntries("testnamespace", targetSelector)...,
	)

	nonKubeSystemEntries := []*iptm.IptEntry{
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressPortChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:backend"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAzureIngressFromChain,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ALL-TO-app:backend-TO-JUMP-TO-" +
				util.IptablesAzureIngressFromChain,
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressFromChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:frontend"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:backend"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAccept,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-app:frontend-TO-app:backend",
			},
		},
	}
	expectedIptEntries = append(expectedIptEntries, nonKubeSystemEntries...)
	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("translatedPolicy failed @ allow-from-pods-policy policy comparison")
		marshalledIptEntries, _ := json.Marshal(iptEntries)
		marshalledExpectedIptEntries, _ := json.Marshal(expectedIptEntries)
		t.Errorf("iptEntries: %s", marshalledIptEntries)
		t.Errorf("expectedIptEntries: %s", marshalledExpectedIptEntries)
	}

	targetSelector = metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app": "frontend",
		},
	}
	allowToFrontendPodsPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "allow-all-to-app:frontend-policy",
			Namespace: "testnamespace",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: targetSelector,
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				networkingv1.NetworkPolicyIngressRule{},
			},
		},
	}

	sets, lists, iptEntries = translatePolicy(allowToFrontendPodsPolicy)

	expectedSets = []string{
		"app:frontend",
	}
	if !reflect.DeepEqual(sets, expectedSets) {
		t.Errorf("translatedPolicy failed @ allow-all-to-app:frontend-policy sets comparison")
		t.Errorf("sets: %v", sets)
		t.Errorf("expectedSets: %v", expectedSets)
	}

	expectedLists = []string{
		util.KubeAllNamespacesFlag,
	}
	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedPolicy failed @ allow-all-to-app:frontend-policy lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	expectedIptEntries = []*iptm.IptEntry{}
	expectedIptEntries = append(
		expectedIptEntries,
		getAllowKubeSystemEntries("testnamespace", targetSelector)...,
	)

	nonKubeSystemEntries = []*iptm.IptEntry{
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressPortChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:frontend"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAccept,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ALL-TO-app:frontend",
			},
		},
	}
	expectedIptEntries = append(expectedIptEntries, nonKubeSystemEntries...)
	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("translatedPolicy failed @ allow-all-to-app:frontend-policy policy comparison")
		marshalledIptEntries, _ := json.Marshal(iptEntries)
		marshalledExpectedIptEntries, _ := json.Marshal(expectedIptEntries)
		t.Errorf("iptEntries: %s", marshalledIptEntries)
		t.Errorf("expectedIptEntries: %s", marshalledExpectedIptEntries)
	}

}
