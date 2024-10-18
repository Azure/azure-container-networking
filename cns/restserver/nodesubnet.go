package restserver

import (
	"context"
	"net/netip"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	nodesubnet "github.com/Azure/azure-container-networking/cns/nodesubnet"
	"github.com/Azure/azure-container-networking/cns/types"
	errors "github.com/pkg/errors"
)

var _ nodesubnet.IPConsumer = &HTTPRestService{}

// Implement the UpdateIPsForNodeSubnet method for HTTPRestService
func (service *HTTPRestService) UpdateIPsForNodeSubnet(secondaryIPs []netip.Addr) error {
	secondaryIPStrs := make([]string, len(secondaryIPs))
	for i, ip := range secondaryIPs {
		secondaryIPStrs[i] = ip.String()
	}

	networkContainerRequest := nodesubnet.CreateNodeSubnetNCRequest(secondaryIPStrs)

	code, msg := service.saveNetworkContainerGoalState(*networkContainerRequest)
	if code != types.Success {
		logger.Debugf("Error in processing IP change")
		return errors.Errorf("failed to save fetched ips. code: %d, message %s", code, msg)
	}

	logger.Debugf("IP change processed successfully")

	// saved NC successfully, generate conflist to indicate CNS is ready
	go service.MustGenerateCNIConflistOnce()
	return nil
}

func (service *HTTPRestService) InitializeNodeSubnet(ctx context.Context, podInfoByIPProvider cns.PodInfoByIPProvider) error {
	// Set orchestrator type
	orchestrator := cns.SetOrchestratorTypeRequest{
		OrchestratorType: cns.KubernetesCRD,
	}
	service.SetNodeOrchestrator(&orchestrator)
	service.nodesubnetIPFetcher = nodesubnet.NewIPFetcher(service.nma, service, 0, 0, logger.Log)
	if podInfoByIPProvider == nil {
		logger.Printf("PodInfoByIPProvider is nil, this usually means no saved endpoint state. Skipping reconciliation")
	} else if _, err := nodesubnet.ReconcileInitialCNSState(ctx, service, podInfoByIPProvider); err != nil {
		return errors.Wrap(err, "reconcile initial CNS state")
	}

	return nil
}

func (service *HTTPRestService) StartNodeSubnet(ctx context.Context) {
	service.nodesubnetIPFetcher.Start(ctx)
}
