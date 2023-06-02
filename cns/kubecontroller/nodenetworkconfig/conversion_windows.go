package nodenetworkconfig

import (
	"net/netip"
	"strconv"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/pkg/errors"
)

// createNCRequestFromStaticNCHelper generates a CreateNetworkContainerRequest from a static NetworkContainer.
// If the NC's DefaultGateway is empty, it will set the 0th IP as the gateway IP and all remaining IPs as
// secondary IPs. If the gateway is not empty, it will not reserve the 0th IP and add it as a secondary IP.
//
//nolint:gocritic //ignore hugeparam
func createNCRequestFromStaticNCHelper(nc v1alpha.NetworkContainer, primaryIPPrefix netip.Prefix, subnet cns.IPSubnet) (*cns.CreateNetworkContainerRequest, error) {
	secondaryIPConfigs := map[string]cns.SecondaryIPConfig{}

	// if NC DefaultGateway is empty, set the 0th IP to the gateway and add the rest of the IPs
	// as secondary IPs
	startingAddr := primaryIPPrefix.Masked().Addr() // the masked address is the 0th IP in the subnet
	if nc.DefaultGateway == "" && nc.Type == v1alpha.Overlay {
		// assign 0th IP to the default gateway
		nc.DefaultGateway = startingAddr.String()
		startingAddr = startingAddr.Next()
	} else if nc.Type == v1alpha.VNETBlock {
		// skipping 0th IP for the Primary IP of NC
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
			}
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
	}, nil
}
