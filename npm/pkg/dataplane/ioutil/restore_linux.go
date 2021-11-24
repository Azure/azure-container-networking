package ioutil

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/Azure/azure-container-networking/common"
	npmerrors "github.com/Azure/azure-container-networking/npm/util/errors"
	"k8s.io/klog"
)

// FileCreator is a tool for:
// - building a buffer file
// - running a command with the file
// - handling errors in the file
type FileCreator struct {
	lines                  []*Line
	sections               map[string]*Section // key is sectionID
	lineNumbersToOmit      map[int]struct{}
	errorsToRetryOn        []*ErrorDefinition
	lineFailureDefinitions []*ErrorDefinition
	tryCount               int
	maxTryCount            int
	ioShim                 *common.IOShim
}

// TODO ideas:
// - section to error handler(s) map for addLine
// - error handlers have the kind of line error pattern as a requirement

// Line defines the content, section, and error handlers for a line
type Line struct {
	content       string
	sectionID     string
	errorHandlers []*LineErrorHandler
}

// Section is a logically connected components (not necessarily adjacent lines)
type Section struct {
	id       string
	lineNums []int
}

// ErrorDefinition defines an error by a regular expression and its error code.
type ErrorDefinition struct {
	matchPattern string
	re           *regexp.Regexp
}

// LineErrorHandler defines an error and how to handle it
type LineErrorHandler struct {
	Definition *ErrorDefinition
	Method     LineErrorHandlerMethod
	Callback   func()
}

// LineErrorHandlerMethod defines behavior when an error occurs
type LineErrorHandlerMethod string

const (
	// Continue specifies skipping this line and all previous lines
	Continue LineErrorHandlerMethod = "continue"
	// ContinueAndAbortSection specifies skipping this line, all previous lines, and all lines tied to this line's section
	ContinueAndAbortSection LineErrorHandlerMethod = "continue-and-abort"

	anyMatchPattern = ".*"
)

// AlwaysMatchDefinition will match any error
var AlwaysMatchDefinition = NewErrorDefinition(anyMatchPattern)

func NewFileCreator(ioShim *common.IOShim, maxTryCount int, lineFailurePatterns ...string) *FileCreator {
	creator := &FileCreator{
		lines:                  make([]*Line, 0),
		sections:               make(map[string]*Section),
		lineNumbersToOmit:      make(map[int]struct{}),
		errorsToRetryOn:        make([]*ErrorDefinition, 0),
		lineFailureDefinitions: make([]*ErrorDefinition, len(lineFailurePatterns)),
		tryCount:               0,
		maxTryCount:            maxTryCount,
		ioShim:                 ioShim,
	}
	for k, lineFailurePattern := range lineFailurePatterns {
		creator.lineFailureDefinitions[k] = NewErrorDefinition(lineFailurePattern)
	}
	return creator
}

func NewErrorDefinition(pattern string) *ErrorDefinition {
	return &ErrorDefinition{
		matchPattern: pattern,
		re:           regexp.MustCompile(pattern),
	}
}

func (creator *FileCreator) AddErrorToRetryOn(definition *ErrorDefinition) {
	creator.errorsToRetryOn = append(creator.errorsToRetryOn, definition)
}

func (creator *FileCreator) AddLine(sectionID string, errorHandlers []*LineErrorHandler, items ...string) {
	section, exists := creator.sections[sectionID]
	if !exists {
		section = &Section{sectionID, make([]int, 0)}
		creator.sections[sectionID] = section
	}
	spaceSeparatedItems := strings.Join(items, " ")
	line := &Line{spaceSeparatedItems, sectionID, errorHandlers}
	creator.lines = append(creator.lines, line)
	section.lineNums = append(section.lineNums, len(creator.lines)-1)
}

// ToString combines the lines in the FileCreator and ends with a new line.
func (creator *FileCreator) ToString() string {
	result := strings.Builder{}
	for lineNum, line := range creator.lines {
		_, isOmitted := creator.lineNumbersToOmit[lineNum]
		if !isOmitted {
			result.WriteString(line.content + "\n")
		}
	}
	return result.String()
}

func (creator *FileCreator) RunCommandWithFile(cmd string, args ...string) error {
	fileString := creator.ToString()
	klog.Infof("beginning to run command for restorer:\nBEGIN-CREATOR-FILE-FOR-COMMAND-%s\n%s\nEND-CREATOR-FILE-FOR-COMMAND-%s", cmd, fileString, cmd) // TODO remove
	wasFileAltered, err := creator.runCommandOnceWithFile(fileString, cmd, args...)
	if err == nil {
		return nil
	}
	commandString := cmd + " " + strings.Join(args, " ")
	for !creator.hasNoMoreRetries() {
		if wasFileAltered {
			fileString = creator.ToString()
			klog.Infof("rerunning command [%s] with new file:\n%s", commandString, fileString)
		} else {
			klog.Infof("rerunning command [%s] with the same file", commandString)
		}
		wasFileAltered, err = creator.runCommandOnceWithFile(fileString, cmd, args...)
		if err == nil {
			klog.Infof("successfully ran command [%s] on try number %d", commandString, creator.tryCount)
			return nil
		}
	}
	errString := fmt.Sprintf("failed to run command [%s] with error: %v", commandString, err)
	klog.Error(errString)
	// TODO conditionally specify as retriable?
	return npmerrors.Errorf(npmerrors.RunFileCreator, false, errString)
}

// RunCommandOnceWithFile runs the command with the file once and increments the try count.
// It returns whether the file was altered and any error.
// For automatic retrying and proper logging, use RunCommandWithFile.
// This method can be used for external testing of file creator contents after each run.
func (creator *FileCreator) RunCommandOnceWithFile(cmd string, args ...string) (bool, error) {
	if creator.hasNoMoreRetries() {
		return false, npmerrors.Errorf(npmerrors.RunFileCreator, false, fmt.Sprintf("reached max try count %d", creator.tryCount))
	}
	fileString := creator.ToString()
	return creator.runCommandOnceWithFile(fileString, cmd, args...)
}

// returns whether the file was altered and any error
// TODO return another bool that specifies if there was a file-level retriable error?
func (creator *FileCreator) runCommandOnceWithFile(fileString, cmd string, args ...string) (bool, error) {
	commandString := cmd + " " + strings.Join(args, " ")
	if fileString == "" { // NOTE this wouldn't prevent us from running an iptables restore file with just "COMMIT\n"
		klog.Infof("returning as a success without running command [%s] since the fileString is empty", commandString)
		return false, nil
	}

	command := creator.ioShim.Exec.Command(cmd, args...)
	command.SetStdin(bytes.NewBufferString(fileString))

	// run the command
	stdErrBytes, err := command.CombinedOutput()
	creator.tryCount++
	if err == nil {
		// success
		return false, nil
	}

	stdErr := string(stdErrBytes)
	klog.Errorf("on try number %d, failed to run command [%s] with error [%v] and stdErr [%s]. Used file:\n%s", creator.tryCount, commandString, err, stdErr, fileString)
	if creator.hasNoMoreRetries() {
		return false, fmt.Errorf("after %d tries, failed with final error [%w] and stdErr [%s]", creator.tryCount, err, stdErr)
	}

	// begin the retry logic
	if creator.hasFileLevelError(stdErr) {
		klog.Infof("detected a file-level error after running command [%s]", commandString)
		return false, fmt.Errorf("file-level error: %w", err)
	}

	// no file-level error, so handle line-level error if there is one
	numLines := creator.numLines()
	for _, lineFailureDefinition := range creator.lineFailureDefinitions {
		lineNum := lineFailureDefinition.getErrorLineNumber(stdErr, commandString, numLines)
		if lineNum != -1 {
			klog.Infof("detected a line number error on line %d", lineNum)
			wasFileAltered := creator.handleLineError(stdErr, commandString, lineNum)
			return wasFileAltered, fmt.Errorf("tried to handle line number error: %w", err)
		}
	}
	klog.Infof("couldn't detect a line number error")
	return false, fmt.Errorf("can't discern error: %w", err)
}

func (creator *FileCreator) hasNoMoreRetries() bool {
	return creator.tryCount >= creator.maxTryCount
}

func (creator *FileCreator) hasFileLevelError(stdErr string) bool {
	for _, errorDefinition := range creator.errorsToRetryOn {
		if errorDefinition.isMatch(stdErr) {
			return true
		}
	}
	return false
}

func (definition *ErrorDefinition) isMatch(stdErr string) bool {
	return definition.matchPattern == anyMatchPattern || definition.re.MatchString(stdErr)
}

func (creator *FileCreator) numLines() int {
	return len(creator.lines) - len(creator.lineNumbersToOmit)
}

// return -1 if there's a failure
func (definition *ErrorDefinition) getErrorLineNumber(stdErr, commandString string, numLines int) int {
	result := definition.re.FindStringSubmatch(stdErr)
	if result == nil || len(result) < 2 {
		klog.Infof("expected error with line number, but couldn't detect one with error regex pattern [%s] for command [%s] with stdErr [%s]", definition.matchPattern, commandString, stdErr)
		return -1
	}
	lineNumString := result[1]
	lineNum, err := strconv.Atoi(lineNumString)
	if err != nil {
		klog.Infof("expected error with line number, but error regex pattern %s didn't produce a number for command [%s] with stdErr [%s]", definition.matchPattern, commandString, stdErr)
		return -1
	}
	if lineNum < 1 || lineNum > numLines {
		klog.Infof(
			"expected error with line number, but error regex pattern %s produced an invalid line number %d for command [%s] with stdErr [%s]",
			definition.matchPattern, lineNum, commandString, stdErr,
		)
		return -1
	}
	return lineNum
}

// return whether the file was altered
func (creator *FileCreator) handleLineError(stdErr, commandString string, lineNum int) bool {
	lineNumIndex := lineNum - 1
	line := creator.lines[lineNumIndex]
	for _, errorHandler := range line.errorHandlers {
		if !errorHandler.Definition.isMatch(stdErr) {
			continue
		}
		switch errorHandler.Method {
		case Continue:
			klog.Errorf("continuing after line %d for command [%s]", lineNum, commandString)
			for i := 0; i <= lineNumIndex; i++ {
				creator.lineNumbersToOmit[i] = struct{}{}
			}
		case ContinueAndAbortSection:
			klog.Errorf("continuing after line %d and aborting section associated with the line for command [%s]", lineNum, commandString)
			for i := 0; i <= lineNumIndex; i++ {
				creator.lineNumbersToOmit[i] = struct{}{}
			}
			section := creator.sections[line.sectionID]
			for _, lineNum := range section.lineNums {
				creator.lineNumbersToOmit[lineNum] = struct{}{}
			}
		}
		errorHandler.Callback()
		return true
	}
	klog.Infof("no error handler for line %d for command [%s] with stdErr [%s]", lineNum, commandString, stdErr)
	return false
}
