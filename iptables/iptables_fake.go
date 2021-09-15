package iptables

import "errors"

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
		return errors.New(m.errorStr)
	}
	return nil
}
