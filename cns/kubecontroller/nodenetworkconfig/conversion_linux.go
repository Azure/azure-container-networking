package nodenetworkconfig

import (
	"net/netip"
	"strconv"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
)

// createNCRequestFromStaticNCHelper generates a CreateNetworkContainerRequest from a static NetworkContainer
// by adding all IPs in the the block to the secondary IP configs list. It does not skip any IPs.
//
//nolint:gocritic //ignore hugeparam
func createNCRequestFromStaticNCHelper(nc v1alpha.NetworkContainer, primaryIPPrefix netip.Prefix, subnet cns.IPSubnet) *cns.CreateNetworkContainerRequest {
	secondaryIPConfigs := map[string]cns.SecondaryIPConfig{}

	// iterate through all IP addresses in the subnet described by primaryPrefix and
	// add them to the request as secondary IPConfigs.
	for addr := primaryIPPrefix.Masked().Addr(); primaryIPPrefix.Contains(addr); addr = addr.Next() {
		secondaryIPConfigs[addr.String()] = cns.SecondaryIPConfig{
			IPAddress: addr.String(),
			NCVersion: int(nc.Version),
		}
	}
	return &cns.CreateNetworkContainerRequest{
		SecondaryIPConfigs:   secondaryIPConfigs,
		NetworkContainerid:   nc.ID,
		NetworkContainerType: cns.Docker,
		Version:              strconv.FormatInt(nc.Version, 10), //nolint:gomnd // it's decimal
		IPConfiguration: cns.IPConfiguration{
			IPSubnet:         subnet,
			GatewayIPAddress: nc.DefaultGateway,
		},
	}
}
