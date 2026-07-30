package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIsScheduledWithDRA(t *testing.T) {
	tests := []struct {
		name   string
		claims []string
		want   bool
	}{
		{"nil list is non-DRA", nil, false},
		{"empty list is non-DRA", []string{}, false},
		{"single claim is DRA", []string{"claim-a"}, true},
		{"multiple claims are DRA", []string{"claim-a", "claim-b"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &MultitenantPodNetworkConfig{
				Spec: MultitenantPodNetworkConfigSpec{ResourceClaims: tt.claims},
			}
			require.Equal(t, tt.want, m.IsScheduledWithDRA())
		})
	}
}

func TestIsDeleting(t *testing.T) {
	m := &MultitenantPodNetworkConfig{}
	require.False(t, m.IsDeleting())

	now := metav1.Now()
	m.DeletionTimestamp = &now
	require.True(t, m.IsDeleting())
}
