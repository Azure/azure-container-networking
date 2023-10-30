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
		NCStatus: nc.Status,
	}, nil
}

// CreateNCRequestFromStaticNC generates a CreateNetworkContainerRequest from a static NetworkContainer.
//
//nolint:gocritic //ignore hugeparam
func CreateNCRequestFromStaticNC(nc v1alpha.NetworkContainer) (*cns.CreateNetworkContainerRequest, error) {
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
		IPAddress:    primaryPrefix.Addr().String(),
		PrefixLength: uint8(subnetPrefix.Bits()),
	}

	req, err := createNCRequestFromStaticNCHelper(nc, primaryPrefix, subnet)
	if err != nil {
		return nil, errors.Wrapf(err, "error while creating NC request from static NC")
	}
	return req, err
}
