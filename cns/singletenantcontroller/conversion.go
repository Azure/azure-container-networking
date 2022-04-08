package kubecontroller

import (
	"net"
	"net/netip" //nolint:gci // netip breaks gci??
	"strconv"
	"strings"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/restserver"
	cnstypes "github.com/Azure/azure-container-networking/cns/types"
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

type cnsClient interface {
	CreateOrUpdateNetworkContainerInternal(*cns.CreateNetworkContainerRequest) cnstypes.ResponseCode
}

var _ nodeNetworkConfigListener = (NodeNetworkConfigListenerFunc)(nil) //nolint:gocritic // clarity

type NodeNetworkConfigListenerFunc func(*v1alpha.NodeNetworkConfig) error

func (f NodeNetworkConfigListenerFunc) Update(nnc *v1alpha.NodeNetworkConfig) error {
	return f(nnc)
}

// SwiftNodeNetworkConfigListener return a function which satisfies the NodeNetworkConfigListener
// interface. It accepts a CreateOrUpdateNetworkContainerInternal implementation, and when Update
// is called, transforms the dynamic NNC in to an NC Request and calls the CNS Service implementation with
// that request.
func SwiftNodeNetworkConfigListener(cnscli cnsClient) NodeNetworkConfigListenerFunc {
	return func(nnc *v1alpha.NodeNetworkConfig) error {
		// Create NC request and hand it off to CNS
		ncRequest, err := CreateNCRequestFromDynamicNNC(nnc)
		if err != nil {
			return errors.Wrap(err, "failed to convert NNC status to network container request")
		}
		responseCode := cnscli.CreateOrUpdateNetworkContainerInternal(&ncRequest)
		err = restserver.ResponseCodeToError(responseCode)
		if err != nil {
			logger.Errorf("[cns-rc] Error creating or updating NC in reconcile: %v", err)
			return errors.Wrap(err, "failed to create or update network container")
		}

		// record assigned IPs metric
		allocatedIPs.Set(float64(len(nnc.Status.NetworkContainers[0].IPAssignments)))
		return nil
	}
}

// CreateNCRequestFromDynamicNNC translates a crd status to createnetworkcontainer request for dynamic NNC (swift)
func CreateNCRequestFromDynamicNNC(nnc *v1alpha.NodeNetworkConfig) (cns.CreateNetworkContainerRequest, error) {
	// if NNC has no NC, return an empty request
	if len(nnc.Status.NetworkContainers) == 0 {
		return cns.CreateNetworkContainerRequest{}, nil
	}

	// only support a single NC per node, error on more
	if len(nnc.Status.NetworkContainers) > 1 {
		return cns.CreateNetworkContainerRequest{}, errors.Wrapf(ErrUnsupportedNCQuantity, "count: %d", len(nnc.Status.NetworkContainers))
	}

	nc := nnc.Status.NetworkContainers[0]

	primaryIP := nc.PrimaryIP
	// if the PrimaryIP is not a CIDR, append a /32
	if !strings.Contains(primaryIP, "/") {
		primaryIP += "/32"
	}

	primaryPrefix, err := netip.ParsePrefix(primaryIP)
	if err != nil {
		return cns.CreateNetworkContainerRequest{}, errors.Wrapf(err, "IP: %s", primaryIP)
	}

	subnetPrefix, err := netip.ParsePrefix(nc.SubnetAddressSpace)
	if err != nil {
		return cns.CreateNetworkContainerRequest{}, errors.Wrapf(err, "invalid SubnetAddressSpace %s", nc.SubnetAddressSpace)
	}

	subnet := cns.IPSubnet{
		IPAddress:    primaryPrefix.Addr().String(),
		PrefixLength: uint8(subnetPrefix.Bits()),
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
		Version:              strconv.FormatInt(nc.Version, 10), //nolint:gomnd // it's decimal
		IPConfiguration: cns.IPConfiguration{
			IPSubnet:         subnet,
			GatewayIPAddress: nc.DefaultGateway,
		},
	}, nil
}

// OverlayNodeNetworkConfigListener returns a function which satisfies the NodeNetworkConfigListener
// interface. It accepts a CreateOrUpdateNetworkContainerInternal implementation, and when Update
// is called, transforms the static NNC in to an NC Request and calls the CNS Service implementation with
// that request.
func OverlayNodeNetworkConfigListener(cnscli cnsClient) NodeNetworkConfigListenerFunc {
	return func(nnc *v1alpha.NodeNetworkConfig) error {
		// Create NC request and hand it off to CNS
		ncRequest, err := CreateNCRequestFromDynamicNNC(nnc)
		if err != nil {
			return errors.Wrap(err, "failed to convert NNC status to network container request")
		}
		responseCode := cnscli.CreateOrUpdateNetworkContainerInternal(&ncRequest)
		err = restserver.ResponseCodeToError(responseCode)
		if err != nil {
			logger.Errorf("[cns-rc] Error creating or updating NC in reconcile: %v", err)
			return errors.Wrap(err, "failed to create or update network container")
		}

		// record assigned IPs metric
		allocatedIPs.Set(float64(len(nnc.Status.NetworkContainers[0].IPAssignments)))
		return nil
	}
}

// CreateNCRequestFromStaticNNC translates a crd status to createnetworkcontainer request for static NNC (overlay)
func CreateNCRequestFromStaticNNC(nnc *v1alpha.NodeNetworkConfig) (cns.CreateNetworkContainerRequest, error) {
	// if NNC has no NC, return an empty request
	if len(nnc.Status.NetworkContainers) == 0 {
		return cns.CreateNetworkContainerRequest{}, nil
	}

	// only support a single NC per node, error on more
	if len(nnc.Status.NetworkContainers) > 1 {
		return cns.CreateNetworkContainerRequest{}, errors.Wrapf(ErrUnsupportedNCQuantity, "count: %d", len(nnc.Status.NetworkContainers))
	}

	nc := nnc.Status.NetworkContainers[0]

	primaryPrefix, err := netip.ParsePrefix(nc.PrimaryIP)
	if err != nil {
		return cns.CreateNetworkContainerRequest{}, errors.Wrapf(err, "IP: %s", nc.PrimaryIP)
	}

	subnetPrefix, err := netip.ParsePrefix(nc.SubnetAddressSpace)
	if err != nil {
		return cns.CreateNetworkContainerRequest{}, errors.Wrapf(err, "invalid SubnetAddressSpace %s", nc.SubnetAddressSpace)
	}

	subnet := cns.IPSubnet{
		IPAddress:    primaryPrefix.Addr().String(),
		PrefixLength: uint8(subnetPrefix.Bits()),
	}

	secondaryIPConfigs := map[string]cns.SecondaryIPConfig{}

	// iterate through all IP addresses in the subnet described by primaryPrefix and
	// add them to the request as secondary IPConfigs. Skip the specific IP of the
	// primaryPrefix; that is the gateway.
	zeroAddr := primaryPrefix.Masked().Addr() // the masked address is the 0th IP in the subnet
	for addr := zeroAddr.Next(); primaryPrefix.Contains(addr); addr = addr.Next() {
		if addr == primaryPrefix.Addr() {
			continue
		}
		secondaryIPConfigs[addr.String()] = cns.SecondaryIPConfig{
			IPAddress: addr.String(),
			NCVersion: int(nc.Version),
		}
	}
	return cns.CreateNetworkContainerRequest{
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
