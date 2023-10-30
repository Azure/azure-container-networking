package nodenetworkconfig

import (
	"net/netip"
	"strconv"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/pkg/errors"
)

// createNCRequestFromStaticNCHelper generates a CreateNetworkContainerRequest from a static NetworkContainer.
// If the NC's DefaultGateway is empty and nc type is overlay, it will set the 2nd IP (*.1) as the gateway IP and all remaining IPs as
// secondary IPs. If the gateway is not empty, it will not reserve the 2nd IP and add it as a secondary IP.
//
//nolint:gocritic //ignore hugeparam
func createNCRequestFromStaticNCHelper(nc v1alpha.NetworkContainer, primaryIPPrefix netip.Prefix, subnet cns.IPSubnet) (*cns.CreateNetworkContainerRequest, error) {
	secondaryIPConfigs := map[string]cns.SecondaryIPConfig{}
	// the masked address is the 0th IP in the subnet and startingAddr is the 2nd IP (*.1)
	startingAddr := primaryIPPrefix.Masked().Addr().Next()
	lastAddr := startingAddr
	// if NC DefaultGateway is empty, set the 2nd IP (*.1) to the gateway and add the rest of the IPs as secondary IPs
	if nc.DefaultGateway == "" && nc.Type == v1alpha.Overlay {
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

	if nc.Type == v1alpha.VNETBlock {
		// Add IPs from CIDR block to the secondary IPConfigs
		for _, ipAssignment := range nc.IPAssignments {
			cidrPrefix, err := netip.ParsePrefix(ipAssignment.IP)
			if err != nil {
				return nil, errors.Wrapf(err, "invalid CIDR block: %s", ipAssignment.IP)
			}

			// iterate through all IP addresses in the CIDR block described by cidrPrefix and
			// add them to the request as secondary IPConfigs.
			for addr := cidrPrefix.Masked().Addr(); cidrPrefix.Contains(addr); addr = addr.Next() {
				secondaryIPConfigs[addr.String()] = cns.SecondaryIPConfig{
					IPAddress: addr.String(),
					NCVersion: int(nc.Version),
				}
				lastAddr = addr
			}
		}
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
	}, nil
}
