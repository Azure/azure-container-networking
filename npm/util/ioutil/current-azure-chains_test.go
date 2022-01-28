package ioutil

import (
	"testing"

	"github.com/Azure/azure-container-networking/common"
	testutils "github.com/Azure/azure-container-networking/test/utils"
	"github.com/stretchr/testify/require"
)

const grepOutputAzureChainsWithPolicies = `Chain AZURE-NPM (1 references)
Chain AZURE-NPM-ACCEPT (1 references)
Chain AZURE-NPM-EGRESS (1 references)
Chain AZURE-NPM-EGRESS-123456 (1 references)
Chain AZURE-NPM-INGRESS (1 references)
Chain AZURE-NPM-INGRESS-123456 (1 references)
Chain AZURE-NPM-INGRESS-ALLOW-MARK (1 references)
`

var listAllCommandStrings = []string{"iptables", "-w", "60", "-t", "filter", "-n", "-L"}

func TestAllCurrentAzureChains(t *testing.T) {
	tests := []struct {
		name           string
		calls          []testutils.TestCmd
		expectedChains []string
		wantErr        bool
	}{
		{
			name: "success with chains",
			calls: []testutils.TestCmd{
				{Cmd: listAllCommandStrings, PipedToCommand: true},
				{
					Cmd:    []string{"grep", "Chain AZURE-NPM"},
					Stdout: grepOutputAzureChainsWithPolicies,
				},
			},
			expectedChains: []string{"AZURE-NPM", "AZURE-NPM-ACCEPT", "AZURE-NPM-EGRESS", "AZURE-NPM-EGRESS-123456", "AZURE-NPM-INGRESS", "AZURE-NPM-INGRESS-123456", "AZURE-NPM-INGRESS-ALLOW-MARK"},
			wantErr:        false,
		},
		{
			name: "ignore missing newline at end of grep result",
			calls: []testutils.TestCmd{
				{Cmd: []string{"iptables", "-w", "60", "-t", "filter", "-n", "-L"}, PipedToCommand: true},
				{
					Cmd: []string{"grep", "Chain AZURE-NPM"},
					Stdout: `Chain AZURE-NPM (1 references)
Chain AZURE-NPM-INGRESS (1 references)`,
				},
			},
			expectedChains: []string{"AZURE-NPM", "AZURE-NPM-INGRESS"},
			wantErr:        false,
		},
		{
			name: "ignore unexpected grep line (chain name too short)",
			calls: []testutils.TestCmd{
				{Cmd: []string{"iptables", "-w", "60", "-t", "filter", "-n", "-L"}, PipedToCommand: true},
				{
					Cmd: []string{"grep", "Chain AZURE-NPM"},
					Stdout: `Chain AZURE-NPM (1 references)
Chain abc (1 references)
Chain AZURE-NPM-INGRESS (1 references)
`,
				},
			},
			expectedChains: []string{"AZURE-NPM", "AZURE-NPM-INGRESS"},
			wantErr:        false,
		},
		{
			name: "ignore unexpected grep line (no space)",
			calls: []testutils.TestCmd{
				{Cmd: []string{"iptables", "-w", "60", "-t", "filter", "-n", "-L"}, PipedToCommand: true},
				{
					Cmd: []string{"grep", "Chain AZURE-NPM"},
					Stdout: `Chain AZURE-NPM (1 references)
abc
Chain AZURE-NPM-INGRESS (1 references)
`,
				},
			},
			expectedChains: []string{"AZURE-NPM", "AZURE-NPM-INGRESS"},
		},
		{
			name: "success with no chains",
			calls: []testutils.TestCmd{
				{Cmd: []string{"iptables", "-w", "60", "-t", "filter", "-n", "-L"}, PipedToCommand: true},
				{Cmd: []string{"grep", "Chain AZURE-NPM"}, ExitCode: 1},
			},
			expectedChains: nil,
			wantErr:        false,
		},
		{
			name: "grep failure",
			calls: []testutils.TestCmd{
				{Cmd: []string{"iptables", "-w", "60", "-t", "filter", "-n", "-L"}, PipedToCommand: true, HasStartError: true, ExitCode: 1},
				{Cmd: []string{"grep", "Chain AZURE-NPM"}},
			},
			expectedChains: nil,
			wantErr:        true,
		},
		{
			name: "invalid grep result",
			calls: []testutils.TestCmd{
				{Cmd: []string{"iptables", "-w", "60", "-t", "filter", "-n", "-L"}, PipedToCommand: true},
				{
					Cmd:    []string{"grep", "Chain AZURE-NPM"},
					Stdout: "",
				},
			},
			expectedChains: nil,
			wantErr:        true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ioshim := common.NewMockIOShim(tt.calls)
			defer ioshim.VerifyCalls(t, tt.calls)
			chains, err := AllCurrentAzureChains(ioshim.Exec, "60")
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, stringsToMap(tt.expectedChains), chains)
		})
	}
}

func stringsToMap(items []string) map[string]struct{} {
	if items == nil {
		return nil
	}
	m := make(map[string]struct{})
	for _, s := range items {
		m[s] = struct{}{}
	}
	return m
}
