package middlewares

import (
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
)

// for AKS L1VH, do not set default route on infraNIC to avoid customer pod reaching all infra vnet services
// default route is set for secondary interface NIC(i.e,delegatedNIC)
func (k *K8sSWIFTv2Middleware) setRoutes(podIPInfo *cns.PodIpInfo) error {
	logger.Printf("[SWIFTv2Middleware] setRoutes: only set skipDefaultRoutes flag to true for InfraNIC")
	if podIPInfo.NICType == cns.InfraNIC {
		podIPInfo.SkipDefaultRoutes = true
	}
	return nil
}
