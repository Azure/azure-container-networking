package testingutils

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/utils/exec"

	fakeexec "k8s.io/utils/exec/testing"
)

type TestCmd struct {
	Cmd      []string
	Stderr   string
	ExitCode int
}

func GetFakeExecWithScripts(calls []TestCmd) (*fakeexec.FakeExec, *fakeexec.FakeCmd) {
	fexec := &fakeexec.FakeExec{ExactOrder: false, DisableScripts: false}

	fcmd := &fakeexec.FakeCmd{}

	for _, call := range calls {
		if call.Stderr != "" || call.ExitCode != 0 {
			stderr := call.Stderr
			err := &fakeexec.FakeExitError{Status: call.ExitCode}
			fcmd.CombinedOutputScript = append(fcmd.CombinedOutputScript, func() ([]byte, []byte, error) { return []byte(stderr), nil, err })
		} else {
			fcmd.CombinedOutputScript = append(fcmd.CombinedOutputScript, func() ([]byte, []byte, error) { return []byte{}, nil, nil })
		}
	}

	for range calls {
		fexec.CommandScript = append(fexec.CommandScript, func(cmd string, args ...string) exec.Cmd { return fakeexec.InitFakeCmd(fcmd, cmd, args...) })
	}

	return fexec, fcmd
}

func VerifyCallsMatch(t *testing.T, calls []TestCmd, fexec *fakeexec.FakeExec, fcmd *fakeexec.FakeCmd) {
	require.Equal(t, len(calls), fexec.CommandCalls)

	for i, call := range calls {
		require.Equalf(t, call.Cmd, fcmd.CombinedOutputLog[i], "Call [%d] doesn't match expected", i)
	}
}
