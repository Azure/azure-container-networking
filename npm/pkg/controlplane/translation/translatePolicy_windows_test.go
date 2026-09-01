//go:build windows
// +build windows

package translation

import (
	"testing"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// These tests exercise the NPM Lite Windows direct-rule path (util.IsWindowsDP() is true when
// GOOS=windows), which the CI Windows job runs natively. They guard against MSRC 132306: an
// IPBlock.Except CIDR must be denied by a higher-priority Block ACL rather than silently
// dropped, which previously allowed the full enclosing CIDR.

func TestNPMLiteWindowsIngressExceptEmitsHigherPriorityDrop(t *testing.T) {
	networkPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "victim-ingress", Namespace: "victim"},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "victim"}},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress: []networkingv1.NetworkPolicyIngressRule{{
				From: []networkingv1.NetworkPolicyPeer{{
					IPBlock: &networkingv1.IPBlock{
						CIDR:   "10.244.1.0/24",
						Except: []string{"10.244.1.106/32"},
					},
				}},
			}},
		},
	}

	translated, err := TranslatePolicy(networkPolicy, true)
	require.NoError(t, err)
	require.Len(t, translated.ACLs, 2)

	allow := translated.ACLs[0]
	require.Equal(t, policies.Allowed, allow.Target)
	require.Equal(t, []string{"10.244.1.0/24"}, allow.SrcDirectIPs)
	require.Zero(t, allow.Priority)

	drop := translated.ACLs[1]
	require.Equal(t, policies.Dropped, drop.Target)
	require.Equal(t, []string{"10.244.1.106/32"}, drop.SrcDirectIPs)
	require.Equal(t, policies.ExceptBlockPriority, drop.Priority)
	// the excepted-drop must out-prioritize the enclosing-CIDR allow (lower number wins on HNS)
	require.Less(t, drop.Priority, uint16(222))
}

func TestNPMLiteWindowsEgressExceptEmitsHigherPriorityDrop(t *testing.T) {
	networkPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "victim-egress", Namespace: "victim"},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "victim"}},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
			Egress: []networkingv1.NetworkPolicyEgressRule{{
				To: []networkingv1.NetworkPolicyPeer{{
					IPBlock: &networkingv1.IPBlock{
						CIDR:   "192.0.2.0/24",
						Except: []string{"192.0.2.5/32"},
					},
				}},
			}},
		},
	}

	translated, err := TranslatePolicy(networkPolicy, true)
	require.NoError(t, err)
	require.Len(t, translated.ACLs, 2)

	allow := translated.ACLs[0]
	require.Equal(t, policies.Allowed, allow.Target)
	require.Equal(t, []string{"192.0.2.0/24"}, allow.DstDirectIPs)

	drop := translated.ACLs[1]
	require.Equal(t, policies.Dropped, drop.Target)
	require.Equal(t, []string{"192.0.2.5/32"}, drop.DstDirectIPs)
	require.Equal(t, policies.ExceptBlockPriority, drop.Priority)
}

func TestNPMLiteWindowsExceptWithPortMirrorsPortScope(t *testing.T) {
	port := intstr.FromInt(8080)
	tcp := v1.ProtocolTCP
	networkPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "victim-port", Namespace: "victim"},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "victim"}},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress: []networkingv1.NetworkPolicyIngressRule{{
				Ports: []networkingv1.NetworkPolicyPort{{
					Protocol: &tcp,
					Port:     &port,
				}},
				From: []networkingv1.NetworkPolicyPeer{{
					IPBlock: &networkingv1.IPBlock{
						CIDR:   "10.244.1.0/24",
						Except: []string{"10.244.1.106/32"},
					},
				}},
			}},
		},
	}

	translated, err := TranslatePolicy(networkPolicy, true)
	require.NoError(t, err)
	require.Len(t, translated.ACLs, 2)

	allow := translated.ACLs[0]
	require.Equal(t, policies.Allowed, allow.Target)
	require.Equal(t, int32(8080), allow.DstPorts.Port)
	require.Equal(t, policies.Protocol("TCP"), allow.Protocol)

	drop := translated.ACLs[1]
	require.Equal(t, policies.Dropped, drop.Target)
	require.Equal(t, []string{"10.244.1.106/32"}, drop.SrcDirectIPs)
	require.Equal(t, int32(8080), drop.DstPorts.Port)
	require.Equal(t, policies.Protocol("TCP"), drop.Protocol)
	require.Equal(t, policies.ExceptBlockPriority, drop.Priority)
}

func TestNPMLiteWindowsMultipleExceptsDeduplicated(t *testing.T) {
	networkPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "victim-multi", Namespace: "victim"},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "victim"}},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress: []networkingv1.NetworkPolicyIngressRule{{
				From: []networkingv1.NetworkPolicyPeer{{
					IPBlock: &networkingv1.IPBlock{
						CIDR:   "10.244.1.0/24",
						Except: []string{"10.244.1.106/32", "10.244.1.106/32", "10.244.1.200/32"},
					},
				}},
			}},
		},
	}

	translated, err := TranslatePolicy(networkPolicy, true)
	require.NoError(t, err)
	// 1 allow + 2 unique drops
	require.Len(t, translated.ACLs, 3)
	require.Equal(t, policies.Allowed, translated.ACLs[0].Target)
	require.Equal(t, policies.Dropped, translated.ACLs[1].Target)
	require.Equal(t, []string{"10.244.1.106/32"}, translated.ACLs[1].SrcDirectIPs)
	require.Equal(t, policies.Dropped, translated.ACLs[2].Target)
	require.Equal(t, []string{"10.244.1.200/32"}, translated.ACLs[2].SrcDirectIPs)
}

func TestNPMLiteWindowsNonIPv4ExceptFailsClosed(t *testing.T) {
	networkPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "victim-bad", Namespace: "victim"},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "victim"}},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress: []networkingv1.NetworkPolicyIngressRule{{
				From: []networkingv1.NetworkPolicyPeer{{
					IPBlock: &networkingv1.IPBlock{
						CIDR:   "10.244.1.0/24",
						Except: []string{"fd00::/64"},
					},
				}},
			}},
		},
	}

	_, err := TranslatePolicy(networkPolicy, true)
	require.ErrorIs(t, err, ErrUnsupportedIPAddress)
}
