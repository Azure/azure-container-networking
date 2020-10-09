package utils

import (
	"errors"

	"github.com/Azure/azure-container-networking/cns/cnsclient"
	"github.com/Azure/azure-container-networking/cns/restserver"
)

// TODO : Move to common directory like common, after fixing circular dependencies
func IsNotFoundError(err error) bool {
	switch err := err.(type) {
	case *cnsclient.CNSClientError:
		var cnsError *cnsclient.CNSClientError
		// not expected to fail
		_ = errors.As(err, &cnsError)
		return (cnsError.Code == restserver.UnknownContainerID)
	}
	return false
}
