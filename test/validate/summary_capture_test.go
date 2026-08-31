package validate

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	testCapturedIP1 = "10.0.0.2"
	testCapturedIP2 = "10.0.0.3"
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
				testCapturedIP2: "namespace/pod-b",
				testCapturedIP1: "namespace/pod-a",
			},
			actualIPs: []string{testCapturedIP2, testCapturedIP1},
		},
		{
			name:      "rejects malformed state IP",
			state:     map[string]string{"bad": "namespace/pod"},
			actualIPs: []string{testCapturedIP1},
			wantErr:   "invalid state IP",
		},
		{
			name:      "rejects unowned live IP",
			state:     map[string]string{testCapturedIP1: "namespace/pod"},
			actualIPs: []string{testCapturedIP2},
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
				{PodID: "namespace/pod-a", IP: netip.MustParseAddr(testCapturedIP1)},
				{PodID: "namespace/pod-b", IP: netip.MustParseAddr(testCapturedIP2)},
			}, summary.Expected)
			require.Equal(t, summary.Expected, summary.Actual)
		})
	}
}

func TestEmptyStateValidationMode(t *testing.T) {
	tests := []struct {
		name        string
		restartCase bool
		wantSkip    bool
		wantErr     bool
	}{
		{
			name:        "existing restart case skips transient empty state",
			restartCase: true,
			wantSkip:    true,
		},
		{
			name:    "strict R22 mode rejects empty state with live pods",
			wantErr: true,
		},
		{
			name:    "normal non-restart validation rejects empty state with live pods",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skip := skipEmptyStateForRestart(0, tt.restartCase)
			require.Equal(t, tt.wantSkip, skip)
			if skip {
				require.False(t, tt.wantErr)
				return
			}
			err := validateLivePodsHaveState(0, 1)
			if tt.wantErr {
				require.ErrorContains(t, err, "state is empty with 1 live pod IPs")
				return
			}
			require.NoError(t, err)
		})
	}
}
