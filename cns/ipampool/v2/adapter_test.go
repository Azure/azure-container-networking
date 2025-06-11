package v2

import (
	"testing"

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
