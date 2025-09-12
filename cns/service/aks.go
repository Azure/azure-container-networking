// AKS specific initialization flows
// nolint // it's not worth it
package main

import (
	"context"

	"github.com/Azure/azure-container-networking/cns"
	cnstypes "github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
)

type cniConflistScenario string

const (
	scenarioV4Overlay        cniConflistScenario = "v4overlay"
	scenarioDualStackOverlay cniConflistScenario = "dualStackOverlay"
	scenarioOverlay          cniConflistScenario = "overlay"
	scenarioCilium           cniConflistScenario = "cilium"
	scenarioSWIFT            cniConflistScenario = "swift"
)

type nodeNetworkConfigGetter interface {
	Get(context.Context) (*v1alpha.NodeNetworkConfig, error)
}

type ipamStateReconciler interface {
	ReconcileIPAMStateForSwift(ncRequests []*cns.CreateNetworkContainerRequest, podInfoByIP map[string]cns.PodInfo, nnc *v1alpha.NodeNetworkConfig) cnstypes.ResponseCode
}
