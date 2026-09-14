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
	const (
		ipv6CIDR      = "2001:db8:2::/48"
		malformedCIDR = "invalid"
	)
	for _, test := range []struct {
		name                 string
		block                networkingv1.IPBlock
		cause                error
		windowsExceptFailure bool
	}{
		{"invalid prefix", networkingv1.IPBlock{CIDR: "192.0.2.0/33"}, util.ErrInvalidCIDR, false},
		{"bare address", networkingv1.IPBlock{CIDR: "192.0.2.1"}, util.ErrInvalidCIDR, false},
		{"IPv6 prefix", networkingv1.IPBlock{CIDR: ipv6CIDR}, util.ErrUnsupportedIPFamily, false},
		{"mapped IPv6 prefix", networkingv1.IPBlock{CIDR: "::ffff:192.0.2.0/120"}, util.ErrUnsupportedIPFamily, false},
		{"invalid parent with exclusion", networkingv1.IPBlock{CIDR: "192.0.2.0/33", Except: []string{"192.0.2.1/32"}}, util.ErrInvalidCIDR, false},
		{"IPv6 parent with exclusion", networkingv1.IPBlock{CIDR: ipv6CIDR, Except: []string{"2001:db8:2::1/128"}}, util.ErrUnsupportedIPFamily, false},
		{"invalid parent and exclusion", networkingv1.IPBlock{CIDR: malformedCIDR, Except: []string{malformedCIDR}}, util.ErrInvalidCIDR, false},
		{"invalid exclusion", networkingv1.IPBlock{CIDR: enclosingCIDR, Except: []string{malformedCIDR}}, util.ErrInvalidCIDR, true},
		{"IPv6 exclusion", networkingv1.IPBlock{CIDR: enclosingCIDR, Except: []string{ipv6CIDR}}, util.ErrUnsupportedIPFamily, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			unsupportedExcept := util.IsWindowsDP() && test.windowsExceptFailure
			before := test.block.DeepCopy()
			set, info, err := ipBlockRule("normalization", defaultNS, policies.Ingress, policies.SrcMatch, 0, 0, &test.block)
			require.Nil(t, set)
			require.Equal(t, policies.SetInfo{}, info)
			require.Equal(t, before, &test.block)

			if unsupportedExcept {
				require.ErrorIs(t, err, ErrUnsupportedExceptCIDR)
			} else {
				require.ErrorIs(t, err, ErrUnsupportedIPAddress)
				require.ErrorIs(t, err, test.cause)
				require.NotErrorIs(t, err, ErrUnsupportedExceptCIDR)
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
				if unsupportedExcept {
					require.ErrorIs(t, policyErr, ErrUnsupportedExceptCIDR)
				} else {
					require.ErrorIs(t, policyErr, ErrUnsupportedIPAddress)
					require.ErrorIs(t, policyErr, test.cause)
				}
			}
		})
	}
}
