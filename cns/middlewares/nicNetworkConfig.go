package middlewares

import (
	"context"
	"strings"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NICNetworkConfigMiddleware enriches NICResource data with NICNetworkConfig CRD information.
type NICNetworkConfigMiddleware struct {
	Cli client.Client
}

// GetNICNCInfoByMAC lists all NICNetworkConfig CRDs and returns a map keyed by MAC address.
func (m *NICNetworkConfigMiddleware) GetNICNCInfoByMAC(ctx context.Context) (map[string]*cns.NICNCInfo, error) {
	var nicNCList v1alpha1.NICNetworkConfigList
	if err := m.Cli.List(ctx, &nicNCList); err != nil {
		return nil, err
	}

	result := make(map[string]*cns.NICNCInfo, len(nicNCList.Items))
	for i := range nicNCList.Items {
		mac := nicNCList.Items[i].Status.MacAddress
		if mac == "" {
			continue
		}
		// Build the host-underlay primary IP as a CIDR using the *subnet*
		// width, not the NC width.
		//
		// Status.PrimaryIP is the NIC's primary CA but its prefix length is
		// the NC's prefix-on-NIC range (e.g., "165.0.0.16/28") — that's not
		// the actual VNet subnet width.
		//
		// For dranet to assign this address to the parent NIC on the host
		// and have the kernel install a connected route that covers *every*
		// pod CA in the customer subnet (including pods on other NICs in
		// the same subnet), we need the subnet width from
		// Status.SubnetAddressSpace (e.g., "165.0.0.0/20" → /20).
		//
		// Result: PrimaryIP = "165.0.0.16/20" — kernel-ready for `ip addr add`.
		primaryIP := buildSubnetWidthPrimaryIP(
			nicNCList.Items[i].Status.PrimaryIP,
			nicNCList.Items[i].Status.SubnetAddressSpace,
		)
		result[mac] = &cns.NICNCInfo{
			NetworkID: nicNCList.Items[i].Spec.NetworkID,
			SubnetID:  nicNCList.Items[i].Spec.SubnetID,
			PrimaryIP: primaryIP,
		}
	}

	logger.Printf("[NICNetworkConfigMiddleware] fetched %d NICNetworkConfigs, %d with MAC addresses", len(nicNCList.Items), len(result))
	return result, nil
}

// buildSubnetWidthPrimaryIP combines the IP portion of Status.PrimaryIP
// (which carries the NC's prefix-on-NIC range, e.g. "165.0.0.16/28") with
// the prefix length from Status.SubnetAddressSpace (the actual VNet subnet,
// e.g. "165.0.0.0/20") to produce a CIDR string suitable for `ip addr add`
// on the host parent NIC: "165.0.0.16/20".
//
// Returns "" if the IP cannot be extracted. If the subnet prefix is missing
// or unparseable, falls back to whatever prefix Status.PrimaryIP carried so
// callers still get a usable address (just with the narrower NC mask).
func buildSubnetWidthPrimaryIP(primaryIPCIDR, subnetAddressSpace string) string {
	if primaryIPCIDR == "" {
		return ""
	}
	ip := primaryIPCIDR
	if idx := strings.IndexByte(ip, '/'); idx > 0 {
		ip = ip[:idx]
	}
	if ip == "" {
		return ""
	}
	if idx := strings.IndexByte(subnetAddressSpace, '/'); idx > 0 && idx+1 < len(subnetAddressSpace) {
		return ip + subnetAddressSpace[idx:]
	}
	return primaryIPCIDR
}
