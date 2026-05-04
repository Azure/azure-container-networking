package v2

import (
	"context"
	"testing"

	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodIPDemandListener(t *testing.T) {
	tests := []struct {
		name     string
		pods     []v1.Pod
		expected int
	}{
		{
			name:     "empty pod list",
			pods:     []v1.Pod{},
			expected: 0,
		},
		{
			name: "single running pod",
			pods: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod1"},
					Status:     v1.PodStatus{Phase: v1.PodRunning},
				},
			},
			expected: 1,
		},
		{
			name: "multiple running pods",
			pods: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod1"},
					Status:     v1.PodStatus{Phase: v1.PodRunning},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod2"},
					Status:     v1.PodStatus{Phase: v1.PodPending},
				},
			},
			expected: 2,
		},
		{
			name: "mix of running and terminal pods - should exclude terminal",
			pods: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod1"},
					Status:     v1.PodStatus{Phase: v1.PodRunning},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod2"},
					Status:     v1.PodStatus{Phase: v1.PodSucceeded},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod3"},
					Status:     v1.PodStatus{Phase: v1.PodFailed},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod4"},
					Status:     v1.PodStatus{Phase: v1.PodPending},
				},
			},
			expected: 2, // Only pod1 (Running) and pod4 (Pending) should be counted
		},
		{
			name: "only terminal pods - should count zero",
			pods: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod1"},
					Status:     v1.PodStatus{Phase: v1.PodSucceeded},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod2"},
					Status:     v1.PodStatus{Phase: v1.PodFailed},
				},
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := make(chan int, 1)
			listener := PodIPDemandListener(ch)

			listener(tt.pods)

			select {
			case result := <-ch:
				if result != tt.expected {
					t.Errorf("expected %d, got %d", tt.expected, result)
				}
			default:
				t.Error("expected value in channel")
			}
		})
	}
}

func TestAsV1ReturnsIPAMPoolMonitor(t *testing.T) {
	t.Parallel()
	mon := &Monitor{z: zap.NewNop()}
	sink := make(chan v1alpha.NodeNetworkConfig, 1)
	pm := mon.AsV1(sink)

	// AsV1 should return an adapter that satisfies the IPAMPoolMonitor interface
	assert.NotNil(t, pm)

	// GetStateSnapshot returns an empty snapshot (adapter always returns zero-value)
	snap := pm.GetStateSnapshot()
	assert.Equal(t, int64(0), snap.MinimumFreeIps)
	assert.Equal(t, int64(0), snap.MaximumFreeIps)
}

func TestAdapterUpdate(t *testing.T) {
	t.Parallel()
	mon := &Monitor{z: zap.NewNop()}
	sink := make(chan v1alpha.NodeNetworkConfig, 1)
	pm := mon.AsV1(sink)

	nnc := &v1alpha.NodeNetworkConfig{
		Spec: v1alpha.NodeNetworkConfigSpec{
			RequestedIPCount: 42,
		},
	}

	err := pm.Update(nnc)
	assert.NoError(t, err)

	// The NNC should have been sent to the sink channel
	select {
	case received := <-sink:
		assert.Equal(t, int64(42), received.Spec.RequestedIPCount)
	default:
		t.Fatal("expected NNC on sink channel")
	}
}

func TestWithLegacyMetricsObserver(t *testing.T) {
	t.Parallel()
	mon := NewMonitor(zap.NewNop(), &ipStateStoreMock{}, &nncClientMock{}, nil, nil, nil)

	// default observer should be a no-op that returns nil
	assert.NoError(t, mon.legacyMetricsObserver(context.Background()))

	// replace with a custom observer
	called := false
	mon.WithLegacyMetricsObserver(func(context.Context) error {
		called = true
		return nil
	})
	assert.NoError(t, mon.legacyMetricsObserver(context.Background()))
	assert.True(t, called)
}
