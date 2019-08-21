// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/kalebmorris/azure-container-networking/npm/util"
	"github.com/kalebmorris/azure-container-networking/telemetry"
	corev1 "k8s.io/api/core/v1"
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

	startsIPs := []string{
		"10.240.0.0",
		"10.240.12.0",
		"10.240.224.0",
	}
	startsTruth := make([]uint32, len(startsIPs))
	for i, ip := range startsIPs {
		converted, err := ipToInt(ip)
		if err != nil {
			t.Errorf("TestGetRanges failed @ ipToInt")
		}
		startsTruth[i] = converted
	}

	endsIPs := []string{
		"10.240.9.255",
		"10.240.219.255",
		"10.240.255.255",
	}
	endsTruth := make([]uint32, len(endsIPs))
	for i, ip := range endsIPs {
		converted, err := ipToInt(ip)
		if err != nil {
			t.Errorf("TestGetRanges failed @ ipToInt")
		}
		endsTruth[i] = converted
	}

	if !reflect.DeepEqual(starts, startsTruth) {
		t.Errorf("TestGetRanges failed @ starts comparison")
	}

	if !reflect.DeepEqual(ends, endsTruth) {
		t.Errorf("TestGetRanges failed @ ends comparison")
	}
}

func TestGetStrCIDR(t *testing.T) {
	strCIDRs := []string{
		"0.0.0.0/16",
		"255.0.1.16/20",
		"10.240.0.0/24",
		"12.144.2.1/31",
		"240.220.10.6/18",
		"11.82.80.0/21",
	}

	var reconstructed []string
	for _, strCIDR := range strCIDRs {
		arrCIDR := strings.Split(strCIDR, "/")
		ip, err := ipToInt(arrCIDR[0])
		if err != nil {
			t.Errorf("TestGetStrCIDR failed @ ipToInt")
		}
		maskNum64, err := strconv.ParseInt(arrCIDR[1], 10, 6)
		if err != nil {
			t.Errorf("TestGetStrCIDR failed @ strconv.ParseUint")
		}
		maskNum := int(maskNum64)
		reconstructed = append(reconstructed, getStrCIDR(ip, maskNum))
	}

	if !reflect.DeepEqual(strCIDRs, reconstructed) {
		t.Errorf("TestGetStrCIDR failed @ strCIDRs comparison")
	}
}

func TestGetCIDRs(t *testing.T) {
	ipblock := &networkingv1.IPBlock{
		CIDR: "10.240.6.6/16",
		Except: []string{
			"10.240.10.2/24",
			"10.240.11.4/24",
			"10.240.221.0/22",
			"10.235.0.0/30",
		},
	}

	CIDRs := getCIDRs(ipblock)
	CIDRsTruth := "10.240.0.0/21,10.240.8.0/23,10.240.12.0/22,10.240.16.0/20,10.240.32.0/19,10.240.64.0/18,10.240.128.0/18,10.240.192.0/20,10.240.208.0/21,10.240.216.0/22,10.240.224.0/19"
	if CIDRs != CIDRsTruth {
		t.Errorf("TestGetCIDRs failed @ CIDRs comparison")
	}
}

func TestGetAffectedNamespaces(t *testing.T) {
	npMgr := &NetworkPolicyManager{
		nsMap:            make(map[string]*namespace),
		TelemetryEnabled: false,
		reportManager: &telemetry.ReportManager{
			ContentType: telemetry.ContentType,
			Report:      &telemetry.NPMReport{},
		},
	}

	allNs, err := newNs(util.KubeAllNamespacesFlag)
	if err != nil {
		panic(err.Error)
	}
	npMgr.nsMap[util.KubeAllNamespacesFlag] = allNs

	tMgr := allNs.tMgr
	if err := tMgr.Save(util.TagTestConfigFile); err != nil {
		t.Errorf("TestAddNamespace failed @ tMgr.Save")
	}

	defer func() {
		if err := tMgr.Restore(util.TagTestConfigFile); err != nil {
			t.Errorf("TestAddNamespace failed @ tMgr.Restore")
		}
	}()

	nsObjOne := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-namespace-one",
			Labels: map[string]string{
				"app": "test-namespace-one",
			},
		},
	}

	if err := npMgr.AddNamespace(nsObjOne); err != nil {
		t.Errorf("TestAddNamespace @ npMgr.AddNamespace")
	}

	nsObjTwo := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-namespace-two",
			Labels: map[string]string{
				"app": "test-namespace-two",
			},
		},
	}

	if err := npMgr.AddNamespace(nsObjTwo); err != nil {
		t.Errorf("TestAddNamespace @ npMgr.AddNamespace")
	}

	matchLabels := map[string]string{
		"app": "test-namespace-one",
	}

	affectedNamespaces, NLTags := getAffectedNamespaces(matchLabels, tMgr)
	affectedNamespacesTruth := []string{
		"test-namespace-two",
	}
	NLTagsTruth := []string{
		util.GetNsIpsetName("app", "test-namespace-one"),
	}

	if !reflect.DeepEqual(affectedNamespaces, affectedNamespacesTruth) {
		t.Errorf("TestGetAffectedNamespaces failed @ affectedNamespaces comparison")
	}

	if !reflect.DeepEqual(NLTags, NLTagsTruth) {
		t.Errorf("TestGetAffectedNamespaces failed @ NLTags comparison")
	}
}
