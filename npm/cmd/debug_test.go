package main

import (
	"bytes"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	iptableSaveFile = "../pkg/dataplane/testdata/iptablesave"
	npmCacheFile    = "../pkg/dataplane/testdata/npmcache.json"
	nonExistingFile = "non-existing-iptables-file"

	npmCacheFlag         = "-c"
	iptablesSaveFileFlag = "-i"
	dstFlag              = "-d"
	srcFlag              = "-s"
	unknownShorthandFlag = "-z"

	testIP1 = "10.240.0.17" // from npmCacheWithCustomFormat.json
	testIP2 = "10.240.0.68" // ditto

	debugCmdString          = "debug"
	convertIPTableCmdString = "convertiptable"
	getTuplesCmdString      = "gettuples"
	parseIPTableCmdString   = "parseiptable"
)

type testCases struct {
	name    string
	args    []string
	wantErr bool
}

func testCommand(t *testing.T, tests []*testCases) {
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			rootCMD := NewRootCmd()
			b := bytes.NewBufferString("")
			rootCMD.SetOut(b)
			rootCMD.SetArgs(tt.args)
			err := rootCMD.Execute()

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			out, err := ioutil.ReadAll(b)
			require.NoError(t, err)
			if tt.wantErr {
				require.NotEmpty(t, out)
			} else {
				require.Empty(t, out)
			}
		})
	}
}

func concatArgs(baseArgs []string, args ...string) []string {
	return append(baseArgs, args...)
}
