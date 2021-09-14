package client

import (
	"errors"
	"fmt"

	"github.com/Azure/azure-container-networking/cns/types"
)

// CNSClientError records an error and relevant code
type CNSClientError struct {
	Code types.ResponseCode
	Err  error
}

func (e *CNSClientError) Error() string {
	return fmt.Sprintf("[Azure CNSClient] Code: %d , Error: %v", e.Code, e.Err)
}

// IsNotFound tests if the provided error is of type CNSClientError and then
// further tests if the error code is of type UnknowContainerID
func IsNotFound(err error) bool {
	e := &CNSClientError{}
	return errors.As(err, &e) && (e.Code == types.UnknownContainerID)
}
