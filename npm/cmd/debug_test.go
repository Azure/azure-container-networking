package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	iptableSaveFile = "../pkg/dataplane/testfiles/iptablesave"
	npmCacheFile    = "../pkg/dataplane/testfiles/npmCacheWithCustomFormat.json"
	nonExistingFile = "non-existing-iptables-file"

	npmCacheFlag         = "-c"
	iptablesSaveFileFlag = "-i"
	unknownShorthandFlag = "-z"

	debugCmdString          = "debug"
	convertIPTableCmdString = "convertiptable"
	getTuplesCmdString      = "gettuples"
	parseIPTableCmdString   = "parseiptable"
)

type testCases struct {
	name            string
	args            []string
	wantErr         bool
	wantEmptyOutput bool
}

func testCommand(t *testing.T, tests []*testCases) {
	rootCMD := rootCmd
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			b := bytes.NewBufferString("")
			rootCMD.SetOut(b)
			rootCMD.SetArgs(tt.args)
			fmt.Println(tt.args)
			err := rootCMD.Execute()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			out, err := ioutil.ReadAll(b)
			require.NoError(t, err)
			if tt.wantEmptyOutput {
				fmt.Println("stuff: ")
				fmt.Println(string(out))
				require.Empty(t, out)
			} else {
				require.NotEmpty(t, out)
			}
		})
	}
}
