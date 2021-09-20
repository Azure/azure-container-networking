package main

import "testing"

// (TODO) test case where HTTP request made for NPM cache
func TestGetTuplesCmd(t *testing.T) {
	baseArgs := []string{debugCmdString, getTuplesCmdString}
	standardArgs := concatArgs(baseArgs, srcFlag, testIP1, dstFlag, testIP2)

	tests := []*testCases{
		{
			name:    "no src or dst",
			args:    baseArgs,
			wantErr: true,
		},
		{
			name:    "no src",
			args:    concatArgs(baseArgs, dstFlag, testIP2),
			wantErr: true,
		},
		{
			name:    "no dst",
			args:    concatArgs(baseArgs, srcFlag, testIP1),
			wantErr: true,
		},
		{
			name:    "unknown shorthand flag",
			args:    concatArgs(standardArgs, unknownShorthandFlag),
			wantErr: true,
		},
		{
			name:    "iptables save file but no cache file",
			args:    concatArgs(standardArgs, iptablesSaveFileFlag, iptableSaveFile),
			wantErr: true,
		},
		{
			name:    "iptables save file but bad cache file",
			args:    concatArgs(standardArgs, iptablesSaveFileFlag, iptableSaveFile, npmCacheFlag, nonExistingFile),
			wantErr: true,
		},
		{
			name:    "cache file but no iptables save file",
			args:    concatArgs(standardArgs, npmCacheFlag, npmCacheFile),
			wantErr: true,
		},
		{
			name:    "cache file but bad iptables save file",
			args:    concatArgs(standardArgs, iptablesSaveFileFlag, nonExistingFile, npmCacheFlag, npmCacheFile),
			wantErr: true,
		},
		{
			name:    "correct files",
			args:    concatArgs(standardArgs, iptablesSaveFileFlag, iptableSaveFile, npmCacheFlag, npmCacheFile),
			wantErr: false,
		},
		{
			name:    "correct files with file order switched",
			args:    concatArgs(standardArgs, npmCacheFlag, npmCacheFile, iptablesSaveFileFlag, iptableSaveFile),
			wantErr: false,
		},
		{
			name:    "src/dst after files",
			args:    concatArgs(baseArgs, npmCacheFlag, npmCacheFile, iptablesSaveFileFlag, iptableSaveFile, srcFlag, testIP1, dstFlag, testIP2),
			wantErr: false,
		},
		{
			name:    "shorthand flags before command",
			args:    []string{debugCmdString, srcFlag, testIP1, dstFlag, testIP2, iptablesSaveFileFlag, iptableSaveFile, npmCacheFlag, npmCacheFile, getTuplesCmdString},
			wantErr: false,
		},
	}

	testCommand(t, tests)
}
