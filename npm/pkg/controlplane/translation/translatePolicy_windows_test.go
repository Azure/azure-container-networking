package translation

import (
	"testing"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTranslatePolicyNPMLiteUsesDirectIPs(t *testing.T) {
	networkPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "allow-cidrs",
			Namespace: "victim",
		},
		Spec: networkingv1.NetworkPolicySpec{
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
	require.Empty(t, translated.RuleIPSets)
	require.Len(t, translated.ACLs, 4)
	require.Equal(t, policies.Allowed, translated.ACLs[0].Target)
	require.Equal(t, []string{"10.42.1.0/24"}, translated.ACLs[0].SrcDirectIPs)
	require.Empty(t, translated.ACLs[0].SrcList)
	require.Equal(t, policies.Allowed, translated.ACLs[2].Target)
	require.Equal(t, []string{"192.0.2.0/24"}, translated.ACLs[2].DstDirectIPs)
	require.Empty(t, translated.ACLs[2].DstList)
}

func TestTranslatePolicyNPMLiteRejectsExceptCIDR(t *testing.T) {
	networkPolicy := &networkingv1.NetworkPolicy{
		Spec: networkingv1.NetworkPolicySpec{
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress: []networkingv1.NetworkPolicyIngressRule{{
				From: []networkingv1.NetworkPolicyPeer{{
					IPBlock: &networkingv1.IPBlock{
						CIDR:   "10.42.0.0/16",
						Except: []string{"10.42.2.0/24"},
					},
				}},
			}},
		},
	}

	_, err := TranslatePolicy(networkPolicy, true)
	require.ErrorIs(t, err, ErrUnsupportedExceptCIDR)
}
