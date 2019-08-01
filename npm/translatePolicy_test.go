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
			Name:  "allow-tcp:6783-to-context:dev",
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
				util.IptablesJumpFlag,
				util.IptablesAzureIngressFromNsChain,
			},
		},
		&iptm.IptEntry{
			Name:  "allow-app:db-to-context:dev",
			Chain: util.IptablesAzureIngressFromPodChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:db"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("context:dev"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAccept,
			},
		},
		&iptm.IptEntry{
			Name:  "allow-testIn:frontend-to-context:dev",
			Chain: util.IptablesAzureIngressFromPodChain,
			Specs: []string{
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
				util.IptablesJumpFlag,
				util.IptablesAccept,
			},
		},
		&iptm.IptEntry{
			Name:  "allow-ns-ns:dev-to-context:dev",
			Chain: util.IptablesAzureIngressFromNsChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-ns:dev"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("context:dev"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAccept,
			},
		},
		&iptm.IptEntry{
			Name:  "allow-ns-testIn:frontendns-to-context:dev",
			Chain: util.IptablesAzureIngressFromNsChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-testIn:frontends"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("context:dev"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAccept,
			},
		},
		&iptm.IptEntry{
			Name:  "allow-tcp:6783-to-testNotIn:frontend",
			Chain: util.IptablesAzureIngressPortChain,
			Specs: []string{
				util.IptablesProtFlag,
				string(v1.ProtocolTCP),
				util.IptablesDstPortFlag,
				"6783",
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("!testNotIn:frontend"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAzureIngressFromNsChain,
			},
		},
		&iptm.IptEntry{
			Name:  "allow-app:db-to-testNotIn:frontend",
			Chain: util.IptablesAzureIngressFromPodChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("app:db"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("!testNotIn:frontend"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAccept,
			},
		},
		&iptm.IptEntry{
			Name:  "allow-testIn:frontend-to-testNotIn:frontend",
			Chain: util.IptablesAzureIngressFromChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("testIn:frontend"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("!testNotIn:frontend"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAccept,
			},
		},
		&iptm.IptEntry{
			Name:  "allow-ns-ns:dev-to-testNotIn:frontend",
			Chain: util.IptablesAzureIngressFromNsChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-ns:dev"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("!testNotIn:frontend"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAccept,
			},
		},
		&iptm.IptEntry{
			Name:  "allow-ns-testIn:frontendns-to-testNotIn:frontend",
			Chain: util.IptablesAzureIngressFromNsChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("ns-testIn:frontendns"),
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName("!testNotIn:frontend"),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAccept,
			},
		},
		&iptm.IptEntry{
			Name:  "TODO",
			Chain: "TODO",
			Specs: []string{},
		},
		&iptm.IptEntry{
			Name:  "TODO",
			Chain: "TODO",
			Specs: []string{},
		},
		&iptm.IptEntry{
			Name:  "TODO",
			Chain: "TODO",
			Specs: []string{},
		},
		&iptm.IptEntry{
			Name:  "TODO",
			Chain: "TODO",
			Specs: []string{},
		},
		&iptm.IptEntry{
			Name:  "TODO",
			Chain: "TODO",
			Specs: []string{},
		},
		&iptm.IptEntry{
			Name:  "TODO",
			Chain: "TODO",
			Specs: []string{},
		},
		&iptm.IptEntry{
			Name:  "TODO",
			Chain: "TODO",
			Specs: []string{},
		},
		&iptm.IptEntry{
			Name:  "TODO",
			Chain: "TODO",
			Specs: []string{},
		},
	}

	if !reflect.DeepEqual(iptEntries, expectedIptEntries) {
		t.Errorf("translatedIngress failed @ iptEntries comparison")
	}
}
