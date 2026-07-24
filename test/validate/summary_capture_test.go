package validate

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildValidationCheckSummary(t *testing.T) {
	tests := []struct {
		name      string
		state     map[string]string
		actualIPs []string
		wantErr   string
	}{
		{
			name: "captures sorted exact identities",
			state: map[string]string{
				"10.0.0.3": "namespace/pod-b",
				"10.0.0.2": "namespace/pod-a",
			},
			actualIPs: []string{"10.0.0.3", "10.0.0.2"},
		},
		{
			name:      "rejects malformed state IP",
			state:     map[string]string{"bad": "namespace/pod"},
			actualIPs: []string{"10.0.0.2"},
			wantErr:   "invalid state IP",
		},
		{
			name:      "rejects unowned live IP",
			state:     map[string]string{"10.0.0.2": "namespace/pod"},
			actualIPs: []string{"10.0.0.3"},
			wantErr:   "has no validated state owner",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary, err := buildValidationCheckSummary("state", "node-1", tt.state, tt.actualIPs)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, 2, summary.LivePodCount)
			require.Equal(t, []PodIPIdentity{
				{PodID: "namespace/pod-a", IP: netip.MustParseAddr("10.0.0.2")},
				{PodID: "namespace/pod-b", IP: netip.MustParseAddr("10.0.0.3")},
			}, summary.Expected)
			require.Equal(t, summary.Expected, summary.Actual)
		})
	}
}
