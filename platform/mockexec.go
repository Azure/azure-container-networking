package platform

import (
	"context"
	"errors"
	"time"
)

type MockExecClient struct {
	returnError                bool
	setExecRawCommand          execRawCommandValidator
	setExecCommand             execCommandValidator
	powershellCommandResponder powershellCommandResponder
}

type (
	execRawCommandValidator    func(string) (string, error)
	execCommandValidator       func(string, ...string) (string, error)
	powershellCommandResponder func(string) (string, error)
)

// ErrMockExec - mock exec error
var ErrMockExec = errors.New("mock exec error")

func NewMockExecClient(returnErr bool) *MockExecClient {
	return &MockExecClient{
		returnError: returnErr,
	}
}

func (e *MockExecClient) ExecuteRawCommand(cmd string) (string, error) {
	if e.setExecRawCommand != nil {
		return e.setExecRawCommand(cmd)
	}

	if e.returnError {
		return "", ErrMockExec
	}

	return "", nil
}

func (e *MockExecClient) ExecuteCommand(_ context.Context, cmd string, args ...string) (string, error) {
	if e.setExecCommand != nil {
		return e.setExecCommand(cmd, args...)
	}

	if e.returnError {
		return "", ErrMockExec
	}

	return "", nil
}

func (e *MockExecClient) SetExecRawCommand(fn execRawCommandValidator) {
	e.setExecRawCommand = fn
}

func (e *MockExecClient) SetExecCommand(fn execCommandValidator) {
	e.setExecCommand = fn
}

func (e *MockExecClient) SetPowershellCommandResponder(fn powershellCommandResponder) {
	e.powershellCommandResponder = fn
}

func (e *MockExecClient) ClearNetworkConfiguration() (bool, error) {
	return true, nil
}

func (e *MockExecClient) ExecutePowershellCommand(cmd string) (string, error) {
	if e.powershellCommandResponder != nil {
		return e.powershellCommandResponder(cmd)
	}
	return "", nil
}

func (e *MockExecClient) GetLastRebootTime() (time.Time, error) {
	return time.Time{}, nil
}

func (e *MockExecClient) KillProcessByName(_ string) error {
	return nil
}

func (e *MockExecClient) ExecutePowershellCommandWithContext(_ context.Context, cmd string) (string, error) {
	if e.powershellCommandResponder != nil {
		return e.powershellCommandResponder(cmd)
	}
	return "", nil
}
