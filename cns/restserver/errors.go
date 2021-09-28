package restserver

import (
	"fmt"

	"github.com/Azure/azure-container-networking/cns/types"
)

// CNSRESTError represents a CNS error
type CNSRESTError struct {
	ResponseCode types.ResponseCode
}

func (c *CNSRESTError) Error() string {
	return fmt.Sprintf("response code: %s", c.ResponseCode.String())
}

// ResponseCodeToError converts a cns response code to error type. If the response code is OK, then return value is nil
func ResponseCodeToError(responseCode types.ResponseCode) error {
	if responseCode == types.Success {
		return nil
	}

	return &CNSRESTError{
		ResponseCode: responseCode,
	}
}
