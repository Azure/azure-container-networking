package translation

import (
	"testing"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestPolicyACLBudgetDirections(t *testing.T) {
	makePorts := func(count int) []networkingv1.NetworkPolicyPort {
		ports := make([]networkingv1.NetworkPolicyPort, count)
		for i := range ports {
			port := intstr.FromInt(i + 1)
			ports[i] = networkingv1.NetworkPolicyPort{Port: &port}
		}
		return ports
	}
	for _, dual := range []bool{false, true} {
		for _, direction := range []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress} {
			for _, over := range []bool{false, true} {
				name := string(direction)
				if dual {
					name += "-dual"
				}
				if over {
					name += "-over"
				}
				t.Run(name, func(t *testing.T) {
					portCount := maxACLsPerPolicy - 1
					policyTypes := []networkingv1.PolicyType{direction}
					if dual {
						portCount -= 2 // One allow and one drop for the other direction.
					}
					if over {
						portCount++
					}
					policy := &networkingv1.NetworkPolicy{
						ObjectMeta: metav1.ObjectMeta{Name: "boundary", Namespace: defaultNS},
						Spec: networkingv1.NetworkPolicySpec{
							PolicyTypes: policyTypes,
						},
					}
					if direction == networkingv1.PolicyTypeIngress {
						policy.Spec.Ingress = []networkingv1.NetworkPolicyIngressRule{{Ports: makePorts(portCount)}}
						if dual {
							policy.Spec.PolicyTypes = append(policy.Spec.PolicyTypes, networkingv1.PolicyTypeEgress)
							policy.Spec.Egress = []networkingv1.NetworkPolicyEgressRule{{Ports: makePorts(1)}}
						}
					} else {
						policy.Spec.Egress = []networkingv1.NetworkPolicyEgressRule{{Ports: makePorts(portCount)}}
						if dual {
							policy.Spec.PolicyTypes = append(policy.Spec.PolicyTypes, networkingv1.PolicyTypeIngress)
							policy.Spec.Ingress = []networkingv1.NetworkPolicyIngressRule{{Ports: makePorts(1)}}
						}
					}
					got, err := TranslatePolicy(policy, false)
					if over {
						require.ErrorIs(t, err, ErrTooManyACLs)
						require.Nil(t, got)
					} else {
						require.NoError(t, err)
						require.Len(t, got.ACLs, maxACLsPerPolicy)
					}
				})
			}
		}
	}
}

func TestDefaultDropDoesNotExceedACLBudget(t *testing.T) {
	for _, direction := range []policies.Direction{policies.Ingress, policies.Egress} {
		t.Run(string(direction), func(t *testing.T) {
			policy := policies.NewNPMNetworkPolicy("full", defaultNS)
			for i := 0; i < maxACLsPerPolicy; i++ {
				policy.ACLs = append(policy.ACLs, policies.NewACLPolicy(policies.Allowed, direction))
			}
			var err error
			if direction == policies.Ingress {
				err = ingressPolicy(policy, "full", nil, false)
			} else {
				err = egressPolicy(policy, "full", nil, false)
			}
			require.ErrorIs(t, err, ErrTooManyACLs)
			require.Len(t, policy.ACLs, maxACLsPerPolicy)
		})
	}
}
