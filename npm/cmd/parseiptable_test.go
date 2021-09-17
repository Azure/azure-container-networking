package main

import "testing"

/* out
Usage:
        azure-npm debug parseiptable [flags]

        Flags:
          -h, --help                   help for parseiptable
          -i, --iptables-file string   Set the iptable-save file path (optional)
*/

func TestParseIPTableCmd(t *testing.T) {
	tests := []*testCases{
		{
			name:            "unknown shorthand flag",
			args:            []string{debugCmdString, parseIPTableCmdString, unknownShorthandFlag},
			wantErr:         true,
			wantEmptyOutput: false,
		},
		{
			name:            "unknown shorthand flag with a correct file",
			args:            []string{debugCmdString, parseIPTableCmdString, unknownShorthandFlag, iptableSaveFile},
			wantErr:         true,
			wantEmptyOutput: false,
		},
		{
			name:            "non-existing iptables file",
			args:            []string{debugCmdString, parseIPTableCmdString, iptablesSaveFileFlag, nonExistingFile},
			wantErr:         true,
			wantEmptyOutput: false,
		},
		{
			name:            "correct iptables file",
			args:            []string{debugCmdString, parseIPTableCmdString, iptablesSaveFileFlag, iptableSaveFile},
			wantErr:         false,
			wantEmptyOutput: true,
		},
		{
			name:            "correct iptables file with shorthand flag first",
			args:            []string{debugCmdString, iptablesSaveFileFlag, iptableSaveFile, parseIPTableCmdString},
			wantErr:         false,
			wantEmptyOutput: true,
		},
		{
			name:            "Iptables information from Kernel",
			args:            []string{debugCmdString, parseIPTableCmdString},
			wantErr:         false,
			wantEmptyOutput: true,
		},
	}

	testCommand(t, tests)
}
