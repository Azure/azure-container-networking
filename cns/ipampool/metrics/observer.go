package metrics

import (
	"context"
	"fmt"
	"net/netip"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/crd/clustersubnetstate/api/v1alpha1"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/pkg/errors"
)

// Subnet ARM ID /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/$(SUBNET)
const subnetARMIDTemplate = "/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/virtualNetworks/%s/subnets/%s"

// ipPoolState is the current actual state of the CNS IP pool.
type ipPoolState struct {
	// allocatedToPods are the IPs CNS gives to Pods.
	allocatedToPods int64
	// available are the IPs in state "Available".
	available int64
	// currentAvailableIPs are the current available IPs: allocated - assigned - pendingRelease.
	currentAvailableIPs int64
	// expectedAvailableIPs are the "future" available IPs, if the requested IP count is honored: requested - assigned.
	expectedAvailableIPs int64
	// pendingProgramming are the IPs in state "PendingProgramming".
	pendingProgramming int64
	// pendingRelease are the IPs in state "PendingRelease".
	pendingRelease int64
	// requestedIPs are the IPs CNS has requested that it be allocated by DNC.
	requestedIPs int64
	// secondaryIPs are all the IPs given to CNS by DNC, not including the primary IP of the NC.
	secondaryIPs int64
}

// metaState is the Monitor's configuration state for the IP pool.
type metaState struct {
	batch              int64
	exhausted          bool
	max                int64
	primaryIPAddresses map[string]struct{}
	subnet             string
	subnetARMID        string
	subnetCIDR         string
}

// NewLegacyMetricsObserver creates a closed functional scope which can be invoked to
// observe the legacy IPAM pool metrics.
//
//nolint:lll // ignore line length
func NewLegacyMetricsObserver(ctx context.Context, ipcli func() map[string]cns.IPConfigurationStatus, nnccli func(context.Context) (*v1alpha.NodeNetworkConfig, error), csscli func(context.Context) ([]v1alpha1.ClusterSubnetState, error)) func() error {
	return func() error {
		return observeMetrics(ctx, ipcli, nnccli, csscli)
	}
}

// generateARMID uses the Subnet ARM ID format to populate the ARM ID with the metadata.
// If either of the metadata attributes are empty, then the ARM ID will be an empty string.
func generateARMID(nc *v1alpha.NetworkContainer) string {
	subscription := nc.SubscriptionID
	resourceGroup := nc.ResourceGroupID
	vnetID := nc.VNETID
	subnetID := nc.SubnetID

	if subscription == "" || resourceGroup == "" || vnetID == "" || subnetID == "" {
		return ""
	}
	return fmt.Sprintf(subnetARMIDTemplate, subscription, resourceGroup, vnetID, subnetID)
}

// observeMetrics observes the IP pool and updates the metrics. Blocking.
//
//nolint:lll // ignore line length
func observeMetrics(ctx context.Context, ipcli func() map[string]cns.IPConfigurationStatus, nnccli func(context.Context) (*v1alpha.NodeNetworkConfig, error), csscli func(context.Context) ([]v1alpha1.ClusterSubnetState, error)) error {
	csslist, err := csscli(ctx)
	if err != nil {
		return err
	}
	nnc, err := nnccli(ctx)
	if err != nil {
		return err
	}
	ips := ipcli()

	var meta metaState
	for i := range csslist {
		if csslist[i].Status.Exhausted {
			meta.exhausted = true
			break
		}
	}
	if len(nnc.Status.NetworkContainers) > 0 {
		// Set SubnetName, SubnetAddressSpace and Pod Network ARM ID values to the global subnet, subnetCIDR and subnetARM variables.
		meta.subnet = nnc.Status.NetworkContainers[0].SubnetName
		meta.subnetCIDR = nnc.Status.NetworkContainers[0].SubnetAddressSpace
		meta.subnetARMID = generateARMID(&nnc.Status.NetworkContainers[0])
	}
	meta.primaryIPAddresses = make(map[string]struct{})
	// Add Primary IP to Map, if not present.
	// This is only for Swift i.e. if NC Type is vnet.
	for i := 0; i < len(nnc.Status.NetworkContainers); i++ {
		nc := nnc.Status.NetworkContainers[i]
		if nc.Type == "" || nc.Type == v1alpha.VNET {
			meta.primaryIPAddresses[nc.PrimaryIP] = struct{}{}
		}

		if nc.Type == v1alpha.VNETBlock {
			primaryPrefix, err := netip.ParsePrefix(nc.PrimaryIP)
			if err != nil {
				return errors.Wrapf(err, "unable to parse ip prefix: %s", nc.PrimaryIP)
			}
			meta.primaryIPAddresses[primaryPrefix.Addr().String()] = struct{}{}
		}
	}

	state := ipPoolState{
		secondaryIPs: int64(len(ips)),
		requestedIPs: nnc.Spec.RequestedIPCount,
	}
	for i := range ips {
		ip := ips[i]
		switch ip.GetState() {
		case types.Assigned:
			state.allocatedToPods++
		case types.Available:
			state.available++
		case types.PendingProgramming:
			state.pendingProgramming++
		case types.PendingRelease:
			state.pendingRelease++
		}
	}
	state.currentAvailableIPs = state.secondaryIPs - state.allocatedToPods - state.pendingRelease
	state.expectedAvailableIPs = state.requestedIPs - state.allocatedToPods

	labels := []string{meta.subnet, meta.subnetCIDR, meta.subnetARMID}
	IpamAllocatedIPCount.WithLabelValues(labels...).Set(float64(state.allocatedToPods))
	IpamAvailableIPCount.WithLabelValues(labels...).Set(float64(state.available))
	IpamBatchSize.WithLabelValues(labels...).Set(float64(meta.batch))
	IpamCurrentAvailableIPcount.WithLabelValues(labels...).Set(float64(state.currentAvailableIPs))
	IpamExpectedAvailableIPCount.WithLabelValues(labels...).Set(float64(state.expectedAvailableIPs))
	IpamMaxIPCount.WithLabelValues(labels...).Set(float64(meta.max))
	IpamPendingProgramIPCount.WithLabelValues(labels...).Set(float64(state.pendingProgramming))
	IpamPendingReleaseIPCount.WithLabelValues(labels...).Set(float64(state.pendingRelease))
	IpamPrimaryIPCount.WithLabelValues(labels...).Set(float64(len(meta.primaryIPAddresses)))
	IpamRequestedIPConfigCount.WithLabelValues(labels...).Set(float64(state.requestedIPs))
	IpamSecondaryIPCount.WithLabelValues(labels...).Set(float64(state.secondaryIPs))
	IpamTotalIPCount.WithLabelValues(labels...).Set(float64(state.secondaryIPs + int64(len(meta.primaryIPAddresses))))
	if meta.exhausted {
		IpamSubnetExhaustionState.WithLabelValues(labels...).Set(float64(SubnetIPExhausted))
	} else {
		IpamSubnetExhaustionState.WithLabelValues(labels...).Set(float64(SubnetIPNotExhausted))
	}
	return nil
}
