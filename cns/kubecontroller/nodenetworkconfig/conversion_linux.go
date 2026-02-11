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
func createNCRequestFromStaticNCHelper(nc v1alpha.NetworkContainer, primaryIPPrefix netip.Prefix, subnet cns.IPSubnet, isSwiftV2 bool) (*cns.CreateNetworkContainerRequest, error) {
	secondaryIPConfigs := map[string]cns.SecondaryIPConfig{}

	// iterate through all IP addresses in the subnet described by primaryPrefix and
	// add them to the request as secondary IPConfigs.
	// Process primary prefix IPs in all scenarios except when nc.Type is v1alpha.VNETBlock AND SwiftV2 is enabled
	if !(isSwiftV2 && nc.Type == v1alpha.VNETBlock) {
		for addr := primaryIPPrefix.Masked().Addr(); primaryIPPrefix.Contains(addr); addr = addr.Next() {
			secondaryIPConfigs[addr.String()] = cns.SecondaryIPConfig{
				IPAddress: addr.String(),
				NCVersion: int(nc.Version),
			}
		}
	}

	// hard code it for now, we can make it configurable if needed in the future.
	// This is to prevent generating too many IPConfigs when the CIDR block is large. For example, if the CIDR block is /64, it will generate 2^64 IPConfigs which is not practical.
	ipv6PrefixClamp := 120
	// Add IPs from CIDR block to the secondary IPConfigs
	if nc.Type == v1alpha.VNETBlock {

		for _, ipAssignment := range nc.IPAssignments {
			cidrPrefix, err := netip.ParsePrefix(ipAssignment.IP)
			if err != nil {
				return nil, errors.Wrapf(err, "invalid CIDR block: %s", ipAssignment.IP)
			}

			if cidrPrefix.Addr().Is4In6() && cidrPrefix.Bits() < ipv6PrefixClamp {
				cidrPrefix = netip.PrefixFrom(cidrPrefix.Masked().Addr(), ipv6PrefixClamp)
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
