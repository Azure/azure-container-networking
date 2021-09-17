package main

import "testing"

/* out
Parse iptable into Go object, dumping it to the console

Usage:
        azure-npm debug parseiptable [flags]

        Flags:
          -h, --help                   help for parseiptable
          -i, --iptables-file string   Set the iptable-save file path (optional)
*/

func TestParseIPTableCmd(t *testing.T) {
	tests := []*testCases{
		{
			name:    "unknown shorthand flag",
			args:    []string{debugCmdString, parseIPTableCmdString, unknownShorthandFlag},
			wantErr: true,
		},
		{
			name:    "unknown shorthand flag with a correct file",
			args:    []string{debugCmdString, parseIPTableCmdString, unknownShorthandFlag, iptableSaveFile},
			wantErr: true,
		},
		{
			name:    "non-existing iptables file",
			args:    []string{debugCmdString, parseIPTableCmdString, iptablesSaveFileFlag, nonExistingFile},
			wantErr: true,
		},
		{
			name:    "correct iptables file",
			args:    []string{debugCmdString, parseIPTableCmdString, iptablesSaveFileFlag, iptableSaveFile},
			wantErr: false,
		},
		{
			name:    "correct iptables file with shorthand flag first",
			args:    []string{debugCmdString, iptablesSaveFileFlag, iptableSaveFile, parseIPTableCmdString},
			wantErr: false,
		},
		{
			name:    "Iptables information from Kernel",
			args:    []string{debugCmdString, parseIPTableCmdString},
			wantErr: false,
		},
	}

	testCommand(t, tests)
}
