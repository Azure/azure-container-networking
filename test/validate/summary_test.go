package validate

import (
	"errors"
	"net/netip"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const testStateBackend StateBackend = "test-backend"

func TestDecodeValidationSummary(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		backend StateBackend
		wantErr string
	}{
		{
			name:    "valid",
			raw:     `{"stateBackend":"test-backend","checks":[{"checkName":"state","nodeName":"node-1","livePodCount":1,"expected":[{"podID":"ns/pod/uid","ip":"10.0.0.2"}],"actual":[{"podID":"ns/pod/uid","ip":"10.0.0.2"}]}]}`,
			backend: testStateBackend,
		},
		{
			name:    "malformed JSON",
			raw:     `{`,
			backend: testStateBackend,
			wantErr: "decoding validation summary",
		},
		{
			name:    "unknown field",
			raw:     `{"stateBackend":"test-backend","checks":[],"unknown":true}`,
			backend: testStateBackend,
			wantErr: "unknown field",
		},
		{
			name:    "trailing JSON value",
			raw:     `{"stateBackend":"test-backend","checks":[]} {}`,
			backend: testStateBackend,
			wantErr: "multiple JSON values",
		},
		{
			name:    "invalid IP",
			raw:     `{"stateBackend":"test-backend","checks":[{"checkName":"state","nodeName":"node-1","livePodCount":1,"expected":[{"podID":"pod","ip":"bad"}],"actual":[]}]}`,
			backend: testStateBackend,
			wantErr: "ParseAddr",
		},
		{
			name:    "missing state backend",
			raw:     `{"checks":[]}`,
			backend: testStateBackend,
			wantErr: "stateBackend is missing",
		},
		{
			name:    "missing required checks",
			raw:     `{"stateBackend":"test-backend"}`,
			backend: testStateBackend,
			wantErr: "summary checks are missing",
		},
		{
			name:    "missing check name",
			raw:     `{"stateBackend":"test-backend","checks":[{"nodeName":"node-1","livePodCount":0,"expected":[],"actual":[]}]}`,
			backend: testStateBackend,
			wantErr: "checkName is missing",
		},
		{
			name:    "missing live pod count",
			raw:     `{"stateBackend":"test-backend","checks":[{"checkName":"state","nodeName":"node-1","expected":[],"actual":[]}]}`,
			backend: testStateBackend,
			wantErr: "livePodCount is missing",
		},
		{
			name:    "missing expected state",
			raw:     `{"stateBackend":"test-backend","checks":[{"checkName":"state","nodeName":"node-1","livePodCount":0,"actual":[]}]}`,
			backend: testStateBackend,
			wantErr: "expected state is missing",
		},
		{
			name:    "missing actual state",
			raw:     `{"stateBackend":"test-backend","checks":[{"checkName":"state","nodeName":"node-1","livePodCount":0,"expected":[]}]}`,
			backend: testStateBackend,
			wantErr: "actual state is missing",
		},
		{
			name:    "wrong backend",
			raw:     `{"stateBackend":"other","checks":[]}`,
			backend: testStateBackend,
			wantErr: "does not match",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeValidationSummary(strings.NewReader(tt.raw), tt.backend)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestValidateValidationSummary(t *testing.T) {
	valid := validValidationSummary()
	tests := []struct {
		name    string
		mutate  func(*ValidationSummary)
		backend StateBackend
		wantErr string
	}{
		{
			name:    "valid dual-stack state",
			backend: testStateBackend,
		},
		{
			name: "valid empty state without live pods",
			mutate: func(summary *ValidationSummary) {
				summary.Checks[0].LivePodCount = 0
				summary.Checks[0].Expected = []PodIPIdentity{}
				summary.Checks[0].Actual = []PodIPIdentity{}
			},
			backend: testStateBackend,
		},
		{
			name:    "empty expected backend",
			backend: "",
			wantErr: "expected backend is empty",
		},
		{
			name: "empty summary backend",
			mutate: func(summary *ValidationSummary) {
				summary.StateBackend = ""
			},
			backend: testStateBackend,
			wantErr: "summary backend is empty",
		},
		{
			name: "backend whitespace",
			mutate: func(summary *ValidationSummary) {
				summary.StateBackend = " test-backend"
			},
			backend: testStateBackend,
			wantErr: "surrounding whitespace",
		},
		{
			name: "wrong backend",
			mutate: func(summary *ValidationSummary) {
				summary.StateBackend = "other"
			},
			backend: testStateBackend,
			wantErr: "does not match",
		},
		{
			name: "missing checks",
			mutate: func(summary *ValidationSummary) {
				summary.Checks = nil
			},
			backend: testStateBackend,
			wantErr: "checks are missing",
		},
		{
			name: "empty checks",
			mutate: func(summary *ValidationSummary) {
				summary.Checks = []ValidationCheckSummary{}
			},
			backend: testStateBackend,
			wantErr: "has no checks",
		},
		{
			name: "empty check name",
			mutate: func(summary *ValidationSummary) {
				summary.Checks[0].CheckName = " "
			},
			backend: testStateBackend,
			wantErr: "surrounding whitespace",
		},
		{
			name: "empty node name",
			mutate: func(summary *ValidationSummary) {
				summary.Checks[0].NodeName = ""
			},
			backend: testStateBackend,
			wantErr: "node name is empty",
		},
		{
			name: "negative live pod count",
			mutate: func(summary *ValidationSummary) {
				summary.Checks[0].LivePodCount = -1
			},
			backend: testStateBackend,
			wantErr: "negative live pod count",
		},
		{
			name: "missing expected state",
			mutate: func(summary *ValidationSummary) {
				summary.Checks[0].Expected = nil
			},
			backend: testStateBackend,
			wantErr: "missing expected state",
		},
		{
			name: "missing actual state",
			mutate: func(summary *ValidationSummary) {
				summary.Checks[0].Actual = nil
			},
			backend: testStateBackend,
			wantErr: "missing actual state",
		},
		{
			name: "live pods with empty expected state",
			mutate: func(summary *ValidationSummary) {
				summary.Checks[0].Expected = []PodIPIdentity{}
				summary.Checks[0].Actual = []PodIPIdentity{}
			},
			backend: testStateBackend,
			wantErr: "live pods but empty expected state",
		},
		{
			name: "duplicate check",
			mutate: func(summary *ValidationSummary) {
				summary.Checks = append(summary.Checks, summary.Checks[0])
			},
			backend: testStateBackend,
			wantErr: "duplicate check",
		},
		{
			name: "expected and actual differ",
			mutate: func(summary *ValidationSummary) {
				summary.Checks[0].Actual[0].PodID = "other-pod"
			},
			backend: testStateBackend,
			wantErr: "pod/IP identity mismatch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := cloneValidationSummary(valid)
			if tt.mutate != nil {
				tt.mutate(&summary)
			}
			err := ValidateValidationSummary(summary, tt.backend)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestComparePodIPIdentities(t *testing.T) {
	pod1v4 := testIdentity("pod-1", "10.0.0.2")
	pod1v6 := testIdentity("pod-1", "fd00::2")
	tests := []struct {
		name     string
		expected []PodIPIdentity
		actual   []PodIPIdentity
		wantErr  string
	}{
		{
			name:     "exact reordered identities",
			expected: []PodIPIdentity{pod1v4, pod1v6},
			actual:   []PodIPIdentity{pod1v6, pod1v4},
		},
		{
			name:     "missing identity",
			expected: []PodIPIdentity{pod1v4, pod1v6},
			actual:   []PodIPIdentity{pod1v4},
			wantErr:  "missing=[pod-1/fd00::2]",
		},
		{
			name:     "unexpected identity",
			expected: []PodIPIdentity{pod1v4},
			actual:   []PodIPIdentity{pod1v4, pod1v6},
			wantErr:  "unexpected=[pod-1/fd00::2]",
		},
		{
			name:     "changed pod identity",
			expected: []PodIPIdentity{pod1v4},
			actual:   []PodIPIdentity{testIdentity("pod-2", "10.0.0.2")},
			wantErr:  "missing=[pod-1/10.0.0.2]",
		},
		{
			name:     "empty pod ID",
			expected: []PodIPIdentity{{IP: netip.MustParseAddr("10.0.0.2")}},
			actual:   []PodIPIdentity{},
			wantErr:  "pod ID is empty",
		},
		{
			name:     "invalid IP",
			expected: []PodIPIdentity{{PodID: "pod-1"}},
			actual:   []PodIPIdentity{},
			wantErr:  "invalid IP",
		},
		{
			name:     "duplicate identity",
			expected: []PodIPIdentity{pod1v4, pod1v4},
			actual:   []PodIPIdentity{},
			wantErr:  "duplicate identity",
		},
		{
			name:     "duplicate actual identity",
			expected: []PodIPIdentity{pod1v4},
			actual:   []PodIPIdentity{pod1v4, pod1v4},
			wantErr:  "invalid actual state: duplicate identity",
		},
		{
			name:     "duplicate IP across pods",
			expected: []PodIPIdentity{pod1v4, testIdentity("pod-2", "10.0.0.2")},
			actual:   []PodIPIdentity{},
			wantErr:  "duplicate IP",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ComparePodIPIdentities(tt.expected, tt.actual)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestCompareValidationSummaries(t *testing.T) {
	baseline := validValidationSummary()
	tests := []struct {
		name    string
		mutate  func(*ValidationSummary)
		wantErr string
	}{
		{
			name: "exact reordered summary",
			mutate: func(summary *ValidationSummary) {
				summary.Checks[0].Expected[0], summary.Checks[0].Expected[1] =
					summary.Checks[0].Expected[1], summary.Checks[0].Expected[0]
				summary.Checks[0].Actual[0], summary.Checks[0].Actual[1] =
					summary.Checks[0].Actual[1], summary.Checks[0].Actual[0]
			},
		},
		{
			name: "missing check",
			mutate: func(summary *ValidationSummary) {
				summary.Checks = summary.Checks[:0]
			},
			wantErr: "summary has no checks",
		},
		{
			name: "unexpected check",
			mutate: func(summary *ValidationSummary) {
				check := summary.Checks[0]
				check.CheckName = "other"
				summary.Checks = append(summary.Checks, check)
			},
			wantErr: "unexpected check",
		},
		{
			name: "reduced live pod count",
			mutate: func(summary *ValidationSummary) {
				summary.Checks[0].LivePodCount--
			},
			wantErr: "live pod count changed",
		},
		{
			name: "increased live pod count",
			mutate: func(summary *ValidationSummary) {
				summary.Checks[0].LivePodCount++
			},
			wantErr: "live pod count changed",
		},
		{
			name: "reduced identities",
			mutate: func(summary *ValidationSummary) {
				summary.Checks[0].Expected = summary.Checks[0].Expected[:1]
				summary.Checks[0].Actual = summary.Checks[0].Actual[:1]
			},
			wantErr: "expected state changed",
		},
		{
			name: "changed pod and IP identity",
			mutate: func(summary *ValidationSummary) {
				replacement := testIdentity("pod-2", "10.0.0.9")
				summary.Checks[0].Expected[0] = replacement
				summary.Checks[0].Actual[0] = replacement
			},
			wantErr: "expected state changed",
		},
		{
			name: "duplicate candidate check",
			mutate: func(summary *ValidationSummary) {
				summary.Checks = append(summary.Checks, summary.Checks[0])
			},
			wantErr: "duplicate check",
		},
		{
			name: "wrong candidate backend",
			mutate: func(summary *ValidationSummary) {
				summary.StateBackend = "other"
			},
			wantErr: "invalid candidate summary",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := cloneValidationSummary(baseline)
			if tt.mutate != nil {
				tt.mutate(&candidate)
			}
			err := CompareValidationSummaries(baseline, candidate, testStateBackend)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
			if strings.Contains(tt.wantErr, "state changed") || strings.Contains(tt.wantErr, "live pod count") {
				require.True(t, errors.Is(err, ErrSummaryRegression))
			}
		})
	}
}

func TestCompareValidationSummariesRejectsMissingCheck(t *testing.T) {
	baseline := validValidationSummary()
	second := baseline.Checks[0]
	second.CheckName = "second"
	baseline.Checks = append(baseline.Checks, second)

	candidate := cloneValidationSummary(baseline)
	candidate.Checks = candidate.Checks[:1]

	err := CompareValidationSummaries(baseline, candidate, testStateBackend)
	require.ErrorContains(t, err, "candidate is missing checks [second/node-1]")
	require.ErrorIs(t, err, ErrSummaryRegression)
}

func validValidationSummary() ValidationSummary {
	identities := []PodIPIdentity{
		testIdentity("ns/pod/uid", "10.0.0.2"),
		testIdentity("ns/pod/uid", "fd00::2"),
	}
	return ValidationSummary{
		StateBackend: testStateBackend,
		Checks: []ValidationCheckSummary{{
			CheckName:    "state",
			NodeName:     "node-1",
			LivePodCount: 1,
			Expected:     append([]PodIPIdentity(nil), identities...),
			Actual:       append([]PodIPIdentity(nil), identities...),
		}},
	}
}

func cloneValidationSummary(summary ValidationSummary) ValidationSummary {
	cloned := summary
	cloned.Checks = append([]ValidationCheckSummary(nil), summary.Checks...)
	for i := range cloned.Checks {
		cloned.Checks[i].Expected = append([]PodIPIdentity(nil), summary.Checks[i].Expected...)
		cloned.Checks[i].Actual = append([]PodIPIdentity(nil), summary.Checks[i].Actual...)
	}
	return cloned
}

func testIdentity(podID, ip string) PodIPIdentity {
	return PodIPIdentity{PodID: podID, IP: netip.MustParseAddr(ip)}
}
