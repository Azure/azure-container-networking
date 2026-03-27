package nodenetworkconfig

import (
	"net"
	"net/netip" //nolint:gci // netip breaks gci??
	"strconv"
	"strings"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/pkg/errors"
)

var (
	// ErrInvalidPrimaryIP indicates the NC primary IP is invalid.
	ErrInvalidPrimaryIP = errors.New("invalid primary IP")
	// ErrInvalidSecondaryIP indicates that a secondary IP on the NC is invalid.
	ErrInvalidSecondaryIP = errors.New("invalid secondary IP")
	// ErrUnsupportedNCQuantity indicates that the node has an unsupported nummber of Network Containers attached.
	ErrUnsupportedNCQuantity = errors.New("unsupported number of network containers")
)

// parseIPv6Subnet validates and builds an IPSubnet from the NC's IPv6 fields.
// Returns a zero-value IPSubnet if SubnetAddressSpaceV6 is empty.
//
//nolint:gocritic //ignore hugeparam
func parseIPv6Subnet(nc v1alpha.NetworkContainer) (cns.IPSubnet, error) {
	if nc.SubnetAddressSpaceV6 == "" {
		return cns.IPSubnet{}, nil
	}
	subnetV6Prefix, err := netip.ParsePrefix(nc.SubnetAddressSpaceV6)
	if err != nil {
		return cns.IPSubnet{}, errors.Wrapf(err, "invalid SubnetAddressSpaceV6 %s", nc.SubnetAddressSpaceV6)
	}
	if !subnetV6Prefix.Addr().Is6() {
		return cns.IPSubnet{}, errors.Errorf("SubnetAddressSpaceV6 %s is not an IPv6 prefix", nc.SubnetAddressSpaceV6)
	}
	if nc.PrimaryIPV6 == "" {
		return cns.IPSubnet{}, errors.New("PrimaryIPV6 must be set when SubnetAddressSpaceV6 is specified")
	}
	return cns.IPSubnet{
		IPAddress:    nc.PrimaryIPV6,
		PrefixLength: uint8(subnetV6Prefix.Bits()), //#nosec G115 -- prefix bits are 0-128, fits uint8
	}, nil
}

// CreateNCRequestFromDynamicNC generates a CreateNetworkContainerRequest from a dynamic NetworkContainer.
//
//nolint:gocritic //ignore hugeparam
func CreateNCRequestFromDynamicNC(nc v1alpha.NetworkContainer) (*cns.CreateNetworkContainerRequest, error) {
	primaryIP := nc.PrimaryIP
	// if the PrimaryIP is not a CIDR, append a /32
	if !strings.Contains(primaryIP, "/") {
		primaryIP += "/32"
	}

	primaryPrefix, err := netip.ParsePrefix(primaryIP)
	if err != nil {
		return nil, errors.Wrapf(err, "IP: %s", primaryIP)
	}

	subnetPrefix, err := netip.ParsePrefix(nc.SubnetAddressSpace)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid SubnetAddressSpace %s", nc.SubnetAddressSpace)
	}

	subnet := cns.IPSubnet{
		IPAddress:    primaryPrefix.Addr().String(),
		PrefixLength: uint8(subnetPrefix.Bits()),
	}

	secondaryIPConfigs := map[string]cns.SecondaryIPConfig{}
	for _, ipAssignment := range nc.IPAssignments {
		secondaryIP := net.ParseIP(ipAssignment.IP)
		if secondaryIP == nil {
			return nil, errors.Wrapf(ErrInvalidSecondaryIP, "IP: %s", ipAssignment.IP)
		}
		secondaryIPConfigs[ipAssignment.Name] = cns.SecondaryIPConfig{
			IPAddress: secondaryIP.String(),
			NCVersion: int(nc.Version),
		}
	}
	ipConfig := cns.IPConfiguration{
		IPSubnet:         subnet,
		GatewayIPAddress: nc.DefaultGateway,
	}

	ipConfig.IPSubnetV6, err = parseIPv6Subnet(nc)
	if err != nil {
		return nil, err
	}

	return &cns.CreateNetworkContainerRequest{
		HostPrimaryIP:        nc.NodeIP,
		SecondaryIPConfigs:   secondaryIPConfigs,
		NetworkContainerid:   nc.ID,
		NetworkContainerType: cns.Docker,
		Version:              strconv.FormatInt(nc.Version, 10), //nolint:gomnd // it's decimal
		IPConfiguration:      ipConfig,
		NCStatus:             nc.Status,
	}, nil
}

// CreateNCRequestFromStaticNC generates a CreateNetworkContainerRequest from a static NetworkContainer.
// ipv6PrefixClamp caps IPv6 CIDR blocks to the given prefix length to prevent generating too many IPs.
//
//nolint:gocritic //ignore hugeparam
func CreateNCRequestFromStaticNC(nc v1alpha.NetworkContainer, isSwiftV2 bool, ipv6PrefixClamp int) (*cns.CreateNetworkContainerRequest, error) {
	if nc.Type == v1alpha.Overlay {
		nc.Version = 0 // fix for NMA always giving us version 0 for Overlay NCs
	}

	primaryPrefix, err := netip.ParsePrefix(nc.PrimaryIP)
	if err != nil {
		return nil, errors.Wrapf(err, "IP: %s", nc.PrimaryIP)
	}

	subnetPrefix, err := netip.ParsePrefix(nc.SubnetAddressSpace)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid SubnetAddressSpace %s", nc.SubnetAddressSpace)
	}

	subnet := cns.IPSubnet{
		PrefixLength: uint8(subnetPrefix.Bits()),
	}
	if nc.Type == v1alpha.VNETBlock {
		subnet.IPAddress = nc.NodeIP
	} else {
		subnet.IPAddress = primaryPrefix.Addr().String()
	}

	subnetV6, err := parseIPv6Subnet(nc)
	if err != nil {
		return nil, err
	}

	req, err := createNCRequestFromStaticNCHelper(nc, primaryPrefix, subnet, subnetV6, isSwiftV2, ipv6PrefixClamp)
	if err != nil {
		return nil, errors.Wrapf(err, "error while creating NC request from static NC")
	}

	return req, err
}
