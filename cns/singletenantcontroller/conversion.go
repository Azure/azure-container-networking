package kubecontroller

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

// CRDStatusToNCRequest translates a crd status to createnetworkcontainer request
func CRDStatusToNCRequest(status *v1alpha.NodeNetworkConfigStatus) (cns.CreateNetworkContainerRequest, error) {
	// if NNC has no NC, return an empty request
	if len(status.NetworkContainers) == 0 {
		return cns.CreateNetworkContainerRequest{}, nil
	}

	// only support a single NC per node, error on more
	if len(status.NetworkContainers) > 1 {
		return cns.CreateNetworkContainerRequest{}, errors.Wrapf(ErrUnsupportedNCQuantity, "count: %d", len(status.NetworkContainers))
	}

	nc := status.NetworkContainers[0]

	primaryIP := nc.PrimaryIP
	// if the PrimaryIP is not a CIDR, append a /32
	if !strings.Contains(primaryIP, "/") {
		primaryIP += "/32"
	}

	primaryPrefix, err := netip.ParsePrefix(primaryIP)
	if err != nil {
		return cns.CreateNetworkContainerRequest{}, errors.Wrapf(err, "IP: %s", primaryIP)
	}

	secondaryPrefix, err := netip.ParsePrefix(nc.SubnetAddressSpace)
	if err != nil {
		return cns.CreateNetworkContainerRequest{}, errors.Wrapf(err, "invalid SubnetAddressSpace %s", nc.SubnetAddressSpace)
	}

	subnet := cns.IPSubnet{
		IPAddress:    primaryPrefix.Addr().String(),
		PrefixLength: uint8(secondaryPrefix.Bits()),
	}

	secondaryIPConfigs := map[string]cns.SecondaryIPConfig{}
	for _, ipAssignment := range nc.IPAssignments {
		secondaryIP := net.ParseIP(ipAssignment.IP)
		if secondaryIP == nil {
			return cns.CreateNetworkContainerRequest{}, errors.Wrapf(ErrInvalidSecondaryIP, "IP: %s", ipAssignment.IP)
		}
		secondaryIPConfigs[ipAssignment.Name] = cns.SecondaryIPConfig{
			IPAddress: secondaryIP.String(),
			NCVersion: int(nc.Version),
		}
	}
	return cns.CreateNetworkContainerRequest{
		SecondaryIPConfigs:   secondaryIPConfigs,
		NetworkContainerid:   nc.ID,
		NetworkContainerType: cns.Docker,
		Version:              strconv.FormatInt(nc.Version, 10),
		IPConfiguration: cns.IPConfiguration{
			IPSubnet:         subnet,
			GatewayIPAddress: nc.DefaultGateway,
		},
	}, nil
}
