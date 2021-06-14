package testingutils

import (
	"log"
	"os"
	"os/user"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/utils/exec"

	fakeexec "k8s.io/utils/exec/testing"
)

type TestCmd struct {
	Cmd      []string
	Stdout   string
	ExitCode int
}

func GetFakeExecWithScripts(calls []TestCmd) (*fakeexec.FakeExec, *fakeexec.FakeCmd) {
	fexec := &fakeexec.FakeExec{ExactOrder: false, DisableScripts: false}

	fcmd := &fakeexec.FakeCmd{}

	for _, call := range calls {
		stdout := call.Stdout
		if call.ExitCode != 0 {
			err := &fakeexec.FakeExitError{Status: call.ExitCode}
			fcmd.CombinedOutputScript = append(fcmd.CombinedOutputScript, func() ([]byte, []byte, error) { return []byte(stdout), nil, err })
		} else {
			fcmd.CombinedOutputScript = append(fcmd.CombinedOutputScript, func() ([]byte, []byte, error) { return []byte(stdout), nil, nil })
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

func isCurrentUserRoot() bool {
	currentUser, err := user.Current()
	if err != nil {
		log.Printf("Failed to get current user")
		return false
	} else if currentUser.Username == "root" {
		return true
	}
	return false
}

func RequireRootforTest(t *testing.T) {
	if !isCurrentUserRoot() {
		t.Fatalf("Test [%s] requires root!", t.Name())
	}
}

func RequireRootforTestMain() {
	if !isCurrentUserRoot() {
		log.Printf("These tests require root!")
		os.Exit(1)
	}
}
