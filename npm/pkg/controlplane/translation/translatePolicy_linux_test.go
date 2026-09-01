package translation

import (
	"testing"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTranslatePolicyNPMLiteUsesIPSets(t *testing.T) {
	networkPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "allow-cidrs",
			Namespace: "victim",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "victim"},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{{
				From: []networkingv1.NetworkPolicyPeer{{
					IPBlock: &networkingv1.IPBlock{CIDR: "10.42.1.0/24"},
				}},
			}},
			Egress: []networkingv1.NetworkPolicyEgressRule{{
				To: []networkingv1.NetworkPolicyPeer{{
					IPBlock: &networkingv1.IPBlock{CIDR: "192.0.2.0/24"},
				}},
			}},
		},
	}

	translated, err := TranslatePolicy(networkPolicy, true)
	require.NoError(t, err)
	require.Len(t, translated.RuleIPSets, 2)
	require.Equal(t, []string{"10.42.1.0/24"}, translated.RuleIPSets[0].Members)
	require.Equal(t, []string{"192.0.2.0/24"}, translated.RuleIPSets[1].Members)

	require.Len(t, translated.ACLs, 4)
	require.Equal(t, policies.Allowed, translated.ACLs[0].Target)
	require.Len(t, translated.ACLs[0].SrcList, 1)
	require.Empty(t, translated.ACLs[0].SrcDirectIPs)
	require.Equal(t, policies.Allowed, translated.ACLs[2].Target)
	require.Len(t, translated.ACLs[2].DstList, 1)
	require.Empty(t, translated.ACLs[2].DstDirectIPs)
}

// TestTranslatePolicyNPMLiteLinuxExceptUsesNomatch guards that, on Linux, NPM Lite keeps
// routing IPBlock.Except through the ipset "nomatch" path (never the Windows direct-drop path),
// so the fix for MSRC 132306 does not change Linux behavior.
func TestTranslatePolicyNPMLiteLinuxExceptUsesNomatch(t *testing.T) {
	networkPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "allow-cidr-except",
			Namespace: "victim",
		},
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
	require.Len(t, translated.RuleIPSets, 1)
	require.Equal(t, []string{"10.244.1.0/24", "10.244.1.106/32 nomatch"}, translated.RuleIPSets[0].Members)

	// No direct-IP ACLs on Linux; the CIDR+Except is enforced through the ipset above.
	for _, acl := range translated.ACLs {
		require.Empty(t, acl.SrcDirectIPs)
		require.Empty(t, acl.DstDirectIPs)
	}
}
