package restserver

import (
	"errors"
	"fmt"

	"github.com/Azure/azure-container-networking/cns/types"
)

var errResponseCode = errors.New("Response code is error")

// ResponseCodeToError converts a cns response code to error type. If the response code is OK, then return value is nil
func ResponseCodeToError(responseCode types.ResponseCode) error {
	if responseCode == 0 {
		return nil
	}
	return fmt.Errorf("%w: %v", errResponseCode, responseCode)
}
