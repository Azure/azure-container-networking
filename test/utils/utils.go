package testingutils

import (
	"io"
	"log"
	"os"
	"os/user"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/utils/exec"

	fakeexec "k8s.io/utils/exec/testing"
)

type TestCmd struct {
	Cmd      []string
	Stdout   string // fakexec doesn't leverage stderr in CombinedOutput, so use stdout for stderr too
	ExitCode int
}

func GetFakeExecWithScripts(calls []TestCmd) *fakeexec.FakeExec {
	fexec := &fakeexec.FakeExec{ExactOrder: true, DisableScripts: false}

	fcmd := &fakeexec.FakeCmd{}

	for _, call := range calls {
		stdout := call.Stdout
		ccmd := call.Cmd
		if call.ExitCode != 0 {
			err := &fakeexec.FakeExitError{Status: call.ExitCode}
			fcmd.CombinedOutputScript = append(fcmd.CombinedOutputScript, func() ([]byte, []byte, error) { return []byte(stdout), nil, err })
		} else {
			fcmd.CombinedOutputScript = append(fcmd.CombinedOutputScript, func() ([]byte, []byte, error) { return []byte(stdout), nil, nil })
		}

		// in fakeexec, stderr isn't used, so we use stdout for piping as well
		fcmd.StdoutPipeResponse = fakeexec.FakeStdIOPipeResponse{ReadCloser: io.NopCloser(strings.NewReader(stdout))}
		fcmd.StderrPipeResponse = fakeexec.FakeStdIOPipeResponse{ReadCloser: io.NopCloser(strings.NewReader(stdout))}

		fexec.CommandScript = append(fexec.CommandScript, func(cmd string, args ...string) exec.Cmd { return fakeexec.InitFakeCmd(fcmd, ccmd[0], ccmd[1:]...) })
	}

	return fexec
}

func VerifyCalls(t *testing.T, fexec *fakeexec.FakeExec, calls []TestCmd) {
	err := recover()
	require.Nil(t, err)
	require.Equalf(t, len(calls), fexec.CommandCalls, "Number of exec calls mismatched, expected [%d], actual [%d]", fexec.CommandCalls, len(calls))
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
