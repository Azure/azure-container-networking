package translation

import (
	"testing"

	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
)

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
