package controllers

import (
	"fmt"
	"testing"

	"github.com/Azure/azure-container-networking/npm/pkg/controlplane/translation"
	dpmocks "github.com/Azure/azure-container-networking/npm/pkg/dataplane/mocks"
	"github.com/Azure/azure-container-networking/npm/util"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/workqueue"
)

const namespaceSelectorLabelKey = "tenant"

func newNetPolQueueFixture(t *testing.T, policy *networkingv1.NetworkPolicy, dp *dpmocks.MockGenericDataplane, npmLite bool) *netPolFixture {
	t.Helper()
	f := newNetPolFixture(t)
	f.netPolLister = append(f.netPolLister, policy)
	f.kubeobjects = append(f.kubeobjects, policy)
	f.newNetPolController(nil, dp, npmLite)
	f.netPolController.workqueue.ShutDown()
	// Zero-delay retries make incorrect requeueing observable without sleeps.
	f.netPolController.workqueue = workqueue.NewTypedRateLimitingQueue[any](workqueue.NewTypedItemFastSlowRateLimiter[any](0, 0, 1))
	t.Cleanup(f.netPolController.workqueue.ShutDown)
	return f
}

func TestFullNPMTranslationFailureWaitsForPolicyChange(t *testing.T) {
	oversized := netPolWithCIDR("192.0.2.0/24")
	selector := &metav1.LabelSelector{}
	for i := 0; i < 19; i++ {
		selector.MatchExpressions = append(selector.MatchExpressions, metav1.LabelSelectorRequirement{
			Key: fmt.Sprintf("key%d", i), Operator: metav1.LabelSelectorOpIn, Values: []string{"a", "b"},
		})
	}
	oversized.Spec.Ingress[0].From = []networkingv1.NetworkPolicyPeer{{NamespaceSelector: selector}}
	invalidWithExcept := netPolWithCIDR("192.0.2.0/33")
	invalidWithExcept.Spec.Ingress[0].From[0].IPBlock.Except = []string{"192.0.2.1/32"}
	unknownOperator := netPolWithCIDR("192.0.2.0/24")
	unknownOperator.Spec.Ingress[0].From = []networkingv1.NetworkPolicyPeer{{
		NamespaceSelector: &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{
			Key: namespaceSelectorLabelKey, Operator: "Unknown",
		}}},
	}}

	tests := []struct {
		name   string
		policy *networkingv1.NetworkPolicy
		cause  error
	}{
		{"malformed CIDR", netPolWithCIDR("192.0.2.0/33"), util.ErrInvalidCIDR},
		{"malformed CIDR with Except", invalidWithExcept, util.ErrInvalidCIDR},
		{"selector expansion", oversized, translation.ErrTooManyFlattenedSelectors},
		{"unsupported operator", unknownOperator, translation.ErrUnsupportedMatchExpressionOperator},
	}
	if !util.IsWindowsDP() {
		tests = append(tests, struct {
			name   string
			policy *networkingv1.NetworkPolicy
			cause  error
		}{"unsupported family", netPolWithCIDR("2001:db8::/32"), util.ErrUnsupportedIPFamily})
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.policy.ResourceVersion = "1"
			ctrl := gomock.NewController(t)
			dp := dpmocks.NewMockGenericDataplane(ctrl)
			f := newNetPolQueueFixture(t, test.policy, dp, false)
			c := f.netPolController
			key := getKey(test.policy, t)

			_, err := c.syncAddAndUpdateNetPol(test.policy)
			require.ErrorIs(t, err, test.cause)
			require.ErrorIs(t, err, errNetPolTranslationFailure)
			require.ErrorContains(t, err, key)
			require.Empty(t, c.rawNpSpecMap)

			c.addNetworkPolicy(test.policy)
			require.Equal(t, 1, c.workqueue.Len())
			require.True(t, c.processNextWorkItem())
			require.Zero(t, c.workqueue.NumRequeues(key))
			require.Zero(t, c.workqueue.Len())
			require.Empty(t, c.rawNpSpecMap)

			c.updateNetworkPolicy(test.policy, test.policy.DeepCopy())
			require.Zero(t, c.workqueue.Len(), "an informer resync must not repeat the rejection")

			corrected := test.policy.DeepCopy()
			corrected.ResourceVersion = "2"
			corrected.Spec = netPolWithCIDR("10.0.0.0/0").Spec
			require.NoError(t, f.kubeInformer.Networking().V1().NetworkPolicies().Informer().GetIndexer().Update(corrected))
			dp.EXPECT().UpdatePolicy(gomock.Any()).Return(nil).Times(1)
			c.updateNetworkPolicy(test.policy, corrected)
			require.Equal(t, 1, c.workqueue.Len())
			require.True(t, c.processNextWorkItem())
			require.Equal(t, &corrected.Spec, c.rawNpSpecMap[key])
			require.Zero(t, c.workqueue.NumRequeues(key))
			require.Zero(t, c.workqueue.Len())
		})
	}
}

func TestFullNPMDataplaneFailureStillRetries(t *testing.T) {
	policy := netPolWithCIDR("10.0.0.0/0")
	ctrl := gomock.NewController(t)
	dp := dpmocks.NewMockGenericDataplane(ctrl)
	f := newNetPolQueueFixture(t, policy, dp, false)
	c := f.netPolController
	key := getKey(policy, t)

	gomock.InOrder(
		dp.EXPECT().UpdatePolicy(gomock.Any()).Return(fmt.Errorf("programming policy: %w", translation.ErrUnsupportedIPAddress)),
		dp.EXPECT().UpdatePolicy(gomock.Any()).Return(nil),
	)
	c.addNetworkPolicy(policy)
	require.True(t, c.processNextWorkItem())
	require.Equal(t, 1, c.workqueue.NumRequeues(key))
	require.Equal(t, 1, c.workqueue.Len())
	require.Empty(t, c.rawNpSpecMap)

	require.True(t, c.processNextWorkItem())
	require.Zero(t, c.workqueue.NumRequeues(key))
	require.Zero(t, c.workqueue.Len())
	require.Equal(t, &policy.Spec, c.rawNpSpecMap[key])
}

func TestFullNPMRejectedUpdateRetainsAppliedPolicyUntilDeletion(t *testing.T) {
	policy := netPolWithCIDR("192.0.2.0/24")
	policy.ResourceVersion = "1"
	ctrl := gomock.NewController(t)
	dp := dpmocks.NewMockGenericDataplane(ctrl)
	f := newNetPolQueueFixture(t, policy, dp, false)
	c := f.netPolController
	key := getKey(policy, t)

	dp.EXPECT().UpdatePolicy(gomock.Any()).Return(nil).Times(1)
	c.addNetworkPolicy(policy)
	require.True(t, c.processNextWorkItem())
	require.Equal(t, &policy.Spec, c.rawNpSpecMap[key])

	rejected := policy.DeepCopy()
	rejected.ResourceVersion = "2"
	rejected.Spec.Ingress[0].From[0].IPBlock.CIDR = "192.0.2.0/33"
	indexer := f.kubeInformer.Networking().V1().NetworkPolicies().Informer().GetIndexer()
	require.NoError(t, indexer.Update(rejected))
	c.updateNetworkPolicy(policy, rejected)
	require.Equal(t, 1, c.workqueue.Len())
	require.True(t, c.processNextWorkItem())
	require.Equal(t, &policy.Spec, c.rawNpSpecMap[key])
	require.Zero(t, c.workqueue.NumRequeues(key))
	require.Zero(t, c.workqueue.Len())

	require.NoError(t, indexer.Delete(rejected))
	dp.EXPECT().RemovePolicy(key).Return(nil).Times(1)
	c.deleteNetworkPolicy(rejected)
	require.Equal(t, 1, c.workqueue.Len())
	require.True(t, c.processNextWorkItem())
	require.Empty(t, c.rawNpSpecMap)
	require.Zero(t, c.workqueue.Len())
}

func TestFullNPMRejectedCreateCanBeDeleted(t *testing.T) {
	policy := netPolWithCIDR("invalid")
	ctrl := gomock.NewController(t)
	dp := dpmocks.NewMockGenericDataplane(ctrl)
	f := newNetPolQueueFixture(t, policy, dp, false)
	c := f.netPolController

	c.addNetworkPolicy(policy)
	require.True(t, c.processNextWorkItem())
	require.Zero(t, c.workqueue.Len())
	require.Empty(t, c.rawNpSpecMap)

	require.NoError(t, f.kubeInformer.Networking().V1().NetworkPolicies().Informer().GetIndexer().Delete(policy))
	c.deleteNetworkPolicy(policy)
	require.Equal(t, 1, c.workqueue.Len())
	require.True(t, c.processNextWorkItem())
	require.Empty(t, c.rawNpSpecMap)
	require.Zero(t, c.workqueue.Len())
}

func TestLiteTranslationFailureHandlingIsUnchanged(t *testing.T) {
	policy := netPolWithCIDR("invalid")
	ctrl := gomock.NewController(t)
	dp := dpmocks.NewMockGenericDataplane(ctrl)
	f := newNetPolQueueFixture(t, policy, dp, true)
	c := f.netPolController
	key := getKey(policy, t)

	_, err := c.syncAddAndUpdateNetPol(policy)
	if util.IsWindowsDP() {
		require.NoError(t, err, "the legacy Lite direct-address limitation stays suppressed")
	} else {
		require.ErrorIs(t, err, util.ErrInvalidCIDR)
		require.NotErrorIs(t, err, errNetPolTranslationFailure)
	}

	c.addNetworkPolicy(policy)
	require.True(t, c.processNextWorkItem())
	require.Empty(t, c.rawNpSpecMap)
	if util.IsWindowsDP() {
		require.Zero(t, c.workqueue.NumRequeues(key))
		require.Zero(t, c.workqueue.Len())
	} else {
		require.Equal(t, 1, c.workqueue.NumRequeues(key))
		require.Equal(t, 1, c.workqueue.Len())
	}
}
