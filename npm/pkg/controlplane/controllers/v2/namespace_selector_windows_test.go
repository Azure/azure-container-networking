package controllers

import (
	"fmt"
	"testing"

	"github.com/Azure/azure-container-networking/npm/pkg/controlplane/translation"
	dpmocks "github.com/Azure/azure-container-networking/npm/pkg/dataplane/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestWindowsFullNPMNamespaceNegationIsNotSubmitted(t *testing.T) {
	for _, requirement := range []metav1.LabelSelectorRequirement{
		{Key: "tenant", Operator: metav1.LabelSelectorOpNotIn, Values: []string{"a"}},
		{Key: "tenant", Operator: metav1.LabelSelectorOpNotIn, Values: []string{"a", "b"}},
		{Key: "tenant", Operator: metav1.LabelSelectorOpDoesNotExist},
	} {
		for _, direction := range []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress} {
			for _, combined := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/%v/combined=%t", direction, requirement.Operator, requirement.Values, combined), func(t *testing.T) {
					peer := networkingv1.NetworkPolicyPeer{
						NamespaceSelector: &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{requirement}},
					}
					if combined {
						peer.PodSelector = &metav1.LabelSelector{MatchLabels: map[string]string{"app": "client"}}
					}
					policy := &networkingv1.NetworkPolicy{
						ObjectMeta: metav1.ObjectMeta{Name: "namespace-selector", Namespace: "test", ResourceVersion: "1"},
						Spec:       networkingv1.NetworkPolicySpec{PolicyTypes: []networkingv1.PolicyType{direction}},
					}
					if direction == networkingv1.PolicyTypeIngress {
						policy.Spec.Ingress = []networkingv1.NetworkPolicyIngressRule{{From: []networkingv1.NetworkPolicyPeer{peer}}}
					} else {
						policy.Spec.Egress = []networkingv1.NetworkPolicyEgressRule{{To: []networkingv1.NetworkPolicyPeer{peer}}}
					}
					translated, err := translation.TranslatePolicy(policy, false)
					require.ErrorIs(t, err, translation.ErrUnsupportedNegativeMatch)
					require.Nil(t, translated)

					ctrl := gomock.NewController(t)
					dp := dpmocks.NewMockGenericDataplane(ctrl)
					f := newNetPolQueueFixture(t, policy, dp, false)
					c := f.netPolController
					c.addNetworkPolicy(policy)
					require.True(t, c.processNextWorkItem())
					require.Empty(t, c.rawNpSpecMap)
					require.Zero(t, c.workqueue.Len())
					require.Zero(t, c.workqueue.NumRequeues(getKey(policy, t)))

					corrected := policy.DeepCopy()
					corrected.ResourceVersion = "2"
					positive := metav1.LabelSelectorRequirement{Key: "tenant", Operator: metav1.LabelSelectorOpExists}
					if direction == networkingv1.PolicyTypeIngress {
						corrected.Spec.Ingress[0].From[0].NamespaceSelector.MatchExpressions[0] = positive
					} else {
						corrected.Spec.Egress[0].To[0].NamespaceSelector.MatchExpressions[0] = positive
					}
					require.NoError(t, f.kubeInformer.Networking().V1().NetworkPolicies().Informer().GetIndexer().Update(corrected))
					dp.EXPECT().UpdatePolicy(gomock.Any()).Return(nil).Times(1)
					c.updateNetworkPolicy(policy, corrected)
					require.True(t, c.processNextWorkItem())
					require.Equal(t, &corrected.Spec, c.rawNpSpecMap[getKey(corrected, t)])
				})
			}
		}
	}
}
