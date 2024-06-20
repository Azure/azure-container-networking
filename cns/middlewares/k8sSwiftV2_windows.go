package middlewares

import (
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
)

// setRoutes sets the routes for podIPInfo used in SWIFT V2 scenario. This is a no-op as route setting is not applicable for Windows.
func (k *K8sSWIFTv2Middleware) setRoutes(_ *cns.PodIpInfo) error {
	logger.Printf("[SWIFTv2Middleware] setRoutes is a no-op on Windows")
	return nil
}
