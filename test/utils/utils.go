package testingutils

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/utils/exec"

	fakeexec "k8s.io/utils/exec/testing"
)

type TestCmd struct {
	Cmd      []string
	Stderr   string
	StdOut   string
	ExitCode int
}

func GetFakeExecWithScripts(calls []TestCmd) (*fakeexec.FakeExec, *fakeexec.FakeCmd) {
	fexec := &fakeexec.FakeExec{ExactOrder: false, DisableScripts: false}

	fcmd := &fakeexec.FakeCmd{}

	for _, call := range calls {
		stderr := call.Stderr
		stdout := call.StdOut
		if call.Stderr != "" || call.ExitCode != 0 {
			err := &fakeexec.FakeExitError{Status: call.ExitCode}
			fcmd.CombinedOutputScript = append(fcmd.CombinedOutputScript, func() ([]byte, []byte, error) { return []byte(stdout), []byte(stderr), err })
		} else {
			fcmd.CombinedOutputScript = append(fcmd.CombinedOutputScript, func() ([]byte, []byte, error) { return []byte(stdout), nil, nil })
		}

		fcmd.StdoutPipeResponse = fakeexec.FakeStdIOPipeResponse{ReadCloser: io.NopCloser(strings.NewReader(stdout))}
		fcmd.StderrPipeResponse = fakeexec.FakeStdIOPipeResponse{ReadCloser: io.NopCloser(strings.NewReader(stderr))}
	}

	for range calls {
		fexec.CommandScript = append(fexec.CommandScript, func(cmd string, args ...string) exec.Cmd { return fakeexec.InitFakeCmd(fcmd, cmd, args...) })
	}

	return fexec, fcmd
}

func VerifyCallsMatch(t *testing.T, calls []TestCmd, fexec *fakeexec.FakeExec, fcmd *fakeexec.FakeCmd) {
	require.Equal(t, len(calls), len(fcmd.CombinedOutputLog))

	for i, call := range calls {
		require.Equalf(t, call.Cmd, fcmd.CombinedOutputLog[i], "Call [%d] doesn't match expected", i)
	}
}
