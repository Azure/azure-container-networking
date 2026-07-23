package main

import (
	"bytes"
	"net/netip"
	"os"
	"path/filepath"
	"testing"

	statevalidate "github.com/Azure/azure-container-networking/test/validate"
	"github.com/stretchr/testify/require"
)

func TestRun(t *testing.T) {
	const (
		baseline = `{
			"stateBackend": "json",
			"checks": [{
				"checkName": "endpoint-state",
				"nodeName": "node-1",
				"livePodCount": 1,
				"expected": [{"podID": "namespace/pod", "ip": "10.0.0.2"}],
				"actual": [{"podID": "namespace/pod", "ip": "10.0.0.2"}]
			}]
		}`
		regressed = `{
			"stateBackend": "json",
			"checks": [{
				"checkName": "endpoint-state",
				"nodeName": "node-1",
				"livePodCount": 1,
				"expected": [{"podID": "namespace/pod", "ip": "10.0.0.3"}],
				"actual": [{"podID": "namespace/pod", "ip": "10.0.0.3"}]
			}]
		}`
		wrongBackend = `{
			"stateBackend": "bolt",
			"checks": [{
				"checkName": "endpoint-state",
				"nodeName": "node-1",
				"livePodCount": 1,
				"expected": [{"podID": "namespace/pod", "ip": "10.0.0.2"}],
				"actual": [{"podID": "namespace/pod", "ip": "10.0.0.2"}]
			}]
		}`
	)

	tests := []struct {
		name       string
		candidate  string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{
			name:       "matching summaries",
			candidate:  baseline,
			wantCode:   0,
			wantStdout: `{"baseline":{"checks":1,"livePods":1,"expectedIPs":1,"actualIPs":1},"candidate":{"checks":1,"livePods":1,"expectedIPs":1,"actualIPs":1}}`,
		},
		{
			name:       "summary regression",
			candidate:  regressed,
			wantCode:   1,
			wantStdout: `{"baseline":{"checks":1,"livePods":1,"expectedIPs":1,"actualIPs":1},"candidate":{"checks":1,"livePods":1,"expectedIPs":1,"actualIPs":1}}`,
			wantStderr: "summary comparison failed: validation summary regression",
		},
		{
			name:       "wrong candidate backend",
			candidate:  wrongBackend,
			wantCode:   2,
			wantStderr: `reading candidate summary: decoding`,
		},
		{
			name:       "malformed candidate",
			candidate:  `{"stateBackend":`,
			wantCode:   2,
			wantStderr: `reading candidate summary: decoding`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()
			baselinePath := filepath.Join(tempDir, "baseline.json")
			candidatePath := filepath.Join(tempDir, "candidate.json")
			require.NoError(t, os.WriteFile(baselinePath, []byte(baseline), 0o600))
			require.NoError(t, os.WriteFile(candidatePath, []byte(tt.candidate), 0o600))

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			gotCode := run([]string{
				"-baseline", baselinePath,
				"-candidate", candidatePath,
				"-expected-backend", "json",
			}, &stdout, &stderr)

			require.Equal(t, tt.wantCode, gotCode)
			if tt.wantStdout == "" {
				require.Empty(t, stdout.String())
			} else {
				require.JSONEq(t, tt.wantStdout, stdout.String())
			}
			if tt.wantStderr == "" {
				require.Empty(t, stderr.String())
			} else {
				require.Contains(t, stderr.String(), tt.wantStderr)
			}
		})
	}
}

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
