package npm

import (
	"testing"
	"reflect"

	"github.com/Azure/azure-container-networking/npm/iptm"
	"github.com/Azure/azure-container-networking/npm/util"
	"k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestCraftPartialIptEntrySpecFromOpAndLabel(t *testing.T) {
	srcOp, srcLabel := "", "src"
	iptEntry := craftPartialIptEntrySpecFromOpAndLabel(srcOp, srcLabel, util.IptablesSrcFlag)
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
	iptEntry = craftPartialIptEntrySpecFromOpAndLabel(dstOp, dstLabel, util.IptablesDstFlag)
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


	srcIptEntry := craftPartialIptEntrySpecFromOpsAndLabels(srcOps, srcLabels, util.IptablesSrcFlag)
	dstIptEntry := craftPartialIptEntrySpecFromOpsAndLabels(dstOps, dstLabels, util.IptablesDstFlag)
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
	srcSelector := metav1.LabelSelector{
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

	labelsWithOps, _, _ := ParseSelector(&srcSelector)
	ops, labelsWithoutOps := GetOperatorsAndLabels(labelsWithOps)
	iptEntrySpec := craftPartialIptEntrySpecFromOpsAndLabels(ops, labelsWithoutOps, util.IptablesSrcFlag)
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
					PodSelector: ingressNamespaceSelector,
				},
				compositeNetworkPolicyPeer,
			},
		},
	}

	sets, lists, iptEntries := translateIngress(ns, targetSelector, rules)
	expectedSets := []string{
		"context:dev",
		"!testNotIn:frontend",
		"app:db",
		"testIn:frontend",
	}

	if !reflect.DeepEqual(sets, expectedSets) {
		t.Errorf("translatedIngress failed @ sets comparison")
		t.Errorf("sets: %v\nexpectedSets: %v", sets, expectedSets)
	}

	expectedLists := []string{
		"ns:dev",
		"ns-testIn:frontendns",
	}

	if !reflect.DeepEqual(lists, expectedLists) {
		t.Errorf("translatedIngress failed @ lists comparison")
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
				"ALLOW-TO-6783-PORT-OF-context:dev-AND-!testNotIn:frontend",
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
				"ALLOW-app:db-AND-testIn:frontend-TO-context:dev-AND-testNotIn:frontend",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressFromChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns:dev"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("testIn:frontendns"),
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
				"ALLOW-ns:dev-AND-testIn:frontendns-TO-context:dev-AND-!testNotIn:frontend",
			},
		},
		&iptm.IptEntry{
			Chain: util.IptablesAzureIngressFromChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("planet:earth"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("keyExists"),
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
				"ALLOW-planet:earth-AND-keyExists-AND-region:northpole-AND-!k-TO-context:dev-AND-!testNotIn:frontend",
			},
		},
	}

	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("translatedIngress failed @ iptEntries comparison")
	}
}
