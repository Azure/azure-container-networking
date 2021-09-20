package main

import "testing"

// (TODO) test case where HTTP request made for NPM cache
func TestConvertIPTableCmd(t *testing.T) {
	baseArgs := []string{debugCmdString, convertIPTableCmdString}

	tests := []*testCases{
		{
			name:    "unknown shorthand flag",
			args:    concatArgs(baseArgs, unknownShorthandFlag),
			wantErr: true,
		},
		{
			name:    "unknown shorthand flag with correct files",
			args:    concatArgs(baseArgs, unknownShorthandFlag, iptableSaveFile, npmCacheFlag, npmCacheFile),
			wantErr: true,
		},
		{
			name:    "iptables save file but no cache file",
			args:    concatArgs(baseArgs, iptablesSaveFileFlag, iptableSaveFile),
			wantErr: true,
		},
		{
			name:    "iptables save file but bad cache file",
			args:    concatArgs(baseArgs, iptablesSaveFileFlag, iptableSaveFile, npmCacheFlag, nonExistingFile),
			wantErr: true,
		},
		{
			name:    "cache file but no iptables save file",
			args:    concatArgs(baseArgs, npmCacheFlag, npmCacheFile),
			wantErr: true,
		},
		{
			name:    "cache file but bad iptables save file",
			args:    concatArgs(baseArgs, iptablesSaveFileFlag, nonExistingFile, npmCacheFlag, npmCacheFile),
			wantErr: true,
		},
		{
			name:    "correct files",
			args:    concatArgs(baseArgs, iptablesSaveFileFlag, iptableSaveFile, npmCacheFlag, npmCacheFile),
			wantErr: false,
		},
		{
			name:    "correct files with file order switched",
			args:    concatArgs(baseArgs, npmCacheFlag, npmCacheFile, iptablesSaveFileFlag, iptableSaveFile),
			wantErr: false,
		},
		{
			name:    "correct files with shorthand flags first",
			args:    []string{debugCmdString, iptablesSaveFileFlag, iptableSaveFile, npmCacheFlag, npmCacheFile, convertIPTableCmdString},
			wantErr: false,
		},
	}

	testCommand(t, tests)
}
