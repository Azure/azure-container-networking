package ioutil

import (
	"testing"

	testutils "github.com/Azure/azure-container-networking/test/utils"
	"github.com/stretchr/testify/require"
)

func TestGrepMatch(t *testing.T) {
	calls := []testutils.TestCmd{
		{
			Cmd:            []string{"some", "command"},
			PipedToCommand: true,
		},
		{
			Cmd:    []string{"grep", "pattern"},
			Stdout: "1: here's the pattern we're looking for",
		},
		{
			Cmd:            []string{"some", "command"},
			PipedToCommand: true,
		},
		{
			Cmd:    []string{"grep", "pattern"},
			Stdout: "2: here's the pattern we're looking for",
		},
	}
	fexec := testutils.GetFakeExecWithScripts(calls)
	defer testutils.VerifyCalls(t, fexec, calls)

	someCommand := fexec.Command("some", "command")
	grepCommand := fexec.Command(Grep, "pattern")
	output, gotMatches, err := PipeCommandToGrep(someCommand, grepCommand)
	require.NoError(t, err)
	require.True(t, gotMatches)
	require.Equal(t, "1: here's the pattern we're looking for", string(output))

	someCommand = fexec.Command("some", "command")
	grepCommand = fexec.Command(Grep, "pattern")
	output, gotMatches, err = PipeCommandToGrep(someCommand, grepCommand)
	require.NoError(t, err)
	require.True(t, gotMatches)
	require.Equal(t, "2: here's the pattern we're looking for", string(output))
}

func TestGrepNoMatch(t *testing.T) {
	calls := []testutils.TestCmd{
		{
			Cmd:            []string{"some", "command"},
			PipedToCommand: true,
			ExitCode:       0,
		},
		{
			Cmd:      []string{"grep", "pattern"},
			ExitCode: 1,
		},
	}
	fexec := testutils.GetFakeExecWithScripts(calls)
	defer testutils.VerifyCalls(t, fexec, calls)

	someCommand := fexec.Command("some", "command")
	grepCommand := fexec.Command(Grep, "pattern")
	output, gotMatches, err := PipeCommandToGrep(someCommand, grepCommand)
	require.NoError(t, err)
	require.False(t, gotMatches)
	require.Nil(t, output)
}

func TestCommandStartError(t *testing.T) {
	calls := []testutils.TestCmd{
		{
			Cmd:            []string{"some", "command"},
			HasStartError:  true,
			PipedToCommand: true,
			ExitCode:       5,
		},
		{
			Cmd:      []string{"grep", "pattern"},
			ExitCode: 0,
		},
	}
	fexec := testutils.GetFakeExecWithScripts(calls)
	defer testutils.VerifyCalls(t, fexec, calls)

	someCommand := fexec.Command("some", "command")
	grepCommand := fexec.Command(Grep, "pattern")
	output, gotMatches, err := PipeCommandToGrep(someCommand, grepCommand)
	require.Error(t, err)
	require.False(t, gotMatches)
	require.Nil(t, output)
}
