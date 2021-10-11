package kubecontroller

import (
	"net"
	"strconv"

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

	ip := net.ParseIP(nc.PrimaryIP)
	if ip == nil {
		return cns.CreateNetworkContainerRequest{}, errors.Wrapf(ErrInvalidPrimaryIP, "IP: %s", nc.PrimaryIP)
	}

	_, ipNet, err := net.ParseCIDR(nc.SubnetAddressSpace)
	if err != nil {
		return cns.CreateNetworkContainerRequest{}, errors.Wrapf(err, "invalid SubnetAddressSpace %s", nc.SubnetAddressSpace)
	}

	size, _ := ipNet.Mask.Size()

	subnet := cns.IPSubnet{
		IPAddress:    ip.String(),
		PrefixLength: uint8(size),
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
