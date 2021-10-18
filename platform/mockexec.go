package platform

import "errors"

type mockExecClient struct {
	returnError bool
}

// ErrMockExec - mock exec error
var ErrMockExec = errors.New("mock exec error")

func NewMockExecClient(returnErr bool) ExecClient {
	return &mockExecClient{
		returnError: returnErr,
	}
}

func (e *mockExecClient) ExecuteCommand(string) (string, error) {
	if e.returnError {
		return "", ErrMockExec
	}

	return "", nil
}
