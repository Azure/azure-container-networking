package utils

import (
	"github.com/Azure/azure-container-networking/cns/cnsclient"
	"github.com/Azure/azure-container-networking/cns/restserver"
)

// TODO : Move to common directory like common, after fixing circular dependencies
func IsNotFoundError(err error) bool {
	switch e := err.(type) {
	case *cnsclient.CNSClientError:
		return (e.Code == restserver.UnknownContainerID)
	}
	return false
}
