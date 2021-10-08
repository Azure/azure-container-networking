package main

import "testing"

func TestParseIPTableCmd(t *testing.T) {
	baseArgs := []string{debugCmdString, parseIPTableCmdString}

	tests := []*testCases{
		{
			name:    "unknown shorthand flag",
			args:    concatArgs(baseArgs, unknownShorthandFlag),
			wantErr: true,
		},
		{
			name:    "unknown shorthand flag with a correct file",
			args:    concatArgs(baseArgs, unknownShorthandFlag, iptableSaveFile),
			wantErr: true,
		},
		{
			name:    "non-existing iptables file",
			args:    concatArgs(baseArgs, iptablesSaveFileFlag, nonExistingFile),
			wantErr: true,
		},
		{
			name:    "correct iptables file",
			args:    concatArgs(baseArgs, iptablesSaveFileFlag, iptableSaveFile),
			wantErr: false,
		},
		{
			name:    "correct iptables file with shorthand flag first",
			args:    []string{debugCmdString, iptablesSaveFileFlag, iptableSaveFile, parseIPTableCmdString},
			wantErr: false,
		},
		{
			name:    "Iptables information from Kernel",
			args:    baseArgs,
			wantErr: false,
		},
	}

	testCommand(t, tests)
}
