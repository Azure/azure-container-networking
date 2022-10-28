//go:build !windows
// +build !windows

package ioutil

import (
	"testing"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	testutils "github.com/Azure/azure-container-networking/test/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testCommandString = "test command"
	section1ID        = "section1"
	section2ID        = "section2"
	section3ID        = "section3"
)

var (
	fakeSuccessCommand = testutils.TestCmd{
		Cmd: []string{testCommandString},
	}
	fakeFailureCommand = testutils.TestCmd{
		Cmd:      []string{testCommandString},
		ExitCode: 1,
	}
)

func TestToStringAndSections(t *testing.T) {
	creator := NewFileCreator(common.NewMockIOShim(nil), 1)
	creator.AddLine(section1ID, nil, "line1-item1", "line1-item2", "line1-item3")
	creator.AddLine(section2ID, nil, "line2-item1", "line2-item2", "line2-item3")
	creator.AddLine(section1ID, nil, "line3-item1", "line3-item2", "line3-item3")

	section1 := creator.sections[section1ID]
	require.Equal(t, section1ID, section1.id)
	require.Equal(t, []int{0, 2}, section1.lineNums)

	section2 := creator.sections[section2ID]
	require.Equal(t, section2ID, section2.id)
	require.Equal(t, []int{1}, section2.lineNums)

	fileString := creator.ToString()
	assert.Equal(
		t,
		`line1-item1 line1-item2 line1-item3
line2-item1 line2-item2 line2-item3
line3-item1 line3-item2 line3-item3
`,
		fileString,
	)
}

func TestRunCommandWithFile(t *testing.T) {
	calls := []testutils.TestCmd{fakeSuccessCommand}
	creator := NewFileCreator(common.NewMockIOShim(calls), 1)
	creator.AddLine("", nil, "line1")
	require.NoError(t, creator.RunCommandWithFile(testCommandString))
}

func TestRunCommandWhenFileIsEmpty(t *testing.T) {
	calls := []testutils.TestCmd{fakeSuccessCommand}
	creator := NewFileCreator(common.NewMockIOShim(calls), 1)
	wasFileAltered, err := creator.RunCommandOnceWithFile(testCommandString)
	require.False(t, wasFileAltered)
	require.NoError(t, err)
}

func TestRunCommandSuccessAfterRecovery(t *testing.T) {
	failure := fakeFailureCommand
	failure.Stdout = "failure on line 1"
	calls := []testutils.TestCmd{failure, fakeSuccessCommand}
	creator := NewFileCreator(common.NewMockIOShim(calls), 2, "failure on line (\\d+)")

	errorHandlers := []*LineErrorHandler{
		{
			Definition: AlwaysMatchDefinition,
			Method:     Continue,
			Callback:   func() { log.Logf("'continue' callback") },
		},
	}
	creator.AddLine("", errorHandlers, "line1")
	creator.AddLine("", nil, "line2")

	originalFileString := "line1\nline2\n"
	require.Equal(t, originalFileString, creator.ToString())

	require.NoError(t, creator.RunCommandWithFile(testCommandString))

	changedFileString := "line2\n"
	require.Equal(t, changedFileString, creator.ToString())
}

func TestRunCommandFailureFromNoMoreTries(t *testing.T) {
	calls := []testutils.TestCmd{fakeFailureCommand}
	creator := NewFileCreator(common.NewMockIOShim(calls), 1)
	creator.AddLine("", nil, "line1")
	require.Error(t, creator.RunCommandWithFile(testCommandString))
}

func TestRunCommandOnceWithNoMoreTries(t *testing.T) {
	creator := NewFileCreator(common.NewMockIOShim(nil), 0)
	_, err := creator.RunCommandOnceWithFile(testCommandString)
	require.Error(t, err)
}

func TestRecoveryForFileLevelErrors(t *testing.T) {
	knownFileLevelErrorCommand := testutils.TestCmd{
		Cmd:      []string{testCommandString},
		Stdout:   "file-level error over here",
		ExitCode: 1,
	}
	unknownFileLevelErrorCommand := testutils.TestCmd{
		Cmd:      []string{testCommandString},
		Stdout:   "not sure what's wrong",
		ExitCode: 1,
	}
	calls := []testutils.TestCmd{
		knownFileLevelErrorCommand,
		unknownFileLevelErrorCommand,
		unknownFileLevelErrorCommand,
		fakeSuccessCommand,
	}
	creator := NewFileCreator(common.NewMockIOShim(calls), 4)
	creator.AddErrorToRetryOn(NewErrorDefinition("file-level error"))
	creator.AddLine("", nil, "line1")
	wasFileAltered, err := creator.RunCommandOnceWithFile(testCommandString)
	require.False(t, wasFileAltered)
	require.Error(t, err)
	wasFileAltered, err = creator.RunCommandOnceWithFile(testCommandString)
	require.False(t, wasFileAltered)
	require.Error(t, err)
	require.NoError(t, creator.RunCommandWithFile(testCommandString))
}

func TestRecoveryWhenFileAltered(t *testing.T) {
	fakeErrorCommand := testutils.TestCmd{
		Cmd:      []string{testCommandString},
		Stdout:   "failure on line 2: match-pattern do something please",
		ExitCode: 1,
	}
	calls := []testutils.TestCmd{fakeErrorCommand, fakeSuccessCommand}
	creator := NewFileCreator(common.NewMockIOShim(calls), 2, "failure on line (\\d+)")
	errorHandlers := []*LineErrorHandler{
		{
			Definition: NewErrorDefinition("match-pattern"),
			Method:     Continue,
			Callback:   func() { log.Logf("'continue' callback") },
		},
	}
	creator.AddLine(section1ID, nil, "line1-item1", "line1-item2", "line1-item3")
	creator.AddLine(section2ID, errorHandlers, "line2-item1", "line2-item2", "line2-item3")
	creator.AddLine(section1ID, nil, "line3-item1", "line3-item2", "line3-item3")
	require.NoError(t, creator.RunCommandWithFile(testCommandString))
}

func TestHandleLineErrorForContinueAndAbortSection(t *testing.T) {
	fakeErrorCommand := testutils.TestCmd{
		Cmd:      []string{testCommandString},
		Stdout:   "failure on line 2: match-pattern do something please",
		ExitCode: 1,
	}
	calls := []testutils.TestCmd{fakeErrorCommand}
	creator := NewFileCreator(common.NewMockIOShim(calls), 2, "failure on line (\\d+)")
	errorHandlers := []*LineErrorHandler{
		// first error handler doesn't match (include this to make sure the real match gets reached)
		{
			Definition: NewErrorDefinition("abc"),
			Method:     ContinueAndAbortSection,
			Callback:   func() {},
		},
		{
			Definition: NewErrorDefinition("match-pattern"),
			Method:     ContinueAndAbortSection,
			Callback:   func() { log.Logf("'continue and abort section' callback") },
		},
	}
	creator.AddLine(section1ID, nil, "line1-item1", "line1-item2", "line1-item3")
	creator.AddLine(section2ID, errorHandlers, "line2-item1", "line2-item2", "line2-item3")
	creator.AddLine(section1ID, nil, "line3-item1", "line3-item2", "line3-item3")
	creator.AddLine(section2ID, nil, "line4-item1", "line4-item2", "line4-item3")
	creator.AddLine(section3ID, nil, "line5-item1", "line5-item2", "line5-item3")
	wasFileAltered, err := creator.RunCommandOnceWithFile(testCommandString)
	require.Error(t, err)
	require.True(t, wasFileAltered)
	fileString := creator.ToString()
	assert.Equal(t, "line3-item1 line3-item2 line3-item3\nline5-item1 line5-item2 line5-item3\n", fileString)

	creator.logLines(testCommandString)
	require.Equal(t, map[int]struct{}{0: {}, 1: {}, 3: {}}, creator.lineNumbersToOmit, "expected line 1, 2, and 4 to be marked omitted")

	_, line := creator.handleLineError("some error", testCommandString, 1)
	require.Equal(t, creator.lines[2], line, "expected a failure in line 1 to map to original line 3")

	_, line = creator.handleLineError("some error", testCommandString, 2)
	require.Equal(t, creator.lines[4], line, "expected a failure in line 2 to map to original line 5")
}

func TestHandleLineErrorForContinue(t *testing.T) {
	fakeErrorCommand := testutils.TestCmd{
		Cmd:      []string{testCommandString},
		Stdout:   "failure on line 2: match-pattern do something please",
		ExitCode: 1,
	}
	calls := []testutils.TestCmd{fakeErrorCommand}
	creator := NewFileCreator(common.NewMockIOShim(calls), 2, "failure on line (\\d+)")
	errorHandlers := []*LineErrorHandler{
		{
			Definition: NewErrorDefinition("match-pattern"),
			Method:     Continue,
			Callback:   func() { log.Logf("'continue' callback") },
		},
	}
	creator.AddLine("", nil, "line1-item1", "line1-item2", "line1-item3")
	creator.AddLine("", errorHandlers, "line2-item1", "line2-item2", "line2-item3")
	creator.AddLine("", nil, "line3-item1", "line3-item2", "line3-item3")
	creator.AddLine("", errorHandlers, "line4-item1", "line4-item2", "line4-item3")
	wasFileAltered, err := creator.RunCommandOnceWithFile(testCommandString)
	require.Error(t, err)
	require.True(t, wasFileAltered)
	fileString := creator.ToString()
	assert.Equal(t, "line3-item1 line3-item2 line3-item3\nline4-item1 line4-item2 line4-item3\n", fileString)

	creator.logLines(testCommandString)
	require.Equal(t, map[int]struct{}{0: {}, 1: {}}, creator.lineNumbersToOmit, "expected line 1 and 2 to be marked omitted")

	_, line := creator.handleLineError("some error", testCommandString, 1)
	require.Equal(t, creator.lines[2], line, "expected a failure in line 1 to map to original line 3")

	_, line = creator.handleLineError("some error", testCommandString, 2)
	require.Equal(t, creator.lines[3], line, "expected a failure in line 2 to map to original line 4")
}

func TestHandleLineErrorNoMatch(t *testing.T) {
	fakeErrorCommand := testutils.TestCmd{
		Cmd:      []string{testCommandString},
		Stdout:   "failure on line 2: match-pattern do something please",
		ExitCode: 1,
	}
	calls := []testutils.TestCmd{fakeErrorCommand}
	creator := NewFileCreator(common.NewMockIOShim(calls), 2, "failure on line (\\d+)")
	errorHandlers := []*LineErrorHandler{
		{
			Definition: NewErrorDefinition("abc"),
			Method:     ContinueAndAbortSection,
			Callback:   func() {},
		},
	}
	creator.AddLine("", nil, "line1-item1", "line1-item2", "line1-item3")
	creator.AddLine("", errorHandlers, "line2-item1", "line2-item2", "line2-item3")
	creator.AddLine("", nil, "line3-item1", "line3-item2", "line3-item3")
	fileStringBefore := creator.ToString()
	wasFileAltered, err := creator.RunCommandOnceWithFile(testCommandString)
	require.Error(t, err)
	require.False(t, wasFileAltered)
	fileStringAfter := creator.ToString()
	require.Equal(t, fileStringBefore, fileStringAfter)

	creator.logLines(testCommandString)
	require.Equal(t, map[int]struct{}{}, creator.lineNumbersToOmit, "expected no lines to be marked omitted")
}

func TestAlwaysMatchDefinition(t *testing.T) {
	require.True(t, AlwaysMatchDefinition.isMatch("123456789asdfghjklxcvbnm, jklfdsa7"))
}

func TestGetErrorLineNumber(t *testing.T) {
	type args struct {
		lineFailurePattern string
		stdErr             string
	}

	tests := []struct {
		name            string
		args            args
		expectedLineNum int
	}{
		{
			"pattern that doesn't match",
			args{
				"abc",
				"xyz",
			},
			-1,
		},
		{
			"matching pattern with no group",
			args{
				"abc",
				"abc",
			},
			-1,
		},
		{
			"matching pattern with non-numeric group",
			args{
				"(abc)",
				"abc",
			},
			-1,
		},
		{
			"stderr gives an out-of-bounds line number",
			args{
				"line (\\d+)",
				"line 777",
			},
			-1,
		},
		{
			"good line match",
			args{
				"line (\\d+)",
				`there was a failure
				on line 11 where the failure happened
				fix it please`,
			},
			11,
		},
	}

	commandString := "test command"
	for _, tt := range tests {
		lineFailureDefinition := NewErrorDefinition(tt.args.lineFailurePattern)
		expectedLineNum := tt.expectedLineNum
		stdErr := tt.args.stdErr
		t.Run(tt.name, func(t *testing.T) {
			lineNum := lineFailureDefinition.getErrorLineNumber(stdErr, commandString, 15)
			require.Equal(t, expectedLineNum, lineNum)
		})
	}
}
