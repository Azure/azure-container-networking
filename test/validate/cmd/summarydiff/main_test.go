package main

import (
	"bytes"
	"net/netip"
	"testing"

	statevalidate "github.com/Azure/azure-container-networking/test/validate"
	"github.com/stretchr/testify/require"
)

func TestParseOptions(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    options
		wantErr string
	}{
		{
			name: "all required flags",
			args: []string{
				"-baseline", "before.json",
				"-candidate", "after.json",
				"-expected-backend", "backend",
			},
			want: options{
				baselinePath:    "before.json",
				candidatePath:   "after.json",
				expectedBackend: "backend",
			},
		},
		{
			name:    "all required flags missing",
			wantErr: "[-baseline -candidate -expected-backend]",
		},
		{
			name:    "expected backend missing",
			args:    []string{"-baseline", "before.json", "-candidate", "after.json"},
			wantErr: "-expected-backend",
		},
		{
			name:    "positional argument",
			args:    []string{"extra", "-baseline", "before.json", "-candidate", "after.json", "-expected-backend", "backend"},
			wantErr: "unexpected positional arguments",
		},
		{
			name:    "unknown flag",
			args:    []string{"-unknown"},
			wantErr: "flag provided but not defined",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stderr bytes.Buffer
			got, err := parseOptions(tt.args, &stderr)
			if tt.wantErr == "" {
				require.NoError(t, err)
				require.Equal(t, tt.want, got)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestAggregate(t *testing.T) {
	summary := statevalidate.ValidationSummary{
		Checks: []statevalidate.ValidationCheckSummary{
			{
				LivePodCount: 2,
				Expected: []statevalidate.PodIPIdentity{
					{PodID: "pod-1", IP: netip.MustParseAddr("10.0.0.2")},
					{PodID: "pod-2", IP: netip.MustParseAddr("10.0.0.3")},
				},
				Actual: []statevalidate.PodIPIdentity{
					{PodID: "pod-1", IP: netip.MustParseAddr("10.0.0.2")},
					{PodID: "pod-2", IP: netip.MustParseAddr("10.0.0.3")},
				},
			},
			{
				LivePodCount: 1,
				Expected: []statevalidate.PodIPIdentity{
					{PodID: "pod-3", IP: netip.MustParseAddr("fd00::2")},
				},
				Actual: []statevalidate.PodIPIdentity{
					{PodID: "pod-3", IP: netip.MustParseAddr("fd00::2")},
				},
			},
		},
	}

	require.Equal(t, summaryStats{
		Checks:      2,
		LivePods:    3,
		ExpectedIPs: 3,
		ActualIPs:   3,
	}, aggregate(summary))
}
