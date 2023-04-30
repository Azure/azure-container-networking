package platform

import "errors"

type MockExecClient struct {
	returnError    bool
	setExecCommand execCommandValidator
}

type execCommandValidator func(string) (string, error)

// ErrMockExec - mock exec error
var ErrMockExec = errors.New("mock exec error")

func NewMockExecClient(returnErr bool) *MockExecClient {
	return &MockExecClient{
		returnError: returnErr,
	}
}

func (e *MockExecClient) ExecuteCommand(cmd string) (string, error) {
	if e.setExecCommand != nil {
		return e.setExecCommand(cmd)
	}

	if e.returnError {
		return "", ErrMockExec
	}

	return "", nil
}

func (e *MockExecClient) SetExecCommand(fn execCommandValidator) {
	e.setExecCommand = fn
}
