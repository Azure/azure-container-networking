package nodenetworkconfig

import (
	"net/netip"
	"strconv"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
)

// createNCRequestFromStaticNCHelper generates a CreateNetworkContainerRequest from a static NetworkContainer.
// If the NC's DefaultGateway is empty and nc type is overlay, it will set the 2nd IP (*.1) as the gateway IP and all remaining IPs as
// secondary IPs. If the gateway is not empty, it will not reserve the 2nd IP and add it as a secondary IP.
//
//nolint:gocritic //ignore hugeparam
func createNCRequestFromStaticNCHelper(nc v1alpha.NetworkContainer, primaryIPPrefix netip.Prefix, subnet cns.IPSubnet) *cns.CreateNetworkContainerRequest {
	secondaryIPConfigs := map[string]cns.SecondaryIPConfig{}
	// the masked address is the 0th IP in the subnet and startingAddr is the 2nd IP (*.1)
	startingAddr := primaryIPPrefix.Masked().Addr().Next()
	lastAddr := startingAddr
	// if NC DefaultGateway is empty, set the 2nd IP (*.1) to the gateway and add the rest of the IPs as secondary IPs
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
		lastAddr = addr
	}
	delete(secondaryIPConfigs, lastAddr.String())

	return &cns.CreateNetworkContainerRequest{
		SecondaryIPConfigs:   secondaryIPConfigs,
		NetworkContainerid:   nc.ID,
		NetworkContainerType: cns.Docker,
		Version:              strconv.FormatInt(nc.Version, 10), //nolint:gomnd // it's decimal
		IPConfiguration: cns.IPConfiguration{
			IPSubnet:         subnet,
			GatewayIPAddress: nc.DefaultGateway,
		},
		NCStatus: nc.Status,
	}
}
