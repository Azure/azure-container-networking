package nodenetworkconfig

import (
	"net/netip"
	"strconv"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
)

// createNCRequestFromStaticNCHelper generates a CreateNetworkContainerRequest from a static NetworkContainer.
// If the NC's DefaultGateway is empty, it will set the 0th IP as the gateway IP and all remaining IPs as
// secondary IPs. If the gateway is not empty, it will not reserve the 0th IP and add it as a secondary IP.
//
//nolint:gocritic //ignore hugeparam
func createNCRequestFromStaticNCHelper(nc v1alpha.NetworkContainer, primaryIPPrefix netip.Prefix, subnet cns.IPSubnet) *cns.CreateNetworkContainerRequest {
	secondaryIPConfigs := map[string]cns.SecondaryIPConfig{}

	// if NC DefaultGateway is empty, set the 0th IP to the gateway and add the rest of the IPs
	// as secondary IPs
	startingAddr := primaryIPPrefix.Masked().Addr() // the masked address is the 0th IP in the subnet
	if nc.DefaultGateway == "" {
		nc.DefaultGateway = startingAddr.String()
		startingAddr = startingAddr.Next()
	}

	// iterate through all IP addresses in the subnet described by primaryPrefix and
	// add them to the request as secondary IPConfigs.
	for addr := startingAddr; primaryIPPrefix.Contains(addr); addr = addr.Next() {
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
