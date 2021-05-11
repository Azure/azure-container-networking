package testingutils

import (
	"io"
	"strings"

	"k8s.io/utils/exec"

	fakeexec "k8s.io/utils/exec/testing"
)

type TestCmd struct {
	Cmd      []string
	Stderr   string
	StdOut   string
	ExitCode int
}

func GetFakeExecWithScripts(calls []TestCmd) *fakeexec.FakeExec {
	fexec := &fakeexec.FakeExec{ExactOrder: true, DisableScripts: false}

	fcmd := &fakeexec.FakeCmd{}

	for _, call := range calls {
		stderr := call.Stderr
		stdout := call.StdOut
		ccmd := call.Cmd
		if call.Stderr != "" || call.ExitCode != 0 {
			err := &fakeexec.FakeExitError{Status: call.ExitCode}
			fcmd.CombinedOutputScript = append(fcmd.CombinedOutputScript, func() ([]byte, []byte, error) { return []byte(stdout), []byte(stderr), err })
		} else {
			fcmd.CombinedOutputScript = append(fcmd.CombinedOutputScript, func() ([]byte, []byte, error) { return []byte(stdout), nil, nil })
		}

		fcmd.StdoutPipeResponse = fakeexec.FakeStdIOPipeResponse{ReadCloser: io.NopCloser(strings.NewReader(stdout))}
		fcmd.StderrPipeResponse = fakeexec.FakeStdIOPipeResponse{ReadCloser: io.NopCloser(strings.NewReader(stderr))}

		fexec.CommandScript = append(fexec.CommandScript, func(cmd string, args ...string) exec.Cmd { return fakeexec.InitFakeCmd(fcmd, ccmd[0], ccmd[1:]...) })
	}

	return fexec
}
