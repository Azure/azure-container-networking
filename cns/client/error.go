package client

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/Azure/azure-container-networking/cns/types"
)

// FailedHTTPRequest describes an HTTP request to CNS that has returned a
// non-200 status code.
type FailedHTTPRequest struct {
	Code int
}

func (f *FailedHTTPRequest) Error() string {
	return fmt.Sprintf("http request failed: %s (%d)", http.StatusText(f.Code), f.Code)
}

// CNSClientError records an error and relevant code
type CNSClientError struct {
	Code types.ResponseCode
	Err  error
}

func (e *CNSClientError) Error() string {
	return fmt.Sprintf("[Azure cnsclient] Code: %d , Error: %v", e.Code, e.Err)
}

// IsNotFound tests if the provided error is of type CNSClientError and then
// further tests if the error code is of type UnknowContainerID
func IsNotFound(err error) bool {
	e := &CNSClientError{}
	return errors.As(err, &e) && (e.Code == types.UnknownContainerID)
}
