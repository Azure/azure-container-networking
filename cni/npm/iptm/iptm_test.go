package iptm

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

)

func TestParseIngress(t *testing.T) {
	iptMgr := &IptablesManager{
		entryMap: make(map[string][]*iptEntry),
	}

	tcp, udp := corev1.ProtocolTCP, corev1.ProtocolUDP
	ruleOne := networkingv1.NetworkPolicyIngressRule{
		From: []networkingv1.NetworkPolicyPeer{{
			PodSelector : &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
		}},
		Ports: []networkingv1.NetworkPolicyPort{{
			Protocol: &tcp,
			Port: &intstr.IntOrString{
				StrVal: "8000",
			},
		}},
	}

	ruleTwo := networkingv1.NetworkPolicyIngressRule{
		From: []networkingv1.NetworkPolicyPeer{{
			PodSelector : &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
		}},
		Ports: []networkingv1.NetworkPolicyPort{{
			Protocol: &udp,
			Port: &intstr.IntOrString{
				StrVal: "8001",
			},
		}},
	}	

	var rules []networkingv1.NetworkPolicyIngressRule
	rules = append(rules, ruleOne)
	rules = append(rules, ruleTwo)

	ipsetName := "testIpsetName"

	if err := iptMgr.parseIngress(ipsetName, rules); err != nil {
		t.Errorf("TestParseIngress failed")
	}
}