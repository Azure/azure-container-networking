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
	"golang.org/x/sync/errgroup"
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

type observer struct {
	ipSrc  func() map[string]cns.IPConfigurationStatus
	nncSrc func(context.Context) (*v1alpha.NodeNetworkConfig, error)
	cssSrc func(context.Context) ([]v1alpha1.ClusterSubnetState, error)
}

// NewLegacyMetricsObserver creates a closed functional scope which can be invoked to
// observe the legacy IPAM pool metrics.
//
//nolint:lll // ignore line length
func NewLegacyMetricsObserver(ipSrc func() map[string]cns.IPConfigurationStatus, nncSrc func(context.Context) (*v1alpha.NodeNetworkConfig, error), cssSrc func(context.Context) ([]v1alpha1.ClusterSubnetState, error)) func(context.Context) error {
	return (&observer{
		ipSrc:  ipSrc,
		nncSrc: nncSrc,
		cssSrc: cssSrc,
	}).observeMetrics
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
func (o *observer) observeMetrics(ctx context.Context) error {
	// The error group is used to allow individual metrics sources to fail without
	// failing out the entire attempt to observe the Pool. This may happen if there is a
	// transient issue with the source of the data, or if the source is not available
	// (like if the CRD is not installed).
	var g errgroup.Group

	// Get the current state of world.
	var meta metaState
	g.Go(func() error {
		// Try to fetch the ClusterSubnetState, if available.
		if o.cssSrc != nil {
			csslist, err := o.cssSrc(ctx)
			if err != nil {
				return err
			}
			for i := range csslist {
				if csslist[i].Status.Exhausted {
					meta.exhausted = true
					break
				}
			}
		}
		return nil
	})

	var state ipPoolState
	g.Go(func() error {
		// Try to fetch the NodeNetworkConfig, if available.
		if o.nncSrc != nil {
			nnc, err := o.nncSrc(ctx)
			if err != nil {
				return err
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
			state.requestedIPs = nnc.Spec.RequestedIPCount
			meta.batch = nnc.Status.Scaler.BatchSize
			meta.max = nnc.Status.Scaler.MaxIPCount
		}
		return nil
	})

	g.Go(func() error {
		// Try to fetch the IPConfigurations, if available.
		if o.ipSrc != nil {
			ips := o.ipSrc()
			state.secondaryIPs = int64(len(ips))
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
		}
		return nil
	})

	err := g.Wait()

	state.currentAvailableIPs = state.secondaryIPs - state.allocatedToPods - state.pendingRelease
	state.expectedAvailableIPs = state.requestedIPs - state.allocatedToPods

	// Update the metrics.
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
	if err != nil {
		return errors.Wrap(err, "failed to collect all metrics")
	}
	return nil
}
