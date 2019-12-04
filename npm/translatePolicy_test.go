package npm

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Azure/azure-container-networking/npm/iptm"
	"github.com/Azure/azure-container-networking/npm/util"
	"k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestCraftPartialIptEntrySpecFromPort(t *testing.T) {
	portRule := networkingv1.NetworkPolicyPort{}

	iptEntrySpec := craftPartialIptEntrySpecFromPort(portRule, util.IptablesDstPortFlag)
	expectedIptEntrySpec := []string{}

	if !reflect.DeepEqual(iptEntrySpec, expectedIptEntrySpec) {
		t.Errorf("TestCraftPartialIptEntrySpecFromPort failed @ empty iptEntrySpec comparison")
		t.Errorf("iptEntrySpec:\n%v", iptEntrySpec)
		t.Errorf("expectedIptEntrySpec:\n%v", expectedIptEntrySpec)
	}

	tcp := v1.ProtocolTCP
	portRule = networkingv1.NetworkPolicyPort{
		Protocol: &tcp,
	}

	iptEntrySpec = craftPartialIptEntrySpecFromPort(portRule, util.IptablesDstPortFlag)
	expectedIptEntrySpec = []string{
		util.IptablesProtFlag,
		"TCP",
	}

	if !reflect.DeepEqual(iptEntrySpec, expectedIptEntrySpec) {
		t.Errorf("TestCraftPartialIptEntrySpecFromPort failed @ tcp iptEntrySpec comparison")
		t.Errorf("iptEntrySpec:\n%v", iptEntrySpec)
		t.Errorf("expectedIptEntrySpec:\n%v", expectedIptEntrySpec)
	}

	port8000 := intstr.FromInt(8000)
	portRule = networkingv1.NetworkPolicyPort{
		Port: &port8000,
	}

	iptEntrySpec = craftPartialIptEntrySpecFromPort(portRule, util.IptablesDstPortFlag)
	expectedIptEntrySpec = []string{
		util.IptablesDstPortFlag,
		"8000",
	}

	if !reflect.DeepEqual(iptEntrySpec, expectedIptEntrySpec) {
		t.Errorf("TestCraftPartialIptEntrySpecFromPort failed @ port 8000 iptEntrySpec comparison")
		t.Errorf("iptEntrySpec:\n%v", iptEntrySpec)
		t.Errorf("expectedIptEntrySpec:\n%v", expectedIptEntrySpec)
	}

	portRule = networkingv1.NetworkPolicyPort{
		Protocol: &tcp,
		Port:     &port8000,
	}

	iptEntrySpec = craftPartialIptEntrySpecFromPort(portRule, util.IptablesDstPortFlag)
	expectedIptEntrySpec = []string{
		util.IptablesProtFlag,
		"TCP",
		util.IptablesDstPortFlag,
		"8000",
	}

	if !reflect.DeepEqual(iptEntrySpec, expectedIptEntrySpec) {
		t.Errorf("TestCraftPartialIptEntrySpecFromPort failed @ tcp port 8000 iptEntrySpec comparison")
		t.Errorf("iptEntrySpec:\n%v", iptEntrySpec)
		t.Errorf("expectedIptEntrySpec:\n%v", expectedIptEntrySpec)
	}
}

func TestCraftPartialIptablesCommentFromPort(t *testing.T) {
	portRule := networkingv1.NetworkPolicyPort{}

	comment := craftPartialIptablesCommentFromPort(portRule, util.IptablesDstPortFlag)
	expectedComment := ""

	if !reflect.DeepEqual(comment, expectedComment) {
		t.Errorf("TestCraftPartialIptablesCommentFromPort failed @ empty comment comparison")
		t.Errorf("comment:\n%v", comment)
		t.Errorf("expectedComment:\n%v", expectedComment)
	}

	tcp := v1.ProtocolTCP
	portRule = networkingv1.NetworkPolicyPort{
		Protocol: &tcp,
	}

	comment = craftPartialIptablesCommentFromPort(portRule, util.IptablesDstPortFlag)
	expectedComment = "TCP-OF-"

	if !reflect.DeepEqual(comment, expectedComment) {
		t.Errorf("TestCraftPartialIptablesCommentFromPort failed @ tcp comment comparison")
		t.Errorf("comment:\n%v", comment)
		t.Errorf("expectedComment:\n%v", expectedComment)
	}

	port8000 := intstr.FromInt(8000)
	portRule = networkingv1.NetworkPolicyPort{
		Port: &port8000,
	}

	comment = craftPartialIptablesCommentFromPort(portRule, util.IptablesDstPortFlag)
	expectedComment = "PORT-8000-OF-"

	if !reflect.DeepEqual(comment, expectedComment) {
		t.Errorf("TestCraftPartialIptablesCommentFromPort failed @ port 8000 comment comparison")
		t.Errorf("comment:\n%v", comment)
		t.Errorf("expectedIptEntrySpec:\n%v", expectedComment)
	}

	portRule = networkingv1.NetworkPolicyPort{
		Protocol: &tcp,
		Port:     &port8000,
	}

	comment = craftPartialIptablesCommentFromPort(portRule, util.IptablesDstPortFlag)
	expectedComment = "TCP-PORT-8000-OF-"

	if !reflect.DeepEqual(comment, expectedComment) {
		t.Errorf("TestCraftPartialIptablesCommentFromPort failed @ tcp port 8000 comment comparison")
		t.Errorf("comment:\n%v", comment)
		t.Errorf("expectedIptEntrySpec:\n%v", expectedComment)
	}
}

func TestCraftPartialIptEntrySpecFromOpAndLabel(t *testing.T) {
	srcOp, srcLabel := "", "src"
	iptEntrySpec := craftPartialIptEntrySpecFromOpAndLabel(srcOp, srcLabel, util.IptablesSrcFlag, false)
	expectedIptEntrySpec := []string{
		util.IptablesModuleFlag,
		util.IptablesSetModuleFlag,
		util.IptablesMatchSetFlag,
		util.GetHashedName(srcLabel),
		util.IptablesSrcFlag,
	}

	if !reflect.DeepEqual(iptEntrySpec, expectedIptEntrySpec) {
		t.Errorf("TestCraftIptEntrySpecFromOpAndLabel failed @ src iptEntrySpec comparison")
		t.Errorf("iptEntrySpec:\n%v", iptEntrySpec)
		t.Errorf("expectedIptEntrySpec:\n%v", expectedIptEntrySpec)
	}

	dstOp, dstLabel := "!", "dst"
	iptEntrySpec = craftPartialIptEntrySpecFromOpAndLabel(dstOp, dstLabel, util.IptablesDstFlag, false)
	expectedIptEntrySpec = []string{
		util.IptablesModuleFlag,
		util.IptablesSetModuleFlag,
		util.IptablesNotFlag,
		util.IptablesMatchSetFlag,
		util.GetHashedName(dstLabel),
		util.IptablesDstFlag,
	}

	if !reflect.DeepEqual(iptEntrySpec, expectedIptEntrySpec) {
		t.Errorf("TestCraftIptEntrySpecFromOpAndLabel failed @ dst iptEntrySpec comparison")
		t.Errorf("iptEntrySpec:\n%v", iptEntrySpec)
		t.Errorf("expectedIptEntrySpec:\n%v", expectedIptEntrySpec)
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

	srcIptEntry := craftPartialIptEntrySpecFromOpsAndLabels("testnamespace", srcOps, srcLabels, util.IptablesSrcFlag, false)
	dstIptEntry := craftPartialIptEntrySpecFromOpsAndLabels("testnamespace", dstOps, dstLabels, util.IptablesDstFlag, false)
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

	iptEntrySpec := craftPartialIptEntrySpecFromSelector("testnamespace", srcSelector, util.IptablesSrcFlag, false)
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
	comment := craftPartialIptablesCommentFromSelector("testnamespace", selector, false)
	expectedComment := "none"
	if comment != expectedComment {
		t.Errorf("TestCraftPartialIptablesCommentFromSelector failed @ nil selector comparison")
		t.Errorf("comment:\n%v", comment)
		t.Errorf("expectedComment:\n%v", expectedComment)
	}

	selector = &metav1.LabelSelector{}
	comment = craftPartialIptablesCommentFromSelector("testnamespace", selector, false)
	expectedComment = "ns-testnamespace"
	if comment != expectedComment {
		t.Errorf("TestCraftPartialIptablesCommentFromSelector failed @ empty podSelector comparison")
		t.Errorf("comment:\n%v", comment)
		t.Errorf("expectedComment:\n%v", expectedComment)
	}

	comment = craftPartialIptablesCommentFromSelector("testnamespace", selector, true)
	expectedComment = util.KubeAllNamespacesFlag
	if comment != expectedComment {
		t.Errorf("TestCraftPartialIptablesCommentFromSelector failed @ empty namespaceSelector comparison")
		t.Errorf("comment:\n%v", comment)
		t.Errorf("expectedComment:\n%v", expectedComment)
	}

	selector = &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"k0": "v0",
		},
		MatchExpressions: []metav1.LabelSelectorRequirement{
			metav1.LabelSelectorRequirement{
				Key:      "k1",
				Operator: metav1.LabelSelectorOpIn,
				Values: []string{
					"v10",
					"v11",
				},
			},
			metav1.LabelSelectorRequirement{
				Key:      "k2",
				Operator: metav1.LabelSelectorOpDoesNotExist,
				Values:   []string{},
			},
		},
	}
	comment = craftPartialIptablesCommentFromSelector("testnamespace", selector, false)
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
				Key:      "k1",
				Operator: metav1.LabelSelectorOpIn,
				Values: []string{
					"v10",
					"v11",
				},
			},
			metav1.LabelSelectorRequirement{
				Key:      "k2",
				Operator: metav1.LabelSelectorOpDoesNotExist,
				Values:   []string{},
			},
		},
	}
	comment = craftPartialIptablesCommentFromSelector("testnamespace", nsSelector, true)
	expectedComment = "ns-k0:v0-AND-ns-k1:v10-AND-ns-k1:v11-AND-ns-!k2"
	if comment != expectedComment {
		t.Errorf("TestCraftPartialIptablesCommentFromSelector failed @ namespace selector comparison")
		t.Errorf("comment:\n%v", comment)
		t.Errorf("expectedComment:\n%v", expectedComment)
	}

}

func TestGetDefaultDropEntries(t *testing.T) {
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

	iptIngressEntries := getDefaultDropEntries(ns, targetSelector, true, false)

	expectedIptIngressEntries := []*iptm.IptEntry{
		&iptm.IptEntry{
			Chain: util.IptablesAzureTargetSetsChain,
			Specs: []string{
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
				util.IptablesDrop,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"DROP-ALL-TO-context:dev-AND-!testNotIn:frontend",
			},
		},
	}

	if !reflect.DeepEqual(iptIngressEntries, expectedIptIngressEntries) {
		t.Errorf("TestGetDefaultDropEntries failed @ iptEntries comparison")
		marshalledIptEntries, _ := json.Marshal(iptIngressEntries)
		marshalledExpectedIptEntries, _ := json.Marshal(expectedIptIngressEntries)
		t.Errorf("iptEntries: %s", marshalledIptEntries)
		t.Errorf("expectedIptEntries: %s", marshalledExpectedIptEntries)
	}

	iptEgressEntries := getDefaultDropEntries(ns, targetSelector, false, true)

	expectedIptEgressEntries := []*iptm.IptEntry{
		&iptm.IptEntry{
			Chain: util.IptablesAzureTargetSetsChain,
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
				util.IptablesJumpFlag,
				util.IptablesDrop,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"DROP-ALL-FROM-context:dev-AND-!testNotIn:frontend",
			},
		},
	}

	if !reflect.DeepEqual(iptEgressEntries, expectedIptEgressEntries) {
		t.Errorf("TestGetDefaultDropEntries failed @ iptEntries comparison")
		marshalledIptEntries, _ := json.Marshal(iptEgressEntries)
		marshalledExpectedIptEntries, _ := json.Marshal(expectedIptEgressEntries)
		t.Errorf("iptEntries: %s", marshalledIptEntries)
		t.Errorf("expectedIptEntries: %s", marshalledExpectedIptEntries)
	}

	iptIngressEgressEntries := getDefaultDropEntries(ns, targetSelector, true, true)

	expectedIptIngressEgressEntries := []*iptm.IptEntry{
		&iptm.IptEntry{
			Chain: util.IptablesAzureTargetSetsChain,
			Specs: []string{
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
				util.IptablesDrop,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"DROP-ALL-TO-context:dev-AND-!testNotIn:frontend",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureTargetSetsChain,
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
				util.IptablesJumpFlag,
				util.IptablesDrop,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"DROP-ALL-FROM-context:dev-AND-!testNotIn:frontend",
			},
		},
	}

	if !reflect.DeepEqual(iptIngressEgressEntries, expectedIptIngressEgressEntries) {
		t.Errorf("TestGetDefaultDropEntries failed @ iptEntries comparison")
		marshalledIptEntries, _ := json.Marshal(iptIngressEgressEntries)
		marshalledExpectedIptEntries, _ := json.Marshal(expectedIptIngressEgressEntries)
		t.Errorf("iptEntries: %s", marshalledIptEntries)
		t.Errorf("expectedIptEntries: %s", marshalledExpectedIptEntries)
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
	sets, lists, iptEntries, _ := translateIngress(ns, targetSelector, rules)
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
				"ALLOW-ALL-TO-TCP-PORT-6783-OF-context:dev-AND-!testNotIn:frontend-TO-JUMP-TO-" +
					util.IptablesAzureIngressFromChain,
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressFromChain,
			Specs: []string{
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
				util.IptablesDrop,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"DROP-ALL-TO-context:dev-AND-!testNotIn:frontend",
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
	sets, lists, iptEntries, _ := translateEgress(ns, targetSelector, rules)
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
				"ALLOW-ALL-FROM-TCP-PORT-6783-OF-context:dev-AND-!testNotIn:frontend-TO-JUMP-TO-" +
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
				util.IptablesJumpFlag,
				util.IptablesDrop,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"DROP-ALL-FROM-context:dev-AND-!testNotIn:frontend",
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
			Name:      "deny-all-policy",
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

	expectedIptEntries := []*iptm.IptEntry{}
	expectedIptEntries = append(expectedIptEntries, getDefaultDropEntries("testnamespace", targetSelector, true, false)...)
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
	allowBackendToFrontendPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ALLOW-app:backend-TO-app:frontend-policy",
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

	sets, lists, iptEntries = translatePolicy(allowBackendToFrontendPolicy)

	expectedSets = []string{
		"app:backend",
		"app:frontend",
	}
	if !reflect.DeepEqual(sets, expectedSets) {
		t.Errorf("translatedPolicy failed @ ALLOW-app:backend-TO-app:frontend-policy sets comparison")
		t.Errorf("sets: %v", sets)
		t.Errorf("expectedSets: %v", expectedSets)
	}

	expectedLists = []string{}
	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedPolicy failed @ ALLOW-app:backend-TO-app:frontend-policy lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	expectedIptEntries = []*iptm.IptEntry{}

	nonKubeSystemEntries := []*iptm.IptEntry{
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
				util.GetHashedName("app:backend"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesDrop,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"DROP-ALL-TO-app:backend",
			},
		},
	}
	expectedIptEntries = append(expectedIptEntries, nonKubeSystemEntries...)
	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("translatedPolicy failed @ ALLOW-app:frontend-TO-app:backend-policy policy comparison")
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
	allowToFrontendPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ALLOW-all-TO-app:frontend-FROM-all-namespaces-policy",
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

	sets, lists, iptEntries = translatePolicy(allowToFrontendPolicy)

	expectedSets = []string{
		"app:frontend",
	}
	if !reflect.DeepEqual(sets, expectedSets) {
		t.Errorf("translatedPolicy failed @ ALLOW-all-TO-app:frontend-FROM-all-namespaces-policy sets comparison")
		t.Errorf("sets: %v", sets)
		t.Errorf("expectedSets: %v", expectedSets)
	}

	expectedLists = []string{
		util.KubeAllNamespacesFlag,
	}
	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedPolicy failed @ ALLOW-all-TO-app:frontend-FROM-all-namespaces-policy lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	expectedIptEntries = []*iptm.IptEntry{}

	nonKubeSystemEntries = []*iptm.IptEntry{
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressPortChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName(util.KubeAllNamespacesFlag),
				util.IptablesSrcFlag,
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
				"ALLOW-ALL-TO-app:frontend-FROM-all-namespaces",
			},
		},
	}
	expectedIptEntries = append(expectedIptEntries, nonKubeSystemEntries...)
	expectedIptEntries = append(expectedIptEntries, getDefaultDropEntries("testnamespace", targetSelector, false, false)...)
	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("translatedPolicy failed @ ALLOW-all-TO-app:frontend-FROM-all-namespaces-policy policy comparison")
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
	denyAllToFrontendPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ALLOW-none-TO-app:frontend-policy",
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

	sets, lists, iptEntries = translatePolicy(denyAllToFrontendPolicy)

	expectedSets = []string{
		"app:frontend",
	}
	if !reflect.DeepEqual(sets, expectedSets) {
		t.Errorf("translatedPolicy failed @ ALLOW-none-TO-app:frontend-policy sets comparison")
		t.Errorf("sets: %v", sets)
		t.Errorf("expectedSets: %v", expectedSets)
	}

	expectedLists = []string{}
	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedPolicy failed @ ALLOW-none-TO-app:frontend-policy lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	expectedIptEntries = []*iptm.IptEntry{}
	expectedIptEntries = append(expectedIptEntries, getDefaultDropEntries("testnamespace", targetSelector, true, false)...)
	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("translatedPolicy failed @ ALLOW-none-TO-app:frontend-policy policy comparison")
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
	allowNsTestNamespaceToFrontendPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ALLOW-ns-testnamespace-TO-app:frontend-policy",
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
							PodSelector: &metav1.LabelSelector{},
						},
					},
				},
			},
		},
	}

	sets, lists, iptEntries = translatePolicy(allowNsTestNamespaceToFrontendPolicy)

	expectedSets = []string{
		"app:frontend",
		"ns-testnamespace",
	}
	if !reflect.DeepEqual(sets, expectedSets) {
		t.Errorf("translatedPolicy failed @ ALLOW-ns-testnamespace-TO-app:frontend-policy sets comparison")
		t.Errorf("sets: %v", sets)
		t.Errorf("expectedSets: %v", expectedSets)
	}

	expectedLists = []string{}
	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedPolicy failed @ ALLOW-ns-testnamespace-TO-app:frontend-policy lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	expectedIptEntries = []*iptm.IptEntry{}
	nonKubeSystemEntries = []*iptm.IptEntry{
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressFromChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-testnamespace"),
				util.IptablesSrcFlag,
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
				"ALLOW-ns-testnamespace-TO-app:frontend",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressPortChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:frontend"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAzureIngressFromChain,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ALL-TO-app:frontend-TO-JUMP-TO-" +
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
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesDrop,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"DROP-ALL-TO-app:frontend",
			},
		},
	}
	expectedIptEntries = append(expectedIptEntries, nonKubeSystemEntries...)
	expectedIptEntries = append(expectedIptEntries, getDefaultDropEntries("testnamespace", targetSelector, false, false)...)
	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("translatedPolicy failed @ ALLOW-ns-testnamespace-TO-app:frontend-policy policy comparison")
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
	allowAllNsToFrontendPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ALLOW-all-namespaces-TO-app:frontend-policy",
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
							NamespaceSelector: &metav1.LabelSelector{},
						},
					},
				},
			},
		},
	}

	sets, lists, iptEntries = translatePolicy(allowAllNsToFrontendPolicy)
	expectedSets = []string{
		"app:frontend",
	}
	if !reflect.DeepEqual(sets, expectedSets) {
		t.Errorf("translatedPolicy failed @ ALLOW-all-namespaces-TO-app:frontend-policy sets comparison")
		t.Errorf("sets: %v", sets)
		t.Errorf("expectedSets: %v", expectedSets)
	}

	expectedLists = []string{
		util.KubeAllNamespacesFlag,
	}
	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedPolicy failed @ ALLOW-all-namespaces-TO-app:frontend-policy lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	expectedIptEntries = []*iptm.IptEntry{}
	nonKubeSystemEntries = []*iptm.IptEntry{
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressFromChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName(util.KubeAllNamespacesFlag),
				util.IptablesSrcFlag,
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
				"ALLOW-all-namespaces-TO-app:frontend",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressPortChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:frontend"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAzureIngressFromChain,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ALL-TO-app:frontend-TO-JUMP-TO-" +
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
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesDrop,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"DROP-ALL-TO-app:frontend",
			},
		},
	}
	expectedIptEntries = append(expectedIptEntries, nonKubeSystemEntries...)
	expectedIptEntries = append(expectedIptEntries, getDefaultDropEntries("testnamespace", targetSelector, false, false)...)
	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("translatedPolicy failed @ ALLOW-all-namespaces-TO-app:frontend-policy policy comparison")
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
	allowNsDevToFrontendPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ALLOW-ns-namespace:dev-AND-!ns-namespace:test0-AND-!ns-namespace:test1-TO-app:frontend-policy",
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
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"namespace": "dev",
								},
								MatchExpressions: []metav1.LabelSelectorRequirement{
									metav1.LabelSelectorRequirement{
										Key:      "namespace",
										Operator: metav1.LabelSelectorOpNotIn,
										Values: []string{
											"test0",
											"test1",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	sets, lists, iptEntries = translatePolicy(allowNsDevToFrontendPolicy)

	expectedSets = []string{
		"app:frontend",
	}
	if !reflect.DeepEqual(sets, expectedSets) {
		t.Errorf("translatedPolicy failed @ ALLOW-ns-namespace:dev-AND-!ns-namespace:test0-AND-!ns-namespace:test1-TO-app:frontend-policy sets comparison")
		t.Errorf("sets: %v", sets)
		t.Errorf("expectedSets: %v", expectedSets)
	}

	expectedLists = []string{
		"ns-namespace:dev",
		"ns-namespace:test0",
		"ns-namespace:test1",
	}
	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedPolicy failed @ ALLOW-ns-namespace:dev-AND-!ns-namespace:test0-AND-!ns-namespace:test1-TO-app:frontend-policy lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	expectedIptEntries = []*iptm.IptEntry{}
	nonKubeSystemEntries = []*iptm.IptEntry{
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressFromChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-namespace:dev"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesNotFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-namespace:test0"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesNotFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-namespace:test1"),
				util.IptablesSrcFlag,
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
				"ALLOW-ns-namespace:dev-AND-ns-!namespace:test0-AND-ns-!namespace:test1-TO-app:frontend",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressPortChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:frontend"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAzureIngressFromChain,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ALL-TO-app:frontend-TO-JUMP-TO-" +
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
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesDrop,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"DROP-ALL-TO-app:frontend",
			},
		},
	}

	expectedIptEntries = append(expectedIptEntries, nonKubeSystemEntries...)
	expectedIptEntries = append(expectedIptEntries, getDefaultDropEntries("testnamespace", targetSelector, false, false)...)
	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("translatedPolicy failed @ ALLOW-ns-namespace:dev-AND-!ns-namespace:test0-AND-!ns-namespace:test1-TO-app:frontend-policy policy comparison")
		marshalledIptEntries, _ := json.Marshal(iptEntries)
		marshalledExpectedIptEntries, _ := json.Marshal(expectedIptEntries)
		t.Errorf("iptEntries: %s", marshalledIptEntries)
		t.Errorf("expectedIptEntries: %s", marshalledExpectedIptEntries)
	}

	targetSelector = metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			metav1.LabelSelectorRequirement{
				Key:      "k0",
				Operator: metav1.LabelSelectorOpDoesNotExist,
				Values:   []string{},
			},
			metav1.LabelSelectorRequirement{
				Key:      "k1",
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{"v0", "v1"},
			},
		},
		MatchLabels: map[string]string{
			"app": "frontend",
		},
	}
	allowAllToFrontendPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "AllOW-ALL-TO-k0-AND-k1:v0-AND-k1:v1-AND-app:frontend-policy",
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
							NamespaceSelector: &metav1.LabelSelector{},
						},
					},
				},
			},
		},
	}

	sets, lists, iptEntries = translatePolicy(allowAllToFrontendPolicy)

	expectedSets = []string{
		"app:frontend",
		"k0",
		"k1:v0",
		"k1:v1",
	}
	if !reflect.DeepEqual(sets, expectedSets) {
		t.Errorf("translatedPolicy failed @ AllOW-ALL-TO-k0-AND-k1:v0-AND-k1:v1-AND-app:frontend-policy sets comparison")
		t.Errorf("sets: %v", sets)
		t.Errorf("expectedSets: %v", expectedSets)
	}

	expectedLists = []string{util.KubeAllNamespacesFlag}
	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedPolicy failed @ AllOW-ALL-TO-k0-AND-k1:v0-AND-k1:v1-AND-app:frontend-policy lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	expectedIptEntries = []*iptm.IptEntry{}
	nonKubeSystemEntries = []*iptm.IptEntry{
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressFromChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName(util.KubeAllNamespacesFlag),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:frontend"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesNotFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("k0"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("k1:v0"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("k1:v1"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAccept,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-all-namespaces-TO-app:frontend-AND-!k0-AND-k1:v0-AND-k1:v1",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressPortChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:frontend"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesNotFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("k0"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("k1:v0"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("k1:v1"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAzureIngressFromChain,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ALL-TO-app:frontend-AND-!k0-AND-k1:v0-AND-k1:v1-TO-JUMP-TO-" +
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
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesNotFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("k0"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("k1:v0"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("k1:v1"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesDrop,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"DROP-ALL-TO-app:frontend-AND-!k0-AND-k1:v0-AND-k1:v1",
			},
		},
	}

	expectedIptEntries = append(expectedIptEntries, nonKubeSystemEntries...)
	expectedIptEntries = append(expectedIptEntries, getDefaultDropEntries("testnamespace", targetSelector, false, false)...)
	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("translatedPolicy failed @ AllOW-all-TO-k0-AND-k1:v0-AND-k1:v1-AND-app:frontend-policy policy comparison")
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
	allowNsDevAndBackendToFrontendPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ALLOW-ns-ns:dev-AND-app:backend-TO-app:frontend",
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
									"app": "backend",
								},
							},
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"ns": "dev",
								},
							},
						},
					},
				},
			},
		},
	}

	util.IsNewNwPolicyVerFlag = true
	sets, lists, iptEntries = translatePolicy(allowNsDevAndBackendToFrontendPolicy)

	expectedSets = []string{
		"app:frontend",
		"app:backend",
	}
	if !reflect.DeepEqual(sets, expectedSets) {
		t.Errorf("translatedPolicy failed @ ALLOW-ns-ns:dev-AND-app:backend-TO-app:frontend sets comparison")
		t.Errorf("sets: %v", sets)
		t.Errorf("expectedSets: %v", expectedSets)
	}

	expectedLists = []string{
		"ns-ns:dev",
	}
	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedPolicy failed @ ALLOW-ns-ns:dev-AND-app:backend-TO-app:frontend lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	expectedIptEntries = []*iptm.IptEntry{}
	nonKubeSystemEntries = []*iptm.IptEntry{
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
				util.GetHashedName("app:backend"),
				util.IptablesSrcFlag,
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
				"ALLOW-ns-ns:dev-AND-app:backend-TO-app:frontend",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressPortChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:frontend"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAzureIngressFromChain,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ALL-TO-app:frontend-TO-JUMP-TO-" +
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
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesDrop,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"DROP-ALL-TO-app:frontend",
			},
		},
	}

	expectedIptEntries = append(expectedIptEntries, nonKubeSystemEntries...)
	expectedIptEntries = append(expectedIptEntries, getDefaultDropEntries("testnamespace", targetSelector, false, false)...)
	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("translatedPolicy failed @ ALLOW-ns-ns:dev-AND-app:backend-TO-app:frontend policy comparison")
		marshalledIptEntries, _ := json.Marshal(iptEntries)
		marshalledExpectedIptEntries, _ := json.Marshal(expectedIptEntries)
		t.Errorf("iptEntries: %s", marshalledIptEntries)
		t.Errorf("expectedIptEntries: %s", marshalledExpectedIptEntries)
	}

	targetSelector = metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app": "backdoor",
		},
	}
	allowInternalAndExternalPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ALLOW-ALL-TO-app:backdoor-policy",
			Namespace: "dangerous",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: targetSelector,
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				networkingv1.NetworkPolicyIngressRule{
					From: []networkingv1.NetworkPolicyPeer{},
				},
			},
		},
	}

	sets, lists, iptEntries = translatePolicy(allowInternalAndExternalPolicy)

	expectedSets = []string{
		"app:backdoor",
	}
	if !reflect.DeepEqual(sets, expectedSets) {
		t.Errorf("translatedPolicy failed @ ALLOW-ALL-TO-app:backdoor-policy sets comparison")
		t.Errorf("sets: %v", sets)
		t.Errorf("expectedSets: %v", expectedSets)
	}

	expectedLists = []string{}
	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedPolicy failed @ ALLOW-ALL-TO-app:backdoor-policy lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	expectedIptEntries = []*iptm.IptEntry{}
	nonKubeSystemEntries = []*iptm.IptEntry{
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressPortChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:backdoor"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAzureIngressFromChain,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ALL-TO-app:backdoor-TO-JUMP-TO-" +
					util.IptablesAzureIngressFromChain,
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressFromChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:backdoor"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAccept,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ALL-TO-app:backdoor",
			},
		},
	}

	expectedIptEntries = append(expectedIptEntries, nonKubeSystemEntries...)
	expectedIptEntries = append(expectedIptEntries, getDefaultDropEntries("dangerous", targetSelector, false, false)...)
	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("translatedPolicy failed @ ALLOW-ALL-TO-app:backdoor-policy policy comparison")
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

	port8000 := intstr.FromInt(8000)
	allowBackendToFrontendPort8000Policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ALLOW-app:backend-TO-app:frontend-port-8000-policy",
			Namespace: "testnamespace",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: targetSelector,
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				networkingv1.NetworkPolicyIngressRule{
					Ports: []networkingv1.NetworkPolicyPort{
						networkingv1.NetworkPolicyPort{
							Port: &port8000,
						},
					},
					From: []networkingv1.NetworkPolicyPeer{
						networkingv1.NetworkPolicyPeer{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "backend",
								},
							},
						},
					},
				},
			},
		},
	}

	sets, lists, iptEntries = translatePolicy(allowBackendToFrontendPort8000Policy)

	expectedSets = []string{
		"app:frontend",
		"app:backend",
	}
	if !reflect.DeepEqual(sets, expectedSets) {
		t.Errorf("translatedPolicy failed @ ALLOW-app:backend-TO-app:frontend-port-8000-policy sets comparison")
		t.Errorf("sets: %v", sets)
		t.Errorf("expectedSets: %v", expectedSets)
	}

	expectedLists = []string{}
	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedPolicy failed @ ALLOW-app:backend-TO-app:frontend-port-8000-policy lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	expectedIptEntries = []*iptm.IptEntry{}
	nonKubeSystemEntries = []*iptm.IptEntry{
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressFromChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:backend"),
				util.IptablesSrcFlag,
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
				"ALLOW-app:backend-TO-app:frontend",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressPortChain,
			Specs: []string{
				util.IptablesDstPortFlag,
				"8000",
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:frontend"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAzureIngressFromChain,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ALL-TO-PORT-8000-OF-app:frontend-TO-JUMP-TO-" +
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
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesDrop,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"DROP-ALL-TO-app:frontend",
			},
		},
	}

	expectedIptEntries = append(expectedIptEntries, nonKubeSystemEntries...)
	expectedIptEntries = append(expectedIptEntries, getDefaultDropEntries("dangerous", targetSelector, false, false)...)
	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("translatedPolicy failed @ ALLOW-ALL-TO-app:backdoor-policy policy comparison")
		marshalledIptEntries, _ := json.Marshal(iptEntries)
		marshalledExpectedIptEntries, _ := json.Marshal(expectedIptEntries)
		t.Errorf("iptEntries: %s", marshalledIptEntries)
		t.Errorf("expectedIptEntries: %s", marshalledExpectedIptEntries)
	}

	targetSelector = metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app":  "k8s",
			"team": "aks",
		},
	}
	allowCniOrCnsToK8sPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ALLOW-program:cni-AND-team:acn-OR-binary:cns-AND-group:container-TO-app:k8s-AND-team:aks-policy",
			Namespace: "acn",
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
									"program": "cni",
									"team":    "acn",
								},
							},
						},
						networkingv1.NetworkPolicyPeer{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"binary": "cns",
									"group":  "container",
								},
							},
						},
					},
				},
			},
		},
	}

	sets, lists, iptEntries = translatePolicy(allowCniOrCnsToK8sPolicy)

	expectedSets = []string{
		"app:k8s",
		"team:aks",
		"program:cni",
		"team:acn",
		"binary:cns",
		"group:container",
	}
	if !reflect.DeepEqual(sets, expectedSets) {
		t.Errorf("translatedPolicy failed @ ALLOW-program:cni-AND-team:acn-OR-binary:cns-AND-group:container-TO-app:k8s-AND-team:aks-policy sets comparison")
		t.Errorf("sets: %v", sets)
		t.Errorf("expectedSets: %v", expectedSets)
	}

	expectedLists = []string{}
	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedPolicy failed @ ALLOW-program:cni-AND-team:acn-OR-binary:cns-AND-group:container-TO-app:k8s-AND-team:aks-policy lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	expectedIptEntries = []*iptm.IptEntry{}
	nonKubeSystemEntries = []*iptm.IptEntry{
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressFromChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("program:cni"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("team:acn"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:k8s"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("team:aks"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAccept,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-program:cni-AND-team:acn-TO-app:k8s-AND-team:aks",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressFromChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("binary:cns"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("group:container"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:k8s"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("team:aks"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAccept,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-binary:cns-AND-group:container-TO-app:k8s-AND-team:aks",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressPortChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:k8s"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("team:aks"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAzureIngressFromChain,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ALL-TO-app:k8s-AND-team:aks-TO-JUMP-TO-" +
					util.IptablesAzureIngressFromChain,
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressFromChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:k8s"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("team:aks"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesDrop,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"DROP-ALL-TO-app:k8s-AND-team:aks",
			},
		},
	}

	expectedIptEntries = append(expectedIptEntries, nonKubeSystemEntries...)
	expectedIptEntries = append(expectedIptEntries, getDefaultDropEntries("acn", targetSelector, false, false)...)
	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("translatedPolicy failed @ ALLOW-program:cni-AND-team:acn-OR-binary:cns-AND-group:container-TO-app:k8s-AND-team:aks-policy policy comparison")
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
	denyAllFromBackendPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ALLOW-none-FROM-app:backend-policy",
			Namespace: "testnamespace",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: targetSelector,
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeEgress,
			},
		},
	}

	sets, lists, iptEntries = translatePolicy(denyAllFromBackendPolicy)

	expectedSets = []string{
		"app:backend",
	}
	if !reflect.DeepEqual(sets, expectedSets) {
		t.Errorf("translatedPolicy failed @ ALLOW-none-FROM-app:backend-policy sets comparison")
		t.Errorf("sets: %v", sets)
		t.Errorf("expectedSets: %v", expectedSets)
	}

	expectedLists = []string{}
	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedPolicy failed @ ALLOW-none-FROM-app:backend-policy lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	expectedIptEntries = []*iptm.IptEntry{}
	expectedIptEntries = append(expectedIptEntries, getDefaultDropEntries("testnamespace", targetSelector, false, true)...)
	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("translatedPolicy failed @ ALLOW-none-FROM-app:backend-policy policy comparison")
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

	//////
	/// This policy tests the case where pods should have unlimited egress traffic
	//////
	allowAllEgress := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ALLOW-all-FROM-app:backend-policy",
			Namespace: "testnamespace",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: targetSelector,
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeEgress,
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{networkingv1.NetworkPolicyEgressRule{}},
		},
	}

	sets, lists, iptEntries = translatePolicy(allowAllEgress)

	expectedSets = []string{
		"app:backend",
	}
	if !reflect.DeepEqual(sets, expectedSets) {
		t.Errorf("translatedPolicy failed @ ALLOW-all-FROM-app:backend-policy sets comparison")
		t.Errorf("sets: %v", sets)
		t.Errorf("expectedSets: %v", expectedSets)
	}

	expectedLists = []string{
		util.KubeAllNamespacesFlag,
	}
	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedPolicy failed @ ALLOW-all-FROM-app:backend-policy lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	expectedIptEntries = []*iptm.IptEntry{}
	nonKubeSystemEntries = []*iptm.IptEntry{
		&iptm.IptEntry{
			Chain: util.IptablesAzureEgressPortChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:backend"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName(util.KubeAllNamespacesFlag),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAccept,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ALL-FROM-app:backend-TO-" +
					util.KubeAllNamespacesFlag,
			},
		},
	}
	expectedIptEntries = append(expectedIptEntries, nonKubeSystemEntries...)
	// has egress, but empty map means allow all
	expectedIptEntries = append(expectedIptEntries, getDefaultDropEntries("testnamespace", targetSelector, false, false)...)
	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("translatedPolicy failed @ ALLOW-all-FROM-app:backend-policy policy comparison")
		marshalledIptEntries, _ := json.Marshal(iptEntries)
		marshalledExpectedIptEntries, _ := json.Marshal(expectedIptEntries)
		t.Errorf("iptEntries: %s", marshalledIptEntries)
		t.Errorf("expectedIptEntries: %s", marshalledExpectedIptEntries)
	}

	targetSelector = metav1.LabelSelector{}
	denyAllFromNsUnsafePolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ALLOW-none-FROM-ns-unsafe-policy",
			Namespace: "unsafe",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: targetSelector,
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeEgress,
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{},
		},
	}

	sets, lists, iptEntries = translatePolicy(denyAllFromNsUnsafePolicy)

	expectedSets = []string{
		"ns-unsafe",
	}
	if !reflect.DeepEqual(sets, expectedSets) {
		t.Errorf("translatedPolicy failed @ ALLOW-none-FROM-ns-unsafe-policy sets comparison")
		t.Errorf("sets: %v", sets)
		t.Errorf("expectedSets: %v", expectedSets)
	}
	expectedLists = []string{}
	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedPolicy failed @ ALLOW-none-FROM-app:backend-policy lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	expectedIptEntries = []*iptm.IptEntry{}
	expectedIptEntries = append(expectedIptEntries, getDefaultDropEntries("unsafe", targetSelector, false, true)...)
	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("translatedPolicy failed @ ALLOW-none-FROM-app:backend-policy policy comparison")
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

	tcp, udp := v1.ProtocolTCP, v1.ProtocolUDP
	port53 := intstr.FromInt(53)
	allowFrontendToTCPPort80UDPPOrt443Policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ALLOW-ALL-FROM-app:frontend-TCP-PORT-53-OR-UDP-PORT-53-policy",
			Namespace: "testnamespace",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: targetSelector,
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeEgress,
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				networkingv1.NetworkPolicyEgressRule{
					Ports: []networkingv1.NetworkPolicyPort{
						networkingv1.NetworkPolicyPort{
							Protocol: &tcp,
							Port:     &port53,
						},
						networkingv1.NetworkPolicyPort{
							Protocol: &udp,
							Port:     &port53,
						},
					},
				},
				networkingv1.NetworkPolicyEgressRule{
					To: []networkingv1.NetworkPolicyPeer{
						networkingv1.NetworkPolicyPeer{
							NamespaceSelector: &metav1.LabelSelector{},
						},
					},
				},
			},
		},
	}

	sets, lists, iptEntries = translatePolicy(allowFrontendToTCPPort80UDPPOrt443Policy)

	expectedSets = []string{
		"app:frontend",
	}
	if !reflect.DeepEqual(sets, expectedSets) {
		t.Errorf("translatedPolicy failed @ ALLOW-ALL-FROM-app:frontend-TCP-PORT-53-OR-UDP-PORT-53-policy sets comparison")
		t.Errorf("sets: %v", sets)
		t.Errorf("expectedSets: %v", expectedSets)
	}

	expectedLists = []string{
		util.KubeAllNamespacesFlag,
	}
	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedPolicy failed @ ALLOW-ALL-FROM-app:frontend-TCP-PORT-53-OR-UDP-PORT-53-policy lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	expectedIptEntries = []*iptm.IptEntry{}
	nonKubeSystemEntries = []*iptm.IptEntry{
		&iptm.IptEntry{
			Chain: util.IptablesAzureEgressPortChain,
			Specs: []string{
				util.IptablesProtFlag,
				"TCP",
				util.IptablesDstPortFlag,
				"53",
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:frontend"),
				util.IptablesSrcFlag,
				util.IptablesJumpFlag,
				util.IptablesAccept,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ALL-FROM-TCP-PORT-53-OF-app:frontend",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureEgressPortChain,
			Specs: []string{
				util.IptablesProtFlag,
				"UDP",
				util.IptablesDstPortFlag,
				"53",
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:frontend"),
				util.IptablesSrcFlag,
				util.IptablesJumpFlag,
				util.IptablesAccept,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ALL-FROM-UDP-PORT-53-OF-app:frontend",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureEgressToChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:frontend"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName(util.KubeAllNamespacesFlag),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAccept,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-app:frontend-TO-" +
					util.KubeAllNamespacesFlag,
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureEgressPortChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:frontend"),
				util.IptablesSrcFlag,
				util.IptablesJumpFlag,
				util.IptablesAzureEgressToChain,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ALL-FROM-app:frontend-TO-JUMP-TO-" +
					util.IptablesAzureEgressToChain,
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureEgressToChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:frontend"),
				util.IptablesSrcFlag,
				util.IptablesJumpFlag,
				util.IptablesDrop,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"DROP-ALL-FROM-app:frontend",
			},
		},
	}
	expectedIptEntries = append(expectedIptEntries, nonKubeSystemEntries...)
	expectedIptEntries = append(expectedIptEntries, getDefaultDropEntries("testnamespace", targetSelector, false, false)...)
	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("translatedPolicy failed @ ALLOW-ALL-FROM-app:frontend-TCP-PORT-53-OR-UDP-PORT-53-policy policy comparison")
		marshalledIptEntries, _ := json.Marshal(iptEntries)
		marshalledExpectedIptEntries, _ := json.Marshal(expectedIptEntries)
		t.Errorf("iptEntries: %s", marshalledIptEntries)
		t.Errorf("expectedIptEntries: %s", marshalledExpectedIptEntries)
	}

	targetSelector = metav1.LabelSelector{
		MatchLabels: map[string]string{
			"role": "db",
		},
	}

	tcp = v1.ProtocolTCP
	port6379, port5978 := intstr.FromInt(6379), intstr.FromInt(5978)
	k8sExamplePolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "k8s-example-policy",
			Namespace: "default",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: targetSelector,
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				networkingv1.NetworkPolicyIngressRule{
					From: []networkingv1.NetworkPolicyPeer{
						networkingv1.NetworkPolicyPeer{
							IPBlock: &networkingv1.IPBlock{
								CIDR: "172.17.0.0/16",
								Except: []string{
									"172.17.1.0/24",
								},
							},
						},
						networkingv1.NetworkPolicyPeer{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"project": "myproject",
								},
							},
						},
						networkingv1.NetworkPolicyPeer{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"role": "frontend",
								},
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						networkingv1.NetworkPolicyPort{
							Protocol: &tcp,
							Port:     &port6379,
						},
					},
				},
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				networkingv1.NetworkPolicyEgressRule{
					To: []networkingv1.NetworkPolicyPeer{
						networkingv1.NetworkPolicyPeer{
							IPBlock: &networkingv1.IPBlock{
								CIDR: "10.0.0.0/24",
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						networkingv1.NetworkPolicyPort{
							Protocol: &tcp,
							Port:     &port5978,
						},
					},
				},
			},
		},
	}

	sets, lists, iptEntries = translatePolicy(k8sExamplePolicy)

	expectedSets = []string{
		"role:db",
		"role:frontend",
	}
	if !reflect.DeepEqual(sets, expectedSets) {
		t.Errorf("translatedPolicy failed @ k8s-example-policy sets comparison")
		t.Errorf("sets: %v", sets)
		t.Errorf("expectedSets: %v", expectedSets)
	}

	expectedLists = []string{
		"ns-project:myproject",
	}
	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedPolicy failed @ k8s-example-policy lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	expectedIptEntries = []*iptm.IptEntry{}
	nonKubeSystemEntries = []*iptm.IptEntry{
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressFromChain,
			Specs: []string{
				util.IptablesSFlag,
				"172.17.1.0/24",
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("role:db"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesDrop,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"DROP-172.17.1.0/24-TO-role:db",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressFromChain,
			Specs: []string{
				util.IptablesSFlag,
				"172.17.0.0/16",
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("role:db"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAccept,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-172.17.0.0/16-TO-role:db",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressFromChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-project:myproject"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("role:db"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAccept,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ns-project:myproject-TO-role:db",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressFromChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("role:frontend"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("role:db"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAccept,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-role:frontend-TO-role:db",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressPortChain,
			Specs: []string{
				util.IptablesProtFlag,
				"TCP",
				util.IptablesDstPortFlag,
				"6379",
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("role:db"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAzureIngressFromChain,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ALL-TO-TCP-PORT-6379-OF-role:db-TO-JUMP-TO-" +
					util.IptablesAzureIngressFromChain,
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressFromChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("role:db"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesDrop,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"DROP-ALL-TO-role:db",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureEgressToChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("role:db"),
				util.IptablesSrcFlag,
				util.IptablesDFlag,
				"10.0.0.0/24",
				util.IptablesJumpFlag,
				util.IptablesAccept,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-10.0.0.0/24-FROM-role:db",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureEgressPortChain,
			Specs: []string{
				util.IptablesProtFlag,
				"TCP",
				util.IptablesDstPortFlag,
				"5978",
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("role:db"),
				util.IptablesSrcFlag,
				util.IptablesJumpFlag,
				util.IptablesAzureEgressToChain,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ALL-FROM-TCP-PORT-5978-OF-role:db-TO-JUMP-TO-" +
					util.IptablesAzureEgressToChain,
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureEgressToChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("role:db"),
				util.IptablesSrcFlag,
				util.IptablesJumpFlag,
				util.IptablesDrop,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"DROP-ALL-FROM-role:db",
			},
		},
	}
	expectedIptEntries = append(expectedIptEntries, nonKubeSystemEntries...)
	expectedIptEntries = append(expectedIptEntries, getDefaultDropEntries("testnamespace", targetSelector, false, false)...)
	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("translatedPolicy failed @ k8s-example-policy policy comparison")
		marshalledIptEntries, _ := json.Marshal(iptEntries)
		marshalledExpectedIptEntries, _ := json.Marshal(expectedIptEntries)
		t.Errorf("iptEntries: %s", marshalledIptEntries)
		t.Errorf("expectedIptEntries: %s", marshalledExpectedIptEntries)
	}
}

func TestDropPrecedenceOverAllow(t *testing.T) {
	targetSelector := metav1.LabelSelector{}
	targetSelectorA := metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app": "test",
		},
		MatchExpressions: []metav1.LabelSelectorRequirement{
			metav1.LabelSelectorRequirement{
				Key:      "testIn",
				Operator: metav1.LabelSelectorOpIn,
				Values: []string{
					"pod-A",
				},
			},
		},
	}
	denyAllPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-deny",
			Namespace: "default",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: targetSelector,
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{},
		},
	}
	allowToPodPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-A",
			Namespace: "default",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: targetSelectorA,
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				networkingv1.NetworkPolicyIngressRule{
					From: []networkingv1.NetworkPolicyPeer{
						networkingv1.NetworkPolicyPeer{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "test",
								},
								MatchExpressions: []metav1.LabelSelectorRequirement{
									metav1.LabelSelectorRequirement{
										Key:      "testIn",
										Operator: metav1.LabelSelectorOpIn,
										Values: []string{
											"pod-B",
										},
									},
								},
							},
						},
						networkingv1.NetworkPolicyPeer{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "test",
								},
								MatchExpressions: []metav1.LabelSelectorRequirement{
									metav1.LabelSelectorRequirement{
										Key:      "testIn",
										Operator: metav1.LabelSelectorOpIn,
										Values: []string{
											"pod-C",
										},
									},
								},
							},
						},
					},
				},
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				networkingv1.NetworkPolicyEgressRule{
					To: []networkingv1.NetworkPolicyPeer{
						networkingv1.NetworkPolicyPeer{
							NamespaceSelector: &metav1.LabelSelector{},
						},
					},
				},
			},
		},
	}

	sets, lists, iptEntries := translatePolicy(denyAllPolicy)
	expectedSets := []string{
		"ns-default",
	}
	if !reflect.DeepEqual(sets, expectedSets) {
		t.Errorf("translatedPolicy failed @ k8s-example-policy sets comparison")
		t.Errorf("sets: %v", sets)
		t.Errorf("expectedSets: %v", expectedSets)
	}

	expectedLists := []string{}
	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedPolicy failed @ k8s-example-policy lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	sets, lists, finalIptEntries := translatePolicy(allowToPodPolicy)
	expectedSets = []string{
		"app:test",
		"testIn:pod-A",
		"testIn:pod-B",
		"testIn:pod-C",
	}
	if !reflect.DeepEqual(sets, expectedSets) {
		t.Errorf("translatedPolicy failed @ k8s-example-policy sets comparison")
		t.Errorf("sets: %v", sets)
		t.Errorf("expectedSets: %v", expectedSets)
	}

	expectedLists = []string{
		"all-namespaces",
	}
	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedPolicy failed @ k8s-example-policy lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	iptEntries = append(iptEntries, finalIptEntries...)

	nonKubeSystemEntries := []*iptm.IptEntry{
		&iptm.IptEntry{
			Chain: util.IptablesAzureTargetSetsChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-default"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesDrop,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"DROP-ALL-TO-ns-default",
			},
		},
	}
	nonKubeSystemEntries2 := []*iptm.IptEntry{
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressFromChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:test"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("testIn:pod-B"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:test"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("testIn:pod-A"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAccept,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-app:test-AND-testIn:pod-B-TO-app:test-AND-testIn:pod-A",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressFromChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:test"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("testIn:pod-C"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:test"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("testIn:pod-A"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAccept,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-app:test-AND-testIn:pod-C-TO-app:test-AND-testIn:pod-A",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressPortChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:test"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("testIn:pod-A"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAzureIngressFromChain,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ALL-TO-app:test-AND-testIn:pod-A-TO-JUMP-TO-" +
					util.IptablesAzureIngressFromChain,
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressFromChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:test"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("testIn:pod-A"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesDrop,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"DROP-ALL-TO-app:test-AND-testIn:pod-A",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureEgressToChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:test"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("testIn:pod-A"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("all-namespaces"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAccept,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-app:test-AND-testIn:pod-A-TO-all-namespaces",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureEgressPortChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:test"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("testIn:pod-A"),
				util.IptablesSrcFlag,
				util.IptablesJumpFlag,
				util.IptablesAzureEgressToChain,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ALL-FROM-app:test-AND-testIn:pod-A-TO-JUMP-TO-" +
					util.IptablesAzureEgressToChain,
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureEgressToChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:test"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("testIn:pod-A"),
				util.IptablesSrcFlag,
				util.IptablesJumpFlag,
				util.IptablesDrop,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"DROP-ALL-FROM-app:test-AND-testIn:pod-A",
			},
		},
	}
	expectedIptEntries := []*iptm.IptEntry{}
	expectedIptEntries = append(expectedIptEntries, nonKubeSystemEntries...)
	expectedIptEntries = append(expectedIptEntries, nonKubeSystemEntries2...)
	expectedIptEntries = append(expectedIptEntries, getDefaultDropEntries("default", targetSelectorA, false, false)...)
	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("TestAllowPrecedenceOverDeny failed @ k8s-example-policy policy comparison")
		marshalledIptEntries, _ := json.Marshal(iptEntries)
		marshalledExpectedIptEntries, _ := json.Marshal(expectedIptEntries)
		t.Errorf("iptEntries: %s", marshalledIptEntries)
		t.Errorf("expectedIptEntries: %s", marshalledExpectedIptEntries)
	}
}
