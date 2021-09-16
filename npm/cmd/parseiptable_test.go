package main

import (
	"bytes"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/require"
)

/* out
Usage:
        azure-npm debug parseiptable [flags]

        Flags:
          -h, --help                   help for parseiptable
          -i, --iptables-file string   Set the iptable-save file path (optional)
*/
const (
	iptableSaveFile = "../pkg/dataplane/testfiles/iptablesave"
	parseIPTableCMD = "parseiptable"
	debugCMD        = "debug"
)

func TestParseIPTableCmd(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		wantErr         bool
		wantEmptyOutput bool
	}{
		{
			name:            "unknown shorthand flag",
			args:            []string{debugCMD, parseIPTableCMD, "-c"},
			wantErr:         true,
			wantEmptyOutput: false,
		},
		{
			name:            "unknown shorthand flag with a correct file",
			args:            []string{debugCMD, parseIPTableCMD, "-c", iptableSaveFile},
			wantErr:         true,
			wantEmptyOutput: false,
		},
		{
			name:            "non-existing iptables file",
			args:            []string{debugCMD, parseIPTableCMD, "-i", "non-existing-iptables-file"},
			wantErr:         true,
			wantEmptyOutput: false,
		},
		{
			name:            "correct iptables file",
			args:            []string{debugCMD, parseIPTableCMD, "-i", iptableSaveFile},
			wantErr:         false,
			wantEmptyOutput: true,
		},
		{
			name:            "correct iptables file with shorthand flag first",
			args:            []string{debugCMD, "-i", iptableSaveFile, parseIPTableCMD},
			wantErr:         false,
			wantEmptyOutput: true,
		},
		{
			name:            "Iptables iformation from Kernel",
			args:            []string{debugCMD, parseIPTableCMD},
			wantErr:         false,
			wantEmptyOutput: true,
		},
	}

	rootCMD := rootCmd
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			b := bytes.NewBufferString("")
			rootCMD.SetOut(b)
			rootCMD.SetArgs(tt.args)
			err := rootCMD.Execute()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			out, err := ioutil.ReadAll(b)
			require.NoError(t, err)
			if tt.wantEmptyOutput {
				require.Empty(t, out)
			} else {
				require.NotEmpty(t, out)
			}
		})
	}
}
