package npm

import (
	"encoding/json"
	"io/ioutil"
	"reflect"
	"testing"

	"github.com/Azure/azure-container-networking/npm/iptm"
	"github.com/Azure/azure-container-networking/npm/util"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
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
	expectedComment = "TCP"

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
	expectedComment = "PORT-8000"

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
	expectedComment = "TCP-PORT-8000"

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
		util.GetHashedName("ns-testnamespace"),
		util.IptablesSrcFlag,
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
		util.IptablesMatchSetFlag,
		util.GetHashedName("ns-testnamespace"),
		util.IptablesDstFlag,
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
		util.GetHashedName("ns-testnamespace"),
		util.IptablesSrcFlag,
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
	expectedComment = "k0:v0-AND-k1:v10-AND-k1:v11-AND-!k2-IN-ns-testnamespace"
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
	comment = craftPartialIptablesCommentFromSelector("", nsSelector, true)
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
			Chain: util.IptablesAzureIngressDropsChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-testnamespace"),
				util.IptablesDstFlag,
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
				"DROP-ALL-TO-context:dev-AND-!testNotIn:frontend-IN-ns-testnamespace",
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
			Chain: util.IptablesAzureEgressDropsChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-testnamespace"),
				util.IptablesSrcFlag,
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
				"DROP-ALL-FROM-context:dev-AND-!testNotIn:frontend-IN-ns-testnamespace",
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
			Chain: util.IptablesAzureIngressDropsChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-testnamespace"),
				util.IptablesDstFlag,
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
				"DROP-ALL-TO-context:dev-AND-!testNotIn:frontend-IN-ns-testnamespace",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureEgressDropsChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-testnamespace"),
				util.IptablesSrcFlag,
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
				"DROP-ALL-FROM-context:dev-AND-!testNotIn:frontend-IN-ns-testnamespace",
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
	name := "testnetworkpolicyname"
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
	sets, _, lists, _, iptEntries := translateIngress(ns, name, targetSelector, rules)
	expectedSets := []string{
		"context:dev",
		"testNotIn:frontend",
		"ns-testnamespace",
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
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-testnamespace"),
				util.IptablesDstFlag,
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
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-testnamespace"),
				util.IptablesSrcFlag,
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
				util.IptablesProtFlag,
				string(v1.ProtocolTCP),
				util.IptablesDstPortFlag,
				"6783",
				util.IptablesJumpFlag,
				util.IptablesMark,
				util.IptablesSetMarkFlag,
				util.IptablesAzureIngressMarkHex,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-app:db-AND-testIn:frontend-IN-ns-testnamespace-AND-TCP-PORT-6783-TO-context:dev-AND-!testNotIn:frontend-IN-ns-testnamespace",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressPortChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-testnamespace"),
				util.IptablesDstFlag,
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
				util.IptablesProtFlag,
				string(v1.ProtocolTCP),
				util.IptablesDstPortFlag,
				"6783",
				util.IptablesJumpFlag,
				util.IptablesMark,
				util.IptablesSetMarkFlag,
				util.IptablesAzureIngressMarkHex,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ns-ns:dev-AND-ns-testIn:frontendns-AND-TCP-PORT-6783-TO-context:dev-AND-!testNotIn:frontend-IN-ns-testnamespace",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressPortChain,
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
				util.GetHashedName("ns-testnamespace"),
				util.IptablesDstFlag,
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
				util.IptablesProtFlag,
				string(v1.ProtocolTCP),
				util.IptablesDstPortFlag,
				"6783",
				util.IptablesJumpFlag,
				util.IptablesMark,
				util.IptablesSetMarkFlag,
				util.IptablesAzureIngressMarkHex,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ns-planet:earth-AND-ns-keyExists-AND-region:northpole-AND-!k-AND-TCP-PORT-6783-TO-context:dev-AND-!testNotIn:frontend-IN-ns-testnamespace",
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
	name := "testnetworkpolicyname"

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
	sets, _, lists, _, iptEntries := translateEgress(ns, name, targetSelector, rules)
	expectedSets := []string{
		"context:dev",
		"testNotIn:frontend",
		"ns-testnamespace",
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
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-testnamespace"),
				util.IptablesDstFlag,
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
				util.IptablesProtFlag,
				string(v1.ProtocolTCP),
				util.IptablesDstPortFlag,
				"6783",
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-testnamespace"),
				util.IptablesSrcFlag,
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
				util.IptablesMark,
				util.IptablesSetMarkFlag,
				util.IptablesAzureEgressXMarkHex,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-app:db-AND-testIn:frontend-IN-ns-testnamespace-AND-TCP-PORT-6783-FROM-context:dev-AND-!testNotIn:frontend-IN-ns-testnamespace",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureEgressPortChain,
			Specs: []string{
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
				util.IptablesProtFlag,
				string(v1.ProtocolTCP),
				util.IptablesDstPortFlag,
				"6783",
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-testnamespace"),
				util.IptablesSrcFlag,
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
				util.IptablesMark,
				util.IptablesSetMarkFlag,
				util.IptablesAzureEgressXMarkHex,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ns-ns:dev-AND-ns-testIn:frontendns-AND-TCP-PORT-6783-FROM-context:dev-AND-!testNotIn:frontend-IN-ns-testnamespace",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureEgressPortChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-testnamespace"),
				util.IptablesSrcFlag,
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
				util.IptablesProtFlag,
				string(v1.ProtocolTCP),
				util.IptablesDstPortFlag,
				"6783",
				util.IptablesJumpFlag,
				util.IptablesMark,
				util.IptablesSetMarkFlag,
				util.IptablesAzureEgressXMarkHex,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-context:dev-AND-!testNotIn:frontend-IN-ns-testnamespace-TO-ns-planet:earth-AND-ns-keyExists-AND-region:northpole-AND-!k-AND-TCP-PORT-6783",
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

func readPolicyYaml(policyYaml string) (*networkingv1.NetworkPolicy, error) {
	decode := scheme.Codecs.UniversalDeserializer().Decode
	b, err := ioutil.ReadFile(policyYaml)
	if err != nil {
		return nil, err
	}
	obj, _, err := decode([]byte(b), nil, nil)
	if err != nil {
		return nil, err
	}
	return obj.(*networkingv1.NetworkPolicy), nil
}

func TestDenyAllPolicy(t *testing.T) {
	denyAllPolicy, err := readPolicyYaml("testpolicies/deny-all-policy.yaml")
	if err != nil {
		t.Fatal(err)
	}

	sets, _, lists, _, _, iptEntries := translatePolicy(denyAllPolicy)

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
	expectedIptEntries = append(expectedIptEntries, getDefaultDropEntries("testnamespace", denyAllPolicy.Spec.PodSelector, true, false)...)
	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("translatedPolicy failed @ deny-all-policy policy comparison")
		marshalledIptEntries, _ := json.Marshal(iptEntries)
		marshalledExpectedIptEntries, _ := json.Marshal(expectedIptEntries)
		t.Errorf("iptEntries: %s", marshalledIptEntries)
		t.Errorf("expectedIptEntries: %s", marshalledExpectedIptEntries)
	}
}

func TestAllowBackendToFrontend(t *testing.T) {
	allowBackendToFrontendPolicy, err := readPolicyYaml("testpolicies/allow-backend-to-frontend.yaml")
	if err != nil {
		t.Fatal(err)
	}
	sets, _, lists, _, _, iptEntries := translatePolicy(allowBackendToFrontendPolicy)

	expectedSets := []string{
		"app:backend",
		"ns-testnamespace",
		"app:frontend",
	}
	if !reflect.DeepEqual(sets, expectedSets) {
		t.Errorf("translatedPolicy failed @ ALLOW-app:backend-TO-app:frontend-policy sets comparison")
		t.Errorf("sets: %v", sets)
		t.Errorf("expectedSets: %v", expectedSets)
	}

	expectedLists := []string{}
	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedPolicy failed @ ALLOW-app:backend-TO-app:frontend-policy lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	expectedIptEntries := []*iptm.IptEntry{}

	nonKubeSystemEntries := []*iptm.IptEntry{
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
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-testnamespace"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:backend"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesMark,
				util.IptablesSetMarkFlag,
				util.IptablesAzureIngressMarkHex,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-app:frontend-IN-ns-testnamespace-TO-app:backend-IN-ns-testnamespace",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressDropsChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-testnamespace"),
				util.IptablesDstFlag,
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
				"DROP-ALL-TO-app:backend-IN-ns-testnamespace",
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
}

func TestAllowAllToAppFrontend(t *testing.T) {
	allowToFrontendPolicy, err := readPolicyYaml("testpolicies/allow-all-to-app-frontend.yaml")
	if err != nil {
		t.Fatal(err)
	}

	sets, _, lists, _, _, iptEntries := translatePolicy(allowToFrontendPolicy)

	expectedSets := []string{
		"app:frontend",
		"ns-testnamespace",
	}
	if !reflect.DeepEqual(sets, expectedSets) {
		t.Errorf("translatedPolicy failed @ ALLOW-all-TO-app:frontend-policy sets comparison")
		t.Errorf("sets: %v", sets)
		t.Errorf("expectedSets: %v", expectedSets)
	}

	expectedLists := []string{}
	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedPolicy failed @ ALLOW-all-TO-app:frontend-policy lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	expectedIptEntries := []*iptm.IptEntry{}

	nonKubeSystemEntries := []*iptm.IptEntry{
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressPortChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-testnamespace"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:frontend"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesMark,
				util.IptablesSetMarkFlag,
				util.IptablesAzureIngressMarkHex,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ALL-TO-app:frontend-IN-ns-testnamespace",
			},
		},
	}
	expectedIptEntries = append(expectedIptEntries, nonKubeSystemEntries...)
	expectedIptEntries = append(expectedIptEntries, getDefaultDropEntries("testnamespace", allowToFrontendPolicy.Spec.PodSelector, false, false)...)
	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("translatedPolicy failed @ ALLOW-all-TO-app:frontend-policy policy comparison")
		marshalledIptEntries, _ := json.Marshal(iptEntries)
		marshalledExpectedIptEntries, _ := json.Marshal(expectedIptEntries)
		t.Errorf("iptEntries: %s", marshalledIptEntries)
		t.Errorf("expectedIptEntries: %s", marshalledExpectedIptEntries)
	}
}

func TestDenyAllToAppFrontend(t *testing.T) {
	denyAllToFrontendPolicy, err := readPolicyYaml("testpolicies/deny-all-to-app-frontend.yaml")
	if err != nil {
		t.Fatal(err)
	}

	sets, _, lists, _, _, iptEntries := translatePolicy(denyAllToFrontendPolicy)

	expectedSets := []string{
		"app:frontend",
		"ns-testnamespace",
	}
	if !reflect.DeepEqual(sets, expectedSets) {
		t.Errorf("translatedPolicy failed @ ALLOW-none-TO-app:frontend-policy sets comparison")
		t.Errorf("sets: %v", sets)
		t.Errorf("expectedSets: %v", expectedSets)
	}

	expectedLists := []string{}
	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedPolicy failed @ ALLOW-none-TO-app:frontend-policy lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	expectedIptEntries := []*iptm.IptEntry{}
	expectedIptEntries = append(expectedIptEntries, getDefaultDropEntries("testnamespace", denyAllToFrontendPolicy.Spec.PodSelector, true, false)...)
	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("translatedPolicy failed @ ALLOW-none-TO-app:frontend-policy policy comparison")
		marshalledIptEntries, _ := json.Marshal(iptEntries)
		marshalledExpectedIptEntries, _ := json.Marshal(expectedIptEntries)
		t.Errorf("iptEntries: %s", marshalledIptEntries)
		t.Errorf("expectedIptEntries: %s", marshalledExpectedIptEntries)
	}
}

func TestNamespaceToFrontend(t *testing.T) {
	allowNsTestNamespaceToFrontendPolicy, err := readPolicyYaml("testpolicies/allow-ns-test-namespace-to-frontend.yaml")
	if err != nil {
		t.Fatal(err)
	}

	sets, _, lists, _, _, iptEntries := translatePolicy(allowNsTestNamespaceToFrontendPolicy)

	expectedSets := []string{
		"app:frontend",
		"ns-testnamespace",
	}
	if !reflect.DeepEqual(sets, expectedSets) {
		t.Errorf("translatedPolicy failed @ ALLOW-ns-testnamespace-TO-app:frontend-policy sets comparison")
		t.Errorf("sets: %v", sets)
		t.Errorf("expectedSets: %v", expectedSets)
	}

	expectedLists := []string{}
	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedPolicy failed @ ALLOW-ns-testnamespace-TO-app:frontend-policy lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	expectedIptEntries := []*iptm.IptEntry{}
	nonKubeSystemEntries := []*iptm.IptEntry{
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
				util.GetHashedName("ns-testnamespace"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:frontend"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesMark,
				util.IptablesSetMarkFlag,
				util.IptablesAzureIngressMarkHex,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ns-testnamespace-TO-app:frontend-IN-ns-testnamespace",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressDropsChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-testnamespace"),
				util.IptablesDstFlag,
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
				"DROP-ALL-TO-app:frontend-IN-ns-testnamespace",
			},
		},
	}
	expectedIptEntries = append(expectedIptEntries, nonKubeSystemEntries...)
	expectedIptEntries = append(expectedIptEntries, getDefaultDropEntries("testnamespace", allowNsTestNamespaceToFrontendPolicy.Spec.PodSelector, false, false)...)
	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("translatedPolicy failed @ ALLOW-ns-testnamespace-TO-app:frontend-policy policy comparison")
		marshalledIptEntries, _ := json.Marshal(iptEntries)
		marshalledExpectedIptEntries, _ := json.Marshal(expectedIptEntries)
		t.Errorf("iptEntries: %s", marshalledIptEntries)
		t.Errorf("expectedIptEntries: %s", marshalledExpectedIptEntries)
	}
}

func TestAllowAllNamespacesToAppFrontend(t *testing.T) {
	allowAllNsToFrontendPolicy, err := readPolicyYaml("testpolicies/allow-all-ns-to-frontend.yaml")
	if err != nil {
		t.Fatal(err)
	}

	sets, _, lists, _, _, iptEntries := translatePolicy(allowAllNsToFrontendPolicy)
	expectedSets := []string{
		"app:frontend",
		"ns-testnamespace",
	}
	if !reflect.DeepEqual(sets, expectedSets) {
		t.Errorf("translatedPolicy failed @ ALLOW-all-namespaces-TO-app:frontend-policy sets comparison")
		t.Errorf("sets: %v", sets)
		t.Errorf("expectedSets: %v", expectedSets)
	}

	expectedLists := []string{
		util.KubeAllNamespacesFlag,
	}
	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedPolicy failed @ ALLOW-all-namespaces-TO-app:frontend-policy lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	expectedIptEntries := []*iptm.IptEntry{}
	nonKubeSystemEntries := []*iptm.IptEntry{
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
				util.GetHashedName("ns-testnamespace"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:frontend"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesMark,
				util.IptablesSetMarkFlag,
				util.IptablesAzureIngressMarkHex,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-all-namespaces-TO-app:frontend-IN-ns-testnamespace",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressDropsChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-testnamespace"),
				util.IptablesDstFlag,
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
				"DROP-ALL-TO-app:frontend-IN-ns-testnamespace",
			},
		},
	}
	expectedIptEntries = append(expectedIptEntries, nonKubeSystemEntries...)
	expectedIptEntries = append(expectedIptEntries, getDefaultDropEntries("testnamespace", allowAllNsToFrontendPolicy.Spec.PodSelector, false, false)...)
	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("translatedPolicy failed @ ALLOW-all-namespaces-TO-app:frontend-policy policy comparison")
		marshalledIptEntries, _ := json.Marshal(iptEntries)
		marshalledExpectedIptEntries, _ := json.Marshal(expectedIptEntries)
		t.Errorf("iptEntries: %s", marshalledIptEntries)
		t.Errorf("expectedIptEntries: %s", marshalledExpectedIptEntries)
	}
}

func TestAllowNamespaceDevToAppFrontend(t *testing.T) {
	allowNsDevToFrontendPolicy, err := readPolicyYaml("testpolicies/allow-ns-dev-to-app-frontend.yaml")
	if err != nil {
		t.Fatal(err)
	}

	sets, _, lists, _, _, iptEntries := translatePolicy(allowNsDevToFrontendPolicy)

	expectedSets := []string{
		"app:frontend",
		"ns-testnamespace",
	}
	if !reflect.DeepEqual(sets, expectedSets) {
		t.Errorf("translatedPolicy failed @ ALLOW-ns-namespace:dev-AND-!ns-namespace:test0-AND-!ns-namespace:test1-TO-app:frontend-policy sets comparison")
		t.Errorf("sets: %v", sets)
		t.Errorf("expectedSets: %v", expectedSets)
	}

	expectedLists := []string{
		"ns-namespace:dev",
		"ns-namespace:test0",
		"ns-namespace:test1",
	}
	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedPolicy failed @ ALLOW-ns-namespace:dev-AND-!ns-namespace:test0-AND-!ns-namespace:test1-TO-app:frontend-policy lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	expectedIptEntries := []*iptm.IptEntry{}
	nonKubeSystemEntries := []*iptm.IptEntry{
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
				util.GetHashedName("ns-testnamespace"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:frontend"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesMark,
				util.IptablesSetMarkFlag,
				util.IptablesAzureIngressMarkHex,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ns-namespace:dev-AND-ns-!namespace:test0-AND-ns-!namespace:test1-TO-app:frontend-IN-ns-testnamespace",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressDropsChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-testnamespace"),
				util.IptablesDstFlag,
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
				"DROP-ALL-TO-app:frontend-IN-ns-testnamespace",
			},
		},
	}

	expectedIptEntries = append(expectedIptEntries, nonKubeSystemEntries...)
	expectedIptEntries = append(expectedIptEntries, getDefaultDropEntries("testnamespace", allowNsDevToFrontendPolicy.Spec.PodSelector, false, false)...)
	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("translatedPolicy failed @ ALLOW-ns-namespace:dev-AND-!ns-namespace:test0-AND-!ns-namespace:test1-TO-app:frontend-policy policy comparison")
		marshalledIptEntries, _ := json.Marshal(iptEntries)
		marshalledExpectedIptEntries, _ := json.Marshal(expectedIptEntries)
		t.Errorf("iptEntries: %s", marshalledIptEntries)
		t.Errorf("expectedIptEntries: %s", marshalledExpectedIptEntries)
	}
}

func TestAllowAllToK0AndK1AndAppFrontend(t *testing.T) {
	allowAllToFrontendPolicy, err := readPolicyYaml("testpolicies/test-allow-all-to-k0-and-k1-and-app-frontend.yaml")
	if err != nil {
		t.Fatal(err)
	}

	sets, _, lists, _, _, iptEntries := translatePolicy(allowAllToFrontendPolicy)

	expectedSets := []string{
		"app:frontend",
		"k0",
		"k1:v0",
		"k1:v1",
		"ns-testnamespace",
	}
	if !reflect.DeepEqual(sets, expectedSets) {
		t.Errorf("translatedPolicy failed @ AllOW-ALL-TO-k0-AND-k1:v0-AND-k1:v1-AND-app:frontend-policy sets comparison")
		t.Errorf("sets: %v", sets)
		t.Errorf("expectedSets: %v", expectedSets)
	}

	expectedLists := []string{util.KubeAllNamespacesFlag}
	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedPolicy failed @ AllOW-ALL-TO-k0-AND-k1:v0-AND-k1:v1-AND-app:frontend-policy lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	expectedIptEntries := []*iptm.IptEntry{}
	nonKubeSystemEntries := []*iptm.IptEntry{
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
				util.GetHashedName("ns-testnamespace"),
				util.IptablesDstFlag,
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
				util.IptablesMark,
				util.IptablesSetMarkFlag,
				util.IptablesAzureIngressMarkHex,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-all-namespaces-TO-app:frontend-AND-!k0-AND-k1:v0-AND-k1:v1-IN-ns-testnamespace",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressDropsChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-testnamespace"),
				util.IptablesDstFlag,
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
				"DROP-ALL-TO-app:frontend-AND-!k0-AND-k1:v0-AND-k1:v1-IN-ns-testnamespace",
			},
		},
	}

	expectedIptEntries = append(expectedIptEntries, nonKubeSystemEntries...)
	expectedIptEntries = append(expectedIptEntries, getDefaultDropEntries("testnamespace", allowAllToFrontendPolicy.Spec.PodSelector, false, false)...)
	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("translatedPolicy failed @ AllOW-all-TO-k0-AND-k1:v0-AND-k1:v1-AND-app:frontend-policy policy comparison")
		marshalledIptEntries, _ := json.Marshal(iptEntries)
		marshalledExpectedIptEntries, _ := json.Marshal(expectedIptEntries)
		t.Errorf("iptEntries: %s", marshalledIptEntries)
		t.Errorf("expectedIptEntries: %s", marshalledExpectedIptEntries)
	}
}

func TestAllowNsDevAndAppBackendToAppFrontend(t *testing.T) {
	allowNsDevAndBackendToFrontendPolicy, err := readPolicyYaml("testpolicies/allow-ns-dev-and-app-backend-to-app-frontend.yaml")
	if err != nil {
		t.Fatal(err)
	}

	util.IsNewNwPolicyVerFlag = true
	sets, _, lists, _, _, iptEntries := translatePolicy(allowNsDevAndBackendToFrontendPolicy)

	expectedSets := []string{
		"app:frontend",
		"ns-testnamespace",
		"app:backend",
	}
	if !reflect.DeepEqual(sets, expectedSets) {
		t.Errorf("translatedPolicy failed @ ALLOW-ns-ns:dev-AND-app:backend-TO-app:frontend sets comparison")
		t.Errorf("sets: %v", sets)
		t.Errorf("expectedSets: %v", expectedSets)
	}

	expectedLists := []string{
		"ns-ns:dev",
	}
	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedPolicy failed @ ALLOW-ns-ns:dev-AND-app:backend-TO-app:frontend lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	expectedIptEntries := []*iptm.IptEntry{}
	nonKubeSystemEntries := []*iptm.IptEntry{
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressFromChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-testnamespace"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:frontend"),
				util.IptablesDstFlag,
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
				util.IptablesJumpFlag,
				util.IptablesMark,
				util.IptablesSetMarkFlag,
				util.IptablesAzureIngressMarkHex,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ns-ns:dev-AND-app:backend-TO-app:frontend-IN-ns-testnamespace",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressDropsChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-testnamespace"),
				util.IptablesDstFlag,
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
				"DROP-ALL-TO-app:frontend-IN-ns-testnamespace",
			},
		},
	}

	expectedIptEntries = append(expectedIptEntries, nonKubeSystemEntries...)
	expectedIptEntries = append(expectedIptEntries, getDefaultDropEntries("testnamespace", allowNsDevAndBackendToFrontendPolicy.Spec.PodSelector, false, false)...)
	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("translatedPolicy failed @ ALLOW-ns-ns:dev-AND-app:backend-TO-app:frontend policy comparison")
		marshalledIptEntries, _ := json.Marshal(iptEntries)
		marshalledExpectedIptEntries, _ := json.Marshal(expectedIptEntries)
		t.Errorf("iptEntries: %s", marshalledIptEntries)
		t.Errorf("expectedIptEntries: %s", marshalledExpectedIptEntries)
	}
}

func TestAllowInternalAndExternal(t *testing.T) {
	allowInternalAndExternalPolicy, err := readPolicyYaml("testpolicies/allow-internal-and-external.yaml")
	if err != nil {
		t.Fatal(err)
	}

	sets, _, lists, _, _, iptEntries := translatePolicy(allowInternalAndExternalPolicy)

	expectedSets := []string{
		"app:backdoor",
		"ns-dangerous",
	}
	if !reflect.DeepEqual(sets, expectedSets) {
		t.Errorf("translatedPolicy failed @ ALLOW-ALL-TO-app:backdoor-policy sets comparison")
		t.Errorf("sets: %v", sets)
		t.Errorf("expectedSets: %v", expectedSets)
	}

	expectedLists := []string{}
	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedPolicy failed @ ALLOW-ALL-TO-app:backdoor-policy lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	expectedIptEntries := []*iptm.IptEntry{}
	nonKubeSystemEntries := []*iptm.IptEntry{
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressPortChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-dangerous"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:backdoor"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesMark,
				util.IptablesSetMarkFlag,
				util.IptablesAzureIngressMarkHex,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ALL-TO-app:backdoor-IN-ns-dangerous",
			},
		},
	}

	expectedIptEntries = append(expectedIptEntries, nonKubeSystemEntries...)
	expectedIptEntries = append(expectedIptEntries, getDefaultDropEntries("dangerous", allowInternalAndExternalPolicy.Spec.PodSelector, false, false)...)
	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("translatedPolicy failed @ ALLOW-ALL-TO-app:backdoor-policy policy comparison")
		marshalledIptEntries, _ := json.Marshal(iptEntries)
		marshalledExpectedIptEntries, _ := json.Marshal(expectedIptEntries)
		t.Errorf("iptEntries: %s", marshalledIptEntries)
		t.Errorf("expectedIptEntries: %s", marshalledExpectedIptEntries)
	}
}

func TestAllowBackendToFrontendPort8000(t *testing.T) {
	allowBackendToFrontendPort8000Policy, err := readPolicyYaml("testpolicies/allow-app-backend-to-app-frontend-port-8000.yaml")
	if err != nil {
		t.Fatal(err)
	}

	sets, _, lists, _, _, iptEntries := translatePolicy(allowBackendToFrontendPort8000Policy)

	expectedSets := []string{
		"app:frontend",
		"ns-testnamespace",
		"app:backend",
	}
	if !reflect.DeepEqual(sets, expectedSets) {
		t.Errorf("translatedPolicy failed @ ALLOW-app:backend-TO-app:frontend-port-8000-policy sets comparison")
		t.Errorf("sets: %v", sets)
		t.Errorf("expectedSets: %v", expectedSets)
	}

	expectedLists := []string{}
	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedPolicy failed @ ALLOW-app:backend-TO-app:frontend-port-8000-policy lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	expectedIptEntries := []*iptm.IptEntry{}
	nonKubeSystemEntries := []*iptm.IptEntry{
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressPortChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-testnamespace"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:frontend"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-testnamespace"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:backend"),
				util.IptablesSrcFlag,
				util.IptablesDstPortFlag,
				"8000",
				util.IptablesJumpFlag,
				util.IptablesMark,
				util.IptablesSetMarkFlag,
				util.IptablesAzureIngressMarkHex,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-app:backend-IN-ns-testnamespace-AND-PORT-8000-TO-app:frontend-IN-ns-testnamespace",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressDropsChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-testnamespace"),
				util.IptablesDstFlag,
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
				"DROP-ALL-TO-app:frontend-IN-ns-testnamespace",
			},
		},
	}

	expectedIptEntries = append(expectedIptEntries, nonKubeSystemEntries...)
	expectedIptEntries = append(expectedIptEntries, getDefaultDropEntries("dangerous", allowBackendToFrontendPort8000Policy.Spec.PodSelector, false, false)...)
	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("translatedPolicy failed @ ALLOW-ALL-TO-app:backdoor-policy policy comparison")
		marshalledIptEntries, _ := json.Marshal(iptEntries)
		marshalledExpectedIptEntries, _ := json.Marshal(expectedIptEntries)
		t.Errorf("iptEntries: %s", marshalledIptEntries)
		t.Errorf("expectedIptEntries: %s", marshalledExpectedIptEntries)
	}
}

func TestAllowBackendToFrontendWithMissingPort(t *testing.T) {
	allowBackendToFrontendMissingPortPolicy, err := readPolicyYaml("testpolicies/allow-backend-to-frontend-with-missing-port.yaml")
	if err != nil {
		t.Fatal(err)
	}

	sets, _, lists, _, _, iptEntries := translatePolicy(allowBackendToFrontendMissingPortPolicy)

	expectedSets := []string{
		"app:frontend",
		"ns-testnamespace",
		"app:backend",
	}
	if !reflect.DeepEqual(sets, expectedSets) {
		t.Errorf("translatedPolicy failed @ ALLOW-app:backend-TO-app:frontend-port-8000-policy sets comparison")
		t.Errorf("sets: %v", sets)
		t.Errorf("expectedSets: %v", expectedSets)
	}

	expectedLists := []string{}
	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedPolicy failed @ ALLOW-app:backend-TO-app:frontend-port-8000-policy lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	expectedIptEntries := []*iptm.IptEntry{}
	nonKubeSystemEntries := []*iptm.IptEntry{
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressPortChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-testnamespace"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:frontend"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-testnamespace"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:backend"),
				util.IptablesSrcFlag,
				util.IptablesJumpFlag,
				util.IptablesMark,
				util.IptablesSetMarkFlag,
				util.IptablesAzureIngressMarkHex,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-app:backend-IN-ns-testnamespace-AND--TO-app:frontend-IN-ns-testnamespace",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressDropsChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-testnamespace"),
				util.IptablesDstFlag,
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
				"DROP-ALL-TO-app:frontend-IN-ns-testnamespace",
			},
		},
	}

	expectedIptEntries = append(expectedIptEntries, nonKubeSystemEntries...)
	expectedIptEntries = append(expectedIptEntries, getDefaultDropEntries("dangerous", allowBackendToFrontendMissingPortPolicy.Spec.PodSelector, false, false)...)
	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("translatedPolicy failed @ ALLOW-ALL-TO-app:backdoor-policy policy comparison")
		marshalledIptEntries, _ := json.Marshal(iptEntries)
		marshalledExpectedIptEntries, _ := json.Marshal(expectedIptEntries)
		t.Errorf("iptEntries: %s", marshalledIptEntries)
		t.Errorf("expectedIptEntries: %s", marshalledExpectedIptEntries)
	}
}

func TestAllowMultipleLabelsToMultipleLabels(t *testing.T) {
	allowCniOrCnsToK8sPolicy, err := readPolicyYaml("testpolicies/allow-multiple-labels-to-multiple-labels.yaml")
	if err != nil {
		t.Fatal(err)
	}

	sets, _, lists, _, _, iptEntries := translatePolicy(allowCniOrCnsToK8sPolicy)

	expectedSets := []string{
		"app:k8s",
		"team:aks",
		"ns-acn",
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

	expectedLists := []string{}
	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedPolicy failed @ ALLOW-program:cni-AND-team:acn-OR-binary:cns-AND-group:container-TO-app:k8s-AND-team:aks-policy lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	expectedIptEntries := []*iptm.IptEntry{}
	nonKubeSystemEntries := []*iptm.IptEntry{
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressFromChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-acn"),
				util.IptablesSrcFlag,
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
				util.GetHashedName("ns-acn"),
				util.IptablesDstFlag,
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
				util.IptablesMark,
				util.IptablesSetMarkFlag,
				util.IptablesAzureIngressMarkHex,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-program:cni-AND-team:acn-IN-ns-acn-TO-app:k8s-AND-team:aks-IN-ns-acn",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressFromChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-acn"),
				util.IptablesSrcFlag,
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
				util.GetHashedName("ns-acn"),
				util.IptablesDstFlag,
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
				util.IptablesMark,
				util.IptablesSetMarkFlag,
				util.IptablesAzureIngressMarkHex,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-binary:cns-AND-group:container-IN-ns-acn-TO-app:k8s-AND-team:aks-IN-ns-acn",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressDropsChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-acn"),
				util.IptablesDstFlag,
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
				"DROP-ALL-TO-app:k8s-AND-team:aks-IN-ns-acn",
			},
		},
	}

	expectedIptEntries = append(expectedIptEntries, nonKubeSystemEntries...)
	expectedIptEntries = append(expectedIptEntries, getDefaultDropEntries("acn", allowCniOrCnsToK8sPolicy.Spec.PodSelector, false, false)...)
	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("translatedPolicy failed @ ALLOW-program:cni-AND-team:acn-OR-binary:cns-AND-group:container-TO-app:k8s-AND-team:aks-policy policy comparison")
		marshalledIptEntries, _ := json.Marshal(iptEntries)
		marshalledExpectedIptEntries, _ := json.Marshal(expectedIptEntries)
		t.Errorf("iptEntries: %s", marshalledIptEntries)
		t.Errorf("expectedIptEntries: %s", marshalledExpectedIptEntries)
	}
}

func TestDenyAllFromAppBackend(t *testing.T) {
	denyAllFromBackendPolicy, err := readPolicyYaml("testpolicies/deny-all-from-app-backend.yaml")
	if err != nil {
		t.Fatal(err)
	}

	sets, _, lists, _, _, iptEntries := translatePolicy(denyAllFromBackendPolicy)

	expectedSets := []string{
		"app:backend",
		"ns-testnamespace",
	}
	if !reflect.DeepEqual(sets, expectedSets) {
		t.Errorf("translatedPolicy failed @ ALLOW-none-FROM-app:backend-policy sets comparison")
		t.Errorf("sets: %v", sets)
		t.Errorf("expectedSets: %v", expectedSets)
	}

	expectedLists := []string{}
	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedPolicy failed @ ALLOW-none-FROM-app:backend-policy lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	expectedIptEntries := []*iptm.IptEntry{}
	expectedIptEntries = append(expectedIptEntries, getDefaultDropEntries("testnamespace", denyAllFromBackendPolicy.Spec.PodSelector, false, true)...)
	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("translatedPolicy failed @ ALLOW-none-FROM-app:backend-policy policy comparison")
		marshalledIptEntries, _ := json.Marshal(iptEntries)
		marshalledExpectedIptEntries, _ := json.Marshal(expectedIptEntries)
		t.Errorf("iptEntries: %s", marshalledIptEntries)
		t.Errorf("expectedIptEntries: %s", marshalledExpectedIptEntries)
	}
}

func TestAllowAllFromAppBackend(t *testing.T) {
	allowAllEgress, err := readPolicyYaml("testpolicies/allow-all-from-app-backend.yaml")
	if err != nil {
		t.Fatal(err)
	}

	sets, _, lists, _, _, iptEntries := translatePolicy(allowAllEgress)

	expectedSets := []string{
		"app:backend",
		"ns-testnamespace",
	}
	if !reflect.DeepEqual(sets, expectedSets) {
		t.Errorf("translatedPolicy failed @ ALLOW-all-FROM-app:backend-policy sets comparison")
		t.Errorf("sets: %v", sets)
		t.Errorf("expectedSets: %v", expectedSets)
	}

	expectedLists := []string{}
	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedPolicy failed @ ALLOW-all-FROM-app:backend-policy lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	expectedIptEntries := []*iptm.IptEntry{}
	nonKubeSystemEntries := []*iptm.IptEntry{
		&iptm.IptEntry{
			Chain: util.IptablesAzureEgressPortChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-testnamespace"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:backend"),
				util.IptablesSrcFlag,
				util.IptablesJumpFlag,
				util.IptablesMark,
				util.IptablesSetMarkFlag,
				util.IptablesAzureEgressXMarkHex,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ALL-FROM-app:backend-IN-ns-testnamespace",
			},
		},
	}
	expectedIptEntries = append(expectedIptEntries, nonKubeSystemEntries...)
	// has egress, but empty map means allow all
	expectedIptEntries = append(expectedIptEntries, getDefaultDropEntries("testnamespace", allowAllEgress.Spec.PodSelector, false, false)...)
	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("translatedPolicy failed @ ALLOW-all-FROM-app:backend-policy policy comparison")
		marshalledIptEntries, _ := json.Marshal(iptEntries)
		marshalledExpectedIptEntries, _ := json.Marshal(expectedIptEntries)
		t.Errorf("iptEntries: %s", marshalledIptEntries)
		t.Errorf("expectedIptEntries: %s", marshalledExpectedIptEntries)
	}
}

func TestDenyAllFromNsUnsafe(t *testing.T) {
	denyAllFromNsUnsafePolicy, err := readPolicyYaml("testpolicies/deny-all-from-ns-unsafe.yaml")
	if err != nil {
		t.Fatal(err)
	}
	sets, _, lists, _, _, iptEntries := translatePolicy(denyAllFromNsUnsafePolicy)

	expectedSets := []string{
		"ns-unsafe",
	}
	if !reflect.DeepEqual(sets, expectedSets) {
		t.Errorf("translatedPolicy failed @ ALLOW-none-FROM-ns-unsafe-policy sets comparison")
		t.Errorf("sets: %v", sets)
		t.Errorf("expectedSets: %v", expectedSets)
	}
	expectedLists := []string{}
	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedPolicy failed @ ALLOW-none-FROM-app:backend-policy lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	expectedIptEntries := []*iptm.IptEntry{}
	expectedIptEntries = append(expectedIptEntries, getDefaultDropEntries("unsafe", denyAllFromNsUnsafePolicy.Spec.PodSelector, false, true)...)
	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("translatedPolicy failed @ ALLOW-none-FROM-app:backend-policy policy comparison")
		marshalledIptEntries, _ := json.Marshal(iptEntries)
		marshalledExpectedIptEntries, _ := json.Marshal(expectedIptEntries)
		t.Errorf("iptEntries: %s", marshalledIptEntries)
		t.Errorf("expectedIptEntries: %s", marshalledExpectedIptEntries)
	}
}

func TestAllowAppFrontendToTCPPort53UDPPort53Policy(t *testing.T) {
	allowFrontendToTCPPort53UDPPort53Policy, err := readPolicyYaml("testpolicies/allow-app-frontend-tcp-port-or-udp-port-53.yaml")
	if err != nil {
		t.Fatal(err)
	}

	sets, _, lists, _, _, iptEntries := translatePolicy(allowFrontendToTCPPort53UDPPort53Policy)

	expectedSets := []string{
		"app:frontend",
		"ns-testnamespace",
	}
	if !reflect.DeepEqual(sets, expectedSets) {
		t.Errorf("translatedPolicy failed @ ALLOW-ALL-FROM-app:frontend-TCP-PORT-53-OR-UDP-PORT-53-policy sets comparison")
		t.Errorf("sets: %v", sets)
		t.Errorf("expectedSets: %v", expectedSets)
	}

	expectedLists := []string{
		util.KubeAllNamespacesFlag,
	}
	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedPolicy failed @ ALLOW-ALL-FROM-app:frontend-TCP-PORT-53-OR-UDP-PORT-53-policy lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	expectedIptEntries := []*iptm.IptEntry{}
	nonKubeSystemEntries := []*iptm.IptEntry{
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
				util.GetHashedName("ns-testnamespace"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:frontend"),
				util.IptablesSrcFlag,
				util.IptablesJumpFlag,
				util.IptablesMark,
				util.IptablesSetMarkFlag,
				util.IptablesAzureEgressXMarkHex,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ALL-TO-TCP-PORT-53-FROM-app:frontend-IN-ns-testnamespace",
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
				util.GetHashedName("ns-testnamespace"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:frontend"),
				util.IptablesSrcFlag,
				util.IptablesJumpFlag,
				util.IptablesMark,
				util.IptablesSetMarkFlag,
				util.IptablesAzureEgressXMarkHex,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ALL-TO-UDP-PORT-53-FROM-app:frontend-IN-ns-testnamespace",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureEgressToChain,
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
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName(util.KubeAllNamespacesFlag),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesMark,
				util.IptablesSetMarkFlag,
				util.IptablesAzureEgressXMarkHex,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-app:frontend-IN-ns-testnamespace-TO-" +
					util.KubeAllNamespacesFlag,
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureEgressDropsChain,
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
				util.IptablesSrcFlag,
				util.IptablesJumpFlag,
				util.IptablesDrop,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"DROP-ALL-FROM-app:frontend-IN-ns-testnamespace",
			},
		},
	}
	expectedIptEntries = append(expectedIptEntries, nonKubeSystemEntries...)
	expectedIptEntries = append(expectedIptEntries, getDefaultDropEntries("testnamespace", allowFrontendToTCPPort53UDPPort53Policy.Spec.PodSelector, false, false)...)
	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("translatedPolicy failed @ ALLOW-ALL-FROM-app:frontend-TCP-PORT-53-OR-UDP-PORT-53-policy policy comparison")
		marshalledIptEntries, _ := json.Marshal(iptEntries)
		marshalledExpectedIptEntries, _ := json.Marshal(expectedIptEntries)
		t.Errorf("iptEntries: %s", marshalledIptEntries)
		t.Errorf("expectedIptEntries: %s", marshalledExpectedIptEntries)
	}
}

func TestComplexPolicy(t *testing.T) {
	k8sExamplePolicy, err := readPolicyYaml("testpolicies/complex-policy.yaml")
	k8sExamplePolicyDiffOrder, err := readPolicyYaml("testpolicies/complex-policy-diff-order.yaml")
	if err != nil {
		t.Fatal(err)
	}

	sets, _, lists, ingressIPCidrs, egressIPCidrs, iptEntries := translatePolicy(k8sExamplePolicy)
	setsDiffOrder, _, listsDiffOrder, ingressIPCidrsDiffOrder, egressIPCidrsDiffOrder, iptEntriesDiffOrder := translatePolicy(k8sExamplePolicyDiffOrder)

	expectedSets := []string{
		"role:db",
		"ns-default",
		"role:frontend",
	}
	if !reflect.DeepEqual(sets, expectedSets) || !reflect.DeepEqual(setsDiffOrder, expectedSets) {
		t.Errorf("translatedPolicy failed @ k8s-example-policy sets comparison")
		t.Errorf("sets: %v", sets)
		t.Errorf("expectedSets: %v", expectedSets)
	}

	expectedLists := []string{
		"ns-project:myproject",
	}
	if !reflect.DeepEqual(lists, expectedLists) || !reflect.DeepEqual(listsDiffOrder, expectedLists) {
		t.Errorf("translatedPolicy failed @ k8s-example-policy lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	expectedIngressIPCidrs := [][]string{
		{"", "", "", "172.17.0.0/16", "172.17.1.0/24nomatch"},
	}

	expectedEgressIPCidrs := [][]string{
		{"", "10.0.0.0/24", "10.0.0.1/32nomatch"},
	}

	if !reflect.DeepEqual(ingressIPCidrs, expectedIngressIPCidrs) || !reflect.DeepEqual(ingressIPCidrsDiffOrder, expectedIngressIPCidrs) {
		t.Errorf("translatedPolicy failed @ k8s-example-policy ingress IP Cidrs comparison")
		t.Errorf("ingress IP Cidrs: %v", ingressIPCidrs)
		t.Errorf("expected ingress IP Cidrs: %v", expectedIngressIPCidrs)
	}

	if !reflect.DeepEqual(egressIPCidrs, expectedEgressIPCidrs) || !reflect.DeepEqual(egressIPCidrsDiffOrder, expectedEgressIPCidrs) {
		t.Errorf("translatedPolicy failed @ k8s-example-policy egress IP Cidrs comparison")
		t.Errorf("egress IP Cidrs: %v", egressIPCidrs)
		t.Errorf("expected egress IP Cidrs: %v", expectedEgressIPCidrs)
	}

	cidrIngressIpsetName := "k8s-example-policy" + "-in-ns-" + "default-" + "0" + "in"
	cidrEgressIpsetName := "k8s-example-policy" + "-in-ns-" + "default-" + "0" + "out"
	expectedIptEntries := []*iptm.IptEntry{}
	nonKubeSystemEntries := []*iptm.IptEntry{
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressPortChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-default"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("role:db"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName(cidrIngressIpsetName),
				util.IptablesSrcFlag,
				util.IptablesProtFlag,
				"TCP",
				util.IptablesDstPortFlag,
				"6379",
				util.IptablesJumpFlag,
				util.IptablesMark,
				util.IptablesSetMarkFlag,
				util.IptablesAzureIngressMarkHex,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-" + cidrIngressIpsetName + "-AND-TCP-PORT-6379-TO-role:db-IN-ns-default",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressPortChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-default"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("role:db"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-project:myproject"),
				util.IptablesSrcFlag,
				util.IptablesProtFlag,
				"TCP",
				util.IptablesDstPortFlag,
				"6379",
				util.IptablesJumpFlag,
				util.IptablesMark,
				util.IptablesSetMarkFlag,
				util.IptablesAzureIngressMarkHex,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ns-project:myproject-AND-TCP-PORT-6379-TO-role:db-IN-ns-default",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressPortChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-default"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("role:db"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-default"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("role:frontend"),
				util.IptablesSrcFlag,
				util.IptablesProtFlag,
				"TCP",
				util.IptablesDstPortFlag,
				"6379",
				util.IptablesJumpFlag,
				util.IptablesMark,
				util.IptablesSetMarkFlag,
				util.IptablesAzureIngressMarkHex,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-role:frontend-IN-ns-default-AND-TCP-PORT-6379-TO-role:db-IN-ns-default",
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
				util.GetHashedName("ns-default"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("role:db"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName(cidrEgressIpsetName),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesMark,
				util.IptablesSetMarkFlag,
				util.IptablesAzureEgressXMarkHex,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-" + cidrEgressIpsetName + "-AND-TCP-PORT-5978-FROM-role:db-IN-ns-default",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressDropsChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-default"),
				util.IptablesDstFlag,
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
				"DROP-ALL-TO-role:db-IN-ns-default",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureEgressDropsChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-default"),
				util.IptablesSrcFlag,
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
				"DROP-ALL-FROM-role:db-IN-ns-default",
			},
		},
	}
	expectedIptEntries = append(expectedIptEntries, nonKubeSystemEntries...)
	expectedIptEntries = append(expectedIptEntries, getDefaultDropEntries("testnamespace", k8sExamplePolicy.Spec.PodSelector, false, false)...)
	if !reflect.DeepEqual(iptEntries, expectedIptEntries) || !reflect.DeepEqual(iptEntriesDiffOrder, expectedIptEntries) {
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
	denyAllPolicy.ObjectMeta.Namespace = metav1.NamespaceDefault
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

	sets, _, lists, _, _, iptEntries := translatePolicy(denyAllPolicy)
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

	sets, _, lists, _, _, finalIptEntries := translatePolicy(allowToPodPolicy)
	expectedSets = []string{
		"app:test",
		"testIn:pod-A",
		"ns-default",
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
			Chain: util.IptablesAzureIngressDropsChain,
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
				util.GetHashedName("ns-default"),
				util.IptablesSrcFlag,
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
				util.GetHashedName("ns-default"),
				util.IptablesDstFlag,
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
				util.IptablesMark,
				util.IptablesSetMarkFlag,
				util.IptablesAzureIngressMarkHex,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-app:test-AND-testIn:pod-B-IN-ns-default-TO-app:test-AND-testIn:pod-A-IN-ns-default",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressFromChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-default"),
				util.IptablesSrcFlag,
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
				util.GetHashedName("ns-default"),
				util.IptablesDstFlag,
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
				util.IptablesMark,
				util.IptablesSetMarkFlag,
				util.IptablesAzureIngressMarkHex,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-app:test-AND-testIn:pod-C-IN-ns-default-TO-app:test-AND-testIn:pod-A-IN-ns-default",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureEgressToChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-default"),
				util.IptablesSrcFlag,
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
				util.IptablesMark,
				util.IptablesSetMarkFlag,
				util.IptablesAzureEgressXMarkHex,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-app:test-AND-testIn:pod-A-IN-ns-default-TO-all-namespaces",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressDropsChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-default"),
				util.IptablesDstFlag,
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
				"DROP-ALL-TO-app:test-AND-testIn:pod-A-IN-ns-default",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureEgressDropsChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-default"),
				util.IptablesSrcFlag,
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
				"DROP-ALL-FROM-app:test-AND-testIn:pod-A-IN-ns-default",
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

func TestNamedPorts(t *testing.T) {
	namedPortPolicy, err := readPolicyYaml("testpolicies/named-port.yaml")
	if err != nil {
		t.Fatal(err)
	}

	sets, namedPorts, lists, _, _, iptEntries := translatePolicy(namedPortPolicy)

	expectedSets := []string{
		"app:server",
		"ns-test",
	}
	if !reflect.DeepEqual(sets, expectedSets) {
		t.Errorf("translatedPolicy failed @ ALLOW-ALL-TCP-PORT-serve-80-TO-app:server-IN-ns-test-policy sets comparison")
		t.Errorf("sets: %v", sets)
		t.Errorf("expectedSets: %v", expectedSets)
	}

	expectedNamedPorts := []string{
		"namedport:serve-80",
	}
	if !reflect.DeepEqual(namedPorts, expectedNamedPorts) {
		t.Errorf("translatedPolicy failed @ ALLOW-ALL-TCP-PORT-serve-80-TO-app:server-IN-ns-test-policy namedPorts comparison")
		t.Errorf("sets: %v", namedPorts)
		t.Errorf("expectedSets: %v", expectedNamedPorts)
	}

	expectedLists := []string{}
	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedPolicy failed @ ALLOW-ALL-TCP-PORT-serve-80-TO-app:server-IN-ns-test-policy lists comparison")
		t.Errorf("lists: %v", lists)
		t.Errorf("expectedLists: %v", expectedLists)
	}

	expectedIptEntries := []*iptm.IptEntry{}
	nonKubeSystemEntries := []*iptm.IptEntry{
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressPortChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-test"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:server"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("namedport:serve-80"),
				util.IptablesDstFlag + "," + util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesMark,
				util.IptablesSetMarkFlag,
				util.IptablesAzureIngressMarkHex,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ALL-TCP-PORT-serve-80-TO-app:server-IN-ns-test",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressDropsChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-test"),
				util.IptablesDstFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:server"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesDrop,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"DROP-ALL-TO-app:server-IN-ns-test",
			},
		},
	}

	expectedIptEntries = append(expectedIptEntries, nonKubeSystemEntries...)
	expectedIptEntries = append(expectedIptEntries, getDefaultDropEntries("test", namedPortPolicy.Spec.PodSelector, false, false)...)
	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("translatedPolicy failed @ ALLOW-ALL-TCP-PORT-serve-80-TO-app:server-IN-ns-test policy comparison")
		marshalledIptEntries, _ := json.Marshal(iptEntries)
		marshalledExpectedIptEntries, _ := json.Marshal(expectedIptEntries)
		t.Errorf("iptEntries: %s", marshalledIptEntries)
		t.Errorf("expectedIptEntries: %s", marshalledExpectedIptEntries)
	}
}
