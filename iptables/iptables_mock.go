package iptables

import (
	"errors"
	"fmt"
)

var errorMockIPTableCommand = errors.New("MockIPTableCommandError")

func newErrorMockIPTableCommand(errorString string) error {
	return fmt.Errorf("%w: %s", errorMockIPTableCommand, errorString)
}

type MockIPTableCommand struct {
	returnError bool
	errorStr    string
}

func NewMockIPTableCommand(returnError bool, errorStr string) MockIPTableCommand {
	return MockIPTableCommand{
		returnError: returnError,
		errorStr:    errorStr,
	}
}

func (m MockIPTableCommand) RunCmd(version string, params string) error {
	if m.returnError {
		return newErrorMockIPTableCommand(m.errorStr)
	}

	fmt.Printf("[mock iptables] Running command - iptables %s\n", params)
	return nil
}
