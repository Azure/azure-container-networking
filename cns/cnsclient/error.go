package cnsclient

import (
	"fmt"
)

// CNSClientError records an error and relevant code
type CNSClientError struct {
	Code int
	Err  error
}

func (e *CNSClientError) Error() string {
	return fmt.Sprintf("[Azure CNSClient] Code: %d , Error: %v", e.Code, e.Err)
}
