package testingutils

import (
	"k8s.io/utils/exec"

	testing "k8s.io/utils/exec/testing"
)

type TestCmd struct {
	Cmd []string
	Err *testing.FakeExitError
}

func GetFakeExecWithScripts(calls []TestCmd) *testing.FakeExec {

	fakeexec := &testing.FakeExec{ExactOrder: true, DisableScripts: false}
	for _, call := range calls {
		var err testing.FakeExitError
		if call.Err != nil {
			err = *call.Err
		}
		fakeCmd := &testing.FakeCmd{}
		cmdAction := makeFakeCmd(fakeCmd, call.Cmd[0], call.Cmd[1:]...)
		fakeCmd.CombinedOutputScript = append(fakeCmd.CombinedOutputScript, func() ([]byte, []byte, error) { return nil, nil, err })
		fakeexec.CommandScript = append(fakeexec.CommandScript, cmdAction)
	}
	return fakeexec
}

func makeFakeCmd(fakeCmd *testing.FakeCmd, cmd string, args ...string) testing.FakeCommandAction {
	c := cmd
	a := args
	return func(cmd string, args ...string) exec.Cmd {
		command := testing.InitFakeCmd(fakeCmd, c, a...)
		return command
	}
}
