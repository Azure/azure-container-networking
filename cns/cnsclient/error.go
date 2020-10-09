package cnsclient

import (
	"fmt"

	"github.com/Azure/azure-container-networking/cns/restserver"
)

// CNSClientError records an error and relevant code
type CNSClientError struct {
	Code int
	Err  error
}

func (e *CNSClientError) Error() string {
	return fmt.Sprintf("[Azure CNSClient] Code: %d , Error: %v", e.Code, e.Err)
}

// IsNotFoundError - Returns a boolean if the error code is not found or not?
func (e *CNSClientError) IsNotFoundError() bool {
	return e.Code == restserver.UnknownContainerID
}
