package nodenetworkconfig

import (
	"fmt"
	"net/netip"
	"strconv"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/restserver"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/pkg/errors"
)

// createNCRequestFromStaticNCHelper generates a CreateNetworkContainerRequest from a static NetworkContainer
// by adding all IPs in the the block to the secondary IP configs list. It does not skip any IPs.
//
//nolint:gocritic //ignore hugeparam
func createNCRequestFromStaticNCHelper(nc v1alpha.NetworkContainer, primaryIPPrefix netip.Prefix, subnet cns.IPSubnet) (*cns.CreateNetworkContainerRequest, error) {
	secondaryIPConfigs := map[string]cns.SecondaryIPConfig{}
	ipFamilies := map[restserver.IPFamily]struct{}{}

	// iterate through all IP addresses in the subnet described by primaryPrefix and
	// add them to the request as secondary IPConfigs.
	// TODO: we need to pass in a pointer to an IPFamilies slice to this function and add the family of the IPs added in
	// Here we check the primary CIDR which is needed for overlay and VNET Block
	for addr := primaryIPPrefix.Masked().Addr(); primaryIPPrefix.Contains(addr); addr = addr.Next() {
		secondaryIPConfigs[addr.String()] = cns.SecondaryIPConfig{
			IPAddress: addr.String(),
			NCVersion: int(nc.Version),
		}

	}

	if primaryIPPrefix.Addr().Is4() {
		ipFamilies[restserver.IPv4Family] = struct{}{}
	} else {
		ipFamilies[restserver.IPv6Family] = struct{}{}
	}

	// Add IPs from CIDR block to the secondary IPConfigs
	if nc.Type == v1alpha.VNETBlock {

		for _, ipAssignment := range nc.IPAssignments {
			// Here we would need to check all other assigned CIDR Blocks that aren't the primary.
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
			if cidrPrefix.Addr().Is4() {
				ipFamilies[restserver.IPv4Family] = struct{}{}
			} else {
				ipFamilies[restserver.IPv6Family] = struct{}{}
			}
		}
	}

	fmt.Printf("IPFamilies on NC %+v and %+v", nc.ID, ipFamilies)

	return &cns.CreateNetworkContainerRequest{
		HostPrimaryIP:        nc.NodeIP,
		SecondaryIPConfigs:   secondaryIPConfigs,
		NetworkContainerid:   nc.ID,
		NetworkContainerType: cns.Docker,
		Version:              strconv.FormatInt(nc.Version, 10), //nolint:gomnd // it's decimal
		IPConfiguration: cns.IPConfiguration{
			IPSubnet:         subnet,
			GatewayIPAddress: nc.DefaultGateway,
		},
		NCStatus:   nc.Status,
		IPFamilies: ipFamilies,
	}, nil
}
