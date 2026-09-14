package translation

import (
	"testing"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
	"github.com/Azure/azure-container-networking/npm/util"
	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIPBlockNormalizationErrorCauses(t *testing.T) {
	for _, test := range []struct {
		name  string
		block networkingv1.IPBlock
		cause error
	}{
		{"invalid prefix", networkingv1.IPBlock{CIDR: "192.0.2.0/33"}, util.ErrInvalidCIDR},
		{"bare address", networkingv1.IPBlock{CIDR: "192.0.2.1"}, util.ErrInvalidCIDR},
		{"IPv6 prefix", networkingv1.IPBlock{CIDR: "2001:db8:2::/48"}, util.ErrUnsupportedIPFamily},
		{"mapped IPv6 prefix", networkingv1.IPBlock{CIDR: "::ffff:192.0.2.0/120"}, util.ErrUnsupportedIPFamily},
		{"invalid exclusion", networkingv1.IPBlock{CIDR: enclosingCIDR, Except: []string{"invalid"}}, util.ErrInvalidCIDR},
		{"IPv6 exclusion", networkingv1.IPBlock{CIDR: enclosingCIDR, Except: []string{"2001:db8:2::/48"}}, util.ErrUnsupportedIPFamily},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := test.block.DeepCopy()
			set, info, err := ipBlockRule("normalization", defaultNS, policies.Ingress, policies.SrcMatch, 0, 0, &test.block)
			require.Nil(t, set)
			require.Equal(t, policies.SetInfo{}, info)
			require.Equal(t, before, &test.block)

			if util.IsWindowsDP() && len(test.block.Except) > 0 {
				require.ErrorIs(t, err, ErrUnsupportedExceptCIDR)
			} else {
				require.ErrorIs(t, err, ErrUnsupportedIPAddress)
				require.ErrorIs(t, err, test.cause)
			}

			for _, direction := range []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress} {
				policy := &networkingv1.NetworkPolicy{
					ObjectMeta: metav1.ObjectMeta{Name: "normalization", Namespace: defaultNS},
					Spec: networkingv1.NetworkPolicySpec{
						PolicyTypes: []networkingv1.PolicyType{direction},
					},
				}
				peers := []networkingv1.NetworkPolicyPeer{{IPBlock: &test.block}}
				if direction == networkingv1.PolicyTypeIngress {
					policy.Spec.Ingress = []networkingv1.NetworkPolicyIngressRule{{From: peers}}
				} else {
					policy.Spec.Egress = []networkingv1.NetworkPolicyEgressRule{{To: peers}}
				}
				translated, policyErr := TranslatePolicy(policy, false)
				require.Nil(t, translated)
				if util.IsWindowsDP() && len(test.block.Except) > 0 {
					require.ErrorIs(t, policyErr, ErrUnsupportedExceptCIDR)
				} else {
					require.ErrorIs(t, policyErr, ErrUnsupportedIPAddress)
					require.ErrorIs(t, policyErr, test.cause)
				}
			}
		})
	}
}
