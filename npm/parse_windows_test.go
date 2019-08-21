// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"reflect"
	"strings"
	"testing"

	"github.com/kalebmorris/azure-container-networking/npm/util"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetTargetTags(t *testing.T) {
	netPol := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-policy",
			Namespace: "test-nwpolicy",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":     "frontend",
					"purpose": "portal",
				},
			},
		},
	}

	reconstructed := make(map[string]string)
	targetTags := getTargetTags(netPol)
	for _, tag := range targetTags {
		idx := strings.Index(tag, util.KubeAllNamespacesFlag)
		if idx == -1 {
			continue
		}
		tag = tag[idx+len(util.KubeAllNamespacesFlag)+1:]
		idx = strings.Index(tag, ":")
		if idx == -1 {
			continue
		}
		key := tag[:idx]
		val := tag[idx+1:]
		reconstructed[key] = val
	}

	if !reflect.DeepEqual(netPol.Spec.PodSelector.MatchLabels, reconstructed) {
		t.Errorf("TestGetTargetTags failed")
	}
}

func TestGetPolicyTypes(t *testing.T) {
	bothPolTypes := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-policy",
			Namespace: "test-nwpolicy",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeEgress,
				networkingv1.PolicyTypeIngress,
			},
		},
	}

	ingressExists, egressExists := getPolicyTypes(bothPolTypes)
	if !ingressExists || !egressExists {
		t.Errorf("TestGetPolicyTypes failed")
	}

	neitherPolTypes := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-policy",
			Namespace: "test-nwpolicy",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PolicyTypes: []networkingv1.PolicyType{},
		},
	}

	ingressExists, egressExists = getPolicyTypes(neitherPolTypes)
	if ingressExists || egressExists {
		t.Errorf("TestGetPolicyTypes failed")
	}
}

func TestIpToInt(t *testing.T) {
	ip, err := ipToInt("0.1.2.3")
	if err != nil || ip != uint32(66051) {
		t.Errorf("TestIpToInt failed @ ipToInt")
	}

	ip, err = ipToInt("3.2.1.0")
	if err != nil || ip != uint32(50462976) {
		t.Errorf("TestIpToInt failed @ ipToInt")
	}
}

func TestGetRanges(t *testing.T) {
	ipblock := &networkingv1.IPBlock{
		CIDR: "10.240.6.6/16",
		Except: []string{
			"10.240.10.2/24",
			"10.240.11.4/24",
			"10.240.221.0/22",
			"10.235.0.0/30",
		},
	}

	starts, ends := getRanges(ipblock)
	startsTruth := []uint32{
		ipToInt("10.240.0.0"),
		ipToInt("10.240.12.0"),
		ipToInt("10.240.224.0"),
	}
	endsTruth := []uint32{
		ipToInt("10.240.9.255"),
		ipToInt("10.240.219.255"),
		ipToInt("10.240.255.255"),
	}

	if !reflect.DeepEqual(starts, startsTruth) {
		t.Errorf("TestGetRanges failed @ starts comparison")
	}

	if !reflect.DeepEqual(ends, endsTruth) {
		t.Errorf("TestGetRanges failed @ ends comparison")
	}
}
