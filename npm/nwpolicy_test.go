// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"testing"

	"github.com/Azure/azure-container-networking/npm/ipsm"
	"github.com/Azure/azure-container-networking/npm/iptm"
	"github.com/Azure/azure-container-networking/npm/util"
	"github.com/Azure/azure-container-networking/telemetry"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestAddNetworkPolicy(t *testing.T) {
	npMgr := &NetworkPolicyManager{
		nsMap:            make(map[string]*namespace),
		TelemetryEnabled: false,
		reportManager: &telemetry.ReportManager{
			HostNetAgentURL: hostNetAgentURLForNpm,
			ContentType:     contentType,
			Report:          &telemetry.NPMReport{},
		},
	}

	allNs, err := newNs(util.KubeAllNamespacesFlag)
	if err != nil {
		panic(err.Error)
	}
	npMgr.nsMap[util.KubeAllNamespacesFlag] = allNs

	iptMgr := iptm.NewIptablesManager()
	if err := iptMgr.Save(util.IptablesTestConfigFile); err != nil {
		t.Errorf("TestAddNetworkPolicy failed @ iptMgr.Save")
	}

	ipsMgr := ipsm.NewIpsetManager()
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestAddNetworkPolicy failed @ ipsMgr.Save")
	}

	defer func() {
		if err := iptMgr.Restore(util.IptablesTestConfigFile); err != nil {
			t.Errorf("TestAddNetworkPolicy failed @ iptMgr.Restore")
		}

		if err := ipsMgr.Restore(util.IpsetTestConfigFile); err != nil {
			t.Errorf("TestAddNetworkPolicy failed @ ipsMgr.Restore")
		}
	}()

	nsObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-nwpolicy",
			Labels: map[string]string{
				"app": "test-namespace",
			},
		},
	}

	if err := npMgr.AddNamespace(nsObj); err != nil {
		t.Errorf("TestAddNetworkPolicy @ npMgr.AddNamespace")
	}

	tcp := corev1.ProtocolTCP
	allow := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "allow-ingress",
			Namespace: "test-nwpolicy",
		},
		Spec: networkingv1.NetworkPolicySpec{
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				networkingv1.NetworkPolicyIngressRule{
					From: []networkingv1.NetworkPolicyPeer{{
						PodSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "test"},
						},
					}},
					Ports: []networkingv1.NetworkPolicyPort{{
						Protocol: &tcp,
						Port: &intstr.IntOrString{
							StrVal: "8000",
						},
					}},
				},
			},
		},
	}

	if err := npMgr.AddNetworkPolicy(allow); err != nil {
		t.Errorf("TestAddNetworkPolicy failed @ AddNetworkPolicy")
	}
}

func TestUpdateNetworkPolicy(t *testing.T) {
	npMgr := &NetworkPolicyManager{
		nsMap:            make(map[string]*namespace),
		TelemetryEnabled: false,
		reportManager: &telemetry.ReportManager{
			HostNetAgentURL: hostNetAgentURLForNpm,
			ContentType:     contentType,
			Report:          &telemetry.NPMReport{},
		},
	}

	allNs, err := newNs(util.KubeAllNamespacesFlag)
	if err != nil {
		panic(err.Error)
	}
	npMgr.nsMap[util.KubeAllNamespacesFlag] = allNs

	iptMgr := iptm.NewIptablesManager()
	if err := iptMgr.Save(util.IptablesTestConfigFile); err != nil {
		t.Errorf("UpdateAddNetworkPolicy failed @ iptMgr.Save")
	}

	ipsMgr := ipsm.NewIpsetManager()
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("UpdateAddNetworkPolicy failed @ ipsMgr.Save")
	}

	defer func() {
		if err := iptMgr.Restore(util.IptablesTestConfigFile); err != nil {
			t.Errorf("UpdateAddNetworkPolicy failed @ iptMgr.Restore")
		}

		if err := ipsMgr.Restore(util.IpsetTestConfigFile); err != nil {
			t.Errorf("UpdateAddNetworkPolicy failed @ ipsMgr.Restore")
		}
	}()

	nsObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-nwpolicy",
			Labels: map[string]string{
				"app": "test-namespace",
			},
		},
	}

	if err := npMgr.AddNamespace(nsObj); err != nil {
		t.Errorf("TestAddNetworkPolicy @ npMgr.AddNamespace")
	}

	tcp, udp := corev1.ProtocolTCP, corev1.ProtocolUDP
	allowIngress := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "allow-ingress",
			Namespace: "test-nwpolicy",
		},
		Spec: networkingv1.NetworkPolicySpec{
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				networkingv1.NetworkPolicyIngressRule{
					From: []networkingv1.NetworkPolicyPeer{{
						PodSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "test"},
						},
					}},
					Ports: []networkingv1.NetworkPolicyPort{{
						Protocol: &tcp,
						Port: &intstr.IntOrString{
							StrVal: "8000",
						},
					}},
				},
			},
		},
	}

	allowEgress := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "allow-egress",
			Namespace: "test-nwpolicy",
		},
		Spec: networkingv1.NetworkPolicySpec{
			Egress: []networkingv1.NetworkPolicyEgressRule{
				networkingv1.NetworkPolicyEgressRule{
					To: []networkingv1.NetworkPolicyPeer{{
						NamespaceSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"ns": "test"},
						},
					}},
					Ports: []networkingv1.NetworkPolicyPort{{
						Protocol: &udp,
						Port: &intstr.IntOrString{
							StrVal: "8001",
						},
					}},
				},
			},
		},
	}

	if err := npMgr.AddNetworkPolicy(allowIngress); err != nil {
		t.Errorf("TestUpdateNetworkPolicy failed @ AddNetworkPolicy")
	}

	if err := npMgr.UpdateNetworkPolicy(allowIngress, allowEgress); err != nil {
		t.Errorf("TestUpdateNetworkPolicy failed @ UpdateNetworkPolicy")
	}
}

func TestDeleteNetworkPolicy(t *testing.T) {
	npMgr := &NetworkPolicyManager{
		nsMap:            make(map[string]*namespace),
		TelemetryEnabled: false,
		reportManager: &telemetry.ReportManager{
			HostNetAgentURL: hostNetAgentURLForNpm,
			ContentType:     contentType,
			Report:          &telemetry.NPMReport{},
		},
	}

	allNs, err := newNs(util.KubeAllNamespacesFlag)
	if err != nil {
		panic(err.Error)
	}
	npMgr.nsMap[util.KubeAllNamespacesFlag] = allNs

	iptMgr := iptm.NewIptablesManager()
	if err := iptMgr.Save(util.IptablesTestConfigFile); err != nil {
		t.Errorf("TestDeleteNetworkPolicy failed @ iptMgr.Save")
	}

	ipsMgr := ipsm.NewIpsetManager()
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestDeleteNetworkPolicy failed @ ipsMgr.Save")
	}

	defer func() {
		if err := iptMgr.Restore(util.IptablesTestConfigFile); err != nil {
			t.Errorf("TestDeleteNetworkPolicy failed @ iptMgr.Restore")
		}

		if err := ipsMgr.Restore(util.IpsetTestConfigFile); err != nil {
			t.Errorf("TestDeleteNetworkPolicy failed @ ipsMgr.Restore")
		}
	}()

	nsObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-nwpolicy",
			Labels: map[string]string{
				"app": "test-namespace",
			},
		},
	}

	if err := npMgr.AddNamespace(nsObj); err != nil {
		t.Errorf("TestDeleteNetworkPolicy @ npMgr.AddNamespace")
	}

	tcp := corev1.ProtocolTCP
	allow := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "allow-ingress",
			Namespace: "test-nwpolicy",
		},
		Spec: networkingv1.NetworkPolicySpec{
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				networkingv1.NetworkPolicyIngressRule{
					From: []networkingv1.NetworkPolicyPeer{{
						PodSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "test"},
						},
					}},
					Ports: []networkingv1.NetworkPolicyPort{{
						Protocol: &tcp,
						Port: &intstr.IntOrString{
							StrVal: "8000",
						},
					}},
				},
			},
		},
	}

	if err := npMgr.AddNetworkPolicy(allow); err != nil {
		t.Errorf("TestAddNetworkPolicy failed @ AddNetworkPolicy")
	}

	if err := npMgr.DeleteNetworkPolicy(allow); err != nil {
		t.Errorf("TestAddNetworkPolicy failed @ DeleteNetworkPolicy")
	}
}
