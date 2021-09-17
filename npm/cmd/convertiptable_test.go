package main

import "testing"

/* out
Get list of iptable's rules in JSON format

Usage:
  azure-npm debug convertiptable [flags]

Flags:
  -c, --cache-file string      Set the NPM cache file path (optional)
  -h, --help                   help for convertiptable
  -i, --iptables-file string   Set the iptable-save file path (optional)
*/

func TestConvertIPTableCmd(t *testing.T) {
	tests := []*testCases{
		{
			name:            "unknown shorthand flag",
			args:            []string{debugCmdString, convertIPTableCmdString, unknownShorthandFlag},
			wantErr:         true,
			wantEmptyOutput: false,
		},
		{
			name:            "unknown shorthand flag with a correct file",
			args:            []string{debugCmdString, convertIPTableCmdString, unknownShorthandFlag, iptableSaveFile},
			wantErr:         true,
			wantEmptyOutput: false,
		},
		{
			name:            "iptables save file but no cache file",
			args:            []string{debugCmdString, convertIPTableCmdString, iptablesSaveFileFlag, iptableSaveFile},
			wantErr:         true,
			wantEmptyOutput: false,
		},
		{
			name:            "iptables save file but bad cache file",
			args:            []string{debugCmdString, convertIPTableCmdString, iptablesSaveFileFlag, iptableSaveFile, npmCacheFlag, nonExistingFile},
			wantErr:         true,
			wantEmptyOutput: false,
		},
		{
			name:            "cache file but no iptables save file",
			args:            []string{debugCmdString, convertIPTableCmdString, npmCacheFlag, npmCacheFile},
			wantErr:         false,
			wantEmptyOutput: true,
		},
		{
			name:            "cache file but bad iptables save file",
			args:            []string{debugCmdString, convertIPTableCmdString, iptablesSaveFileFlag, nonExistingFile, npmCacheFlag, npmCacheFile},
			wantErr:         true,
			wantEmptyOutput: false,
		},
		{
			name:            "correct files",
			args:            []string{debugCmdString, convertIPTableCmdString, iptablesSaveFileFlag, iptableSaveFile, npmCacheFlag, npmCacheFile},
			wantErr:         false,
			wantEmptyOutput: true,
		},
		{
			name:            "correct files with file order switched",
			args:            []string{debugCmdString, convertIPTableCmdString, npmCacheFlag, npmCacheFile, iptablesSaveFileFlag, iptableSaveFile},
			wantErr:         false,
			wantEmptyOutput: true,
		},
		{
			name:            "correct files with shorthand flags first",
			args:            []string{debugCmdString, iptablesSaveFileFlag, iptableSaveFile, npmCacheFlag, npmCacheFile, convertIPTableCmdString},
			wantErr:         false,
			wantEmptyOutput: true,
		},
		{
			name:            "Iptables information from Kernel",
			args:            []string{debugCmdString, convertIPTableCmdString},
			wantErr:         false,
			wantEmptyOutput: true,
		},
	}

	testCommand(t, tests)
}
