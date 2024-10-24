package restserver

import (
	"context"
	"net/netip"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	nodesubnet "github.com/Azure/azure-container-networking/cns/nodesubnet"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/pkg/errors"
)

var _ nodesubnet.IPConsumer = &HTTPRestService{}

// UpdateIPsForNodeSubnet updates the IP pool of HTTPRestService with newly fetched secondary IPs
func (service *HTTPRestService) UpdateIPsForNodeSubnet(secondaryIPs []netip.Addr) error {
	secondaryIPStrs := make([]string, len(secondaryIPs))
	for i, ip := range secondaryIPs {
		secondaryIPStrs[i] = ip.String()
	}

	networkContainerRequest := nodesubnet.CreateNodeSubnetNCRequest(secondaryIPStrs)

	code, msg := service.saveNetworkContainerGoalState(*networkContainerRequest)
	if code != types.Success {
		return errors.Errorf("failed to save fetched ips. code: %d, message %s", code, msg)
	}

	logger.Debugf("IP change processed successfully")

	// saved NC successfully, generate conflist to indicate CNS is ready
	go service.MustGenerateCNIConflistOnce()
	return nil
}

// InitializeNodeSubnet prepares CNS for serving NodeSubnet requests.
// It sets the orchestrator type to KubernetesCRD, reconciles the initial
// CNS state from the statefile, then creates an IP fetcher.
func (service *HTTPRestService) InitializeNodeSubnet(ctx context.Context, podInfoByIPProvider cns.PodInfoByIPProvider) error {
	// set orchestrator type
	orchestrator := cns.SetOrchestratorTypeRequest{
		OrchestratorType: cns.KubernetesCRD,
	}
	service.SetNodeOrchestrator(&orchestrator)

	if podInfoByIPProvider == nil {
		logger.Printf("PodInfoByIPProvider is nil, this usually means no saved endpoint state. Skipping reconciliation")
	} else if _, err := nodesubnet.ReconcileInitialCNSState(ctx, service, podInfoByIPProvider); err != nil {
		return errors.Wrap(err, "reconcile initial CNS state")
	}
	// statefile (if any) is reconciled. Initialize the IP fetcher. Start the IP fetcher only after the service is started,
	// and any pending async delete operations are completed.
	service.nodesubnetIPFetcher = nodesubnet.NewIPFetcher(service.nma, service, 0, 0, logger.Log)

	return nil
}

func (service *HTTPRestService) StartNodeSubnet(ctx context.Context) {
	service.nodesubnetIPFetcher.Start(ctx)
}
