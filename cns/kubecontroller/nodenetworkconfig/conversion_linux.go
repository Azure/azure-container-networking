package nodenetworkconfig

import (
	"net/netip"
	"strconv"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/pkg/errors"
)

// createNCRequestFromStaticNCHelper generates a CreateNetworkContainerRequest from a static NetworkContainer
// by adding all IPs in the the block to the secondary IP configs list. It does not skip any IPs.
//
//nolint:gocritic //ignore hugeparam
func createNCRequestFromStaticNCHelper(nc v1alpha.NetworkContainer, primaryIPPrefix netip.Prefix, subnet cns.IPSubnet) (*cns.CreateNetworkContainerRequest, error) {
	secondaryIPConfigs := map[string]cns.SecondaryIPConfig{}

	// Todo: Segregate the NIC’s primary IP from the list of secondary IPs to ensure the primary IP is not assigned to pods
	// WorkItem: https://msazure.visualstudio.com/One/_workitems/edit/33460135
	// iterate through all IP addresses in the subnet described by primaryPrefix and
	// add them to the request as secondary IPConfigs.
	for addr := primaryIPPrefix.Masked().Addr(); primaryIPPrefix.Contains(addr); addr = addr.Next() {
		secondaryIPConfigs[addr.String()] = cns.SecondaryIPConfig{
			IPAddress: addr.String(),
			NCVersion: int(nc.Version),
		}
	}

	// Add IPs from CIDR block to the secondary IPConfigs
	if nc.Type == v1alpha.VNETBlock {

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
		HostPrimaryIP:        nc.NodeIP,
		SecondaryIPConfigs:   secondaryIPConfigs,
		NetworkContainerid:   nc.ID,
		NetworkContainerType: cns.Docker,
		Version:              strconv.FormatInt(nc.Version, 10), //nolint:gomnd // it's decimal
		IPConfiguration: cns.IPConfiguration{
			IPSubnet:           subnet,
			GatewayIPAddress:   nc.DefaultGateway,
			GatewayIPv6Address: nc.DefaultGatewayV6,
		},
		NCStatus: nc.Status,
		NetworkInterfaceInfo: cns.NetworkInterfaceInfo{
			MACAddress: nc.MacAddress,
		},
	}, nil
}
