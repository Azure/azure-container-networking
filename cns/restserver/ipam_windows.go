package restserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"strings"

	"github.com/Azure/azure-container-networking/aitelemetry"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/hnsclient"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Microsoft/hcsshim/hcn"
)

type HNSClient interface {
	ListEndpoints() ([]hcn.HostComputeEndpoint, error)
	ListNetworks() ([]hcn.HostComputeNetwork, error)
	DeleteEndpointByID(endpointID string) error
	DeleteNetworkByID(networkID string) error
}

type hnsClientImpl struct{}

func (hnsClientImpl) ListEndpoints() ([]hcn.HostComputeEndpoint, error) {
	endpoints, err := hcn.ListEndpoints()
	if err != nil {
		return nil, fmt.Errorf("list HNS endpoints: %w", err)
	}
	return endpoints, nil
}

func (hnsClientImpl) ListNetworks() ([]hcn.HostComputeNetwork, error) {
	networks, err := hcn.ListNetworks()
	if err != nil {
		return nil, fmt.Errorf("list HNS networks: %w", err)
	}
	return networks, nil
}

func (hnsClientImpl) DeleteEndpointByID(endpointID string) error {
	if err := hnsclient.DeleteHNSEndpointbyID(endpointID); err != nil {
		return fmt.Errorf("delete HNS endpoint %s: %w", endpointID, err)
	}
	return nil
}

func (hnsClientImpl) DeleteNetworkByID(networkID string) error {
	if err := hnsclient.DeleteNetworkByIDHnsV2(networkID); err != nil {
		return fmt.Errorf("delete HNS network %s: %w", networkID, err)
	}
	return nil
}

var defaultHNSClient HNSClient = hnsClientImpl{} //nolint:gochecknoglobals // swapped in tests

// cleanupStaleHNSResources removes HNS resources leaked by a previous NC whose
// cleanup never reached this node (eg. no CNI DEL), before the incoming NC reuses
// its IPs or delegated NIC. SwiftV2 NCs with managed endpoint state are cleaned
// from EndpointState; all other NCs are cleaned by scanning HNS.
// HNS calls can be slow, so the service lock is only held to read or update CNS
// state, never across an HNS call. The caller serializes creates of the same NC.
func (service *HTTPRestService) cleanupStaleHNSResources(req cns.CreateNetworkContainerRequest) error {
	existingContainerDetails, exists := service.getNetworkContainerDetails(req.NetworkContainerid)
	mac := req.NetworkInterfaceInfo.MACAddress
	hasDelegatedNIC := mac != "" && isDelegatedNICType(req.NetworkInterfaceInfo.NICType)
	if hasDelegatedNIC && service.Options[common.OptManageEndpointState] == true {
		// Skip when the same NC+MAC is already in NC state - this is a repeat CreateNC, not stale.
		if exists && normalizeMAC(existingContainerDetails.CreateNetworkContainerRequest.NetworkInterfaceInfo.MACAddress) == normalizeMAC(mac) {
			return nil
		}
		return service.cleanupStaleSwiftV2Resources(req.NetworkContainerid, mac, req.LocalIPConfiguration.IPSubnet.IPAddress)
	}
	return service.cleanupStaleSwiftV1Endpoints(req, !exists)
}

func isDelegatedNICType(nicType cns.NICType) bool {
	return nicType == cns.DelegatedVMNIC || nicType == cns.NodeNetworkInterfaceFrontendNIC //nolint:staticcheck // both NIC types are delegated
}

// cleanupStaleSwiftV1Endpoints deletes HNS endpoints leaked by deleted NCs that
// still hold the incoming NC's IPs. DNC only reassigns an IP after the previous
// NC is deleted, so another holder means that NC's cleanup never reached this node.
func (service *HTTPRestService) cleanupStaleSwiftV1Endpoints(req cns.CreateNetworkContainerRequest, isNewNC bool) (returnErr error) {
	ncID := req.NetworkContainerid
	ncIP := req.IPConfiguration.IPSubnet.IPAddress
	// Dual-stack NCs get an IPv6 address on the same endpoint, assigned independently of the IPv4 one.
	ncIPv6 := req.IPv6Configuration.IPSubnet.IPAddress
	var apipaIP string
	if isApipaEnabled(&req) {
		apipaIP = req.LocalIPConfiguration.IPSubnet.IPAddress
	}

	var deletedEndpointIDs []string
	defer func() {
		if len(deletedEndpointIDs) > 0 || returnErr != nil {
			sendStaleHNSCleanupMetric(map[string]string{
				"ncID":           ncID,
				"apipaIP":        apipaIP,
				"ncIP":           ncIP,
				"ncIPv6":         ncIPv6,
				"hnsEndpointIDs": strings.Join(deletedEndpointIDs, ","),
			}, returnErr)
		}
	}()

	// The NC's own sandbox endpoint also holds the NC IP, so only an NC that is
	// new to this node proves another holder of its IP has leaked.
	var customerNetworks map[string]bool
	if isNewNC {
		networks, err := customerNetworkIDs(req)
		if err != nil {
			return err
		}
		customerNetworks = networks
	}
	if apipaIP == "" && len(customerNetworks) == 0 {
		return nil
	}

	endpoints, err := defaultHNSClient.ListEndpoints()
	if err != nil {
		return err //nolint:wrapcheck // wrapped by the client
	}
	apipaPrefix := hnsclient.GetHostNCApipaEndpointName("")
	ownApipaName := hnsclient.GetHostNCApipaEndpointName(ncID)
	var errs []error
	for i := range endpoints {
		ep := &endpoints[i]
		// All APIPA endpoints on a node share one HNS network, so the IP alone identifies the collision.
		staleApipa := apipaIP != "" && strings.HasPrefix(ep.Name, apipaPrefix) && ep.Name != ownApipaName && endpointHoldsIP(ep, apipaIP)
		// NC IPs are only unique within the customer's VNet, so only look inside its HNS network.
		staleNCIP := customerNetworks[ep.HostComputeNetwork] && (endpointHoldsIP(ep, ncIP) || endpointHoldsIP(ep, ncIPv6))
		if !staleApipa && !staleNCIP {
			continue
		}
		logger.Printf("[Azure CNS] deleting stale endpoint %s (%s) for NC %s", ep.Name, ep.Id, ncID) //nolint:staticcheck // TODO: migrate to zap logger
		if delErr := defaultHNSClient.DeleteEndpointByID(ep.Id); delErr != nil {
			errs = append(errs, delErr)
			continue
		}
		deletedEndpointIDs = append(deletedEndpointIDs, ep.Id)
	}
	return errors.Join(errs...)
}

// customerNetworkIDs returns the IDs of the HNS networks CNI created for the NC's
// customer subnet: a subnet equal to the NC's, tagged with the NC's VLAN. NCs
// without VLAN multitenancy or a valid IP configuration have none.
func customerNetworkIDs(req cns.CreateNetworkContainerRequest) (map[string]bool, error) {
	vlanID := req.MultiTenancyInfo.ID
	ipSubnet := req.IPConfiguration.IPSubnet
	subnet, err := netip.ParsePrefix(fmt.Sprintf("%s/%d", ipSubnet.IPAddress, ipSubnet.PrefixLength))
	if req.MultiTenancyInfo.EncapType != cns.Vlan || vlanID <= 0 || err != nil {
		return map[string]bool{}, nil //nolint:nilerr // NCs without a valid IP configuration have no customer network
	}
	subnet = subnet.Masked()

	networks, err := defaultHNSClient.ListNetworks()
	if err != nil {
		return nil, err //nolint:wrapcheck // wrapped by the client
	}
	ids := make(map[string]bool)
	for i := range networks {
		if isCustomerNetwork(&networks[i], vlanID, subnet) {
			ids[networks[i].Id] = true
		}
	}
	return ids, nil
}

// isCustomerNetwork returns whether a network has a subnet matching subnet and tagged with vlanID.
func isCustomerNetwork(nw *hcn.HostComputeNetwork, vlanID int, subnet netip.Prefix) bool {
	for _, ipam := range nw.Ipams {
		for _, s := range ipam.Subnets {
			prefix, err := netip.ParsePrefix(s.IpAddressPrefix)
			if err == nil && prefix.Masked() == subnet && subnetHasVLAN(s, vlanID) {
				return true
			}
		}
	}
	return false
}

// subnetHasVLAN returns whether a subnet has a VLAN policy with the given vlan ID.
func subnetHasVLAN(s hcn.Subnet, vlanID int) bool {
	for _, raw := range s.Policies {
		var policy hcn.SubnetPolicy
		var vlan hcn.VlanPolicySetting
		if json.Unmarshal(raw, &policy) == nil && policy.Type == hcn.VLAN &&
			json.Unmarshal(policy.Settings, &vlan) == nil && int(vlan.IsolationId) == vlanID {
			return true
		}
	}
	return false
}

// endpointHoldsIP returns whether an endpoint has an IP configuration with the given IP address.
// Addresses are parsed so IPv6 matches regardless of formatting; an invalid or empty ip matches nothing.
func endpointHoldsIP(ep *hcn.HostComputeEndpoint, ip string) bool {
	want, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	return slices.ContainsFunc(ep.IpConfigurations, func(c hcn.IpConfig) bool {
		got, err := netip.ParseAddr(c.IpAddress)
		return err == nil && got == want
	})
}

func normalizeMAC(mac string) string {
	return strings.ToLower(strings.ReplaceAll(mac, "-", ":"))
}

// staleSwiftV2Container is a container in EndpointState left behind by a previous
// NC, with the HNS objects CNI created for it.
type staleSwiftV2Container struct {
	containerID     string
	apipaEndpointID string
	endpointID      string
	networkID       string
}

// findStaleSwiftV2Containers returns the containers in EndpointState that still
// hold the incoming NC's delegated NIC MAC, or its APIPA IP for another NC.
// Caller must hold the service lock.
func (service *HTTPRestService) findStaleSwiftV2Containers(ncID, mac, apipaIP string) []staleSwiftV2Container {
	targetApipaIP := net.ParseIP(apipaIP)
	var staleContainers []staleSwiftV2Container
	for containerID, epInfo := range service.EndpointState {
		container := staleSwiftV2Container{containerID: containerID}
		holdsMAC, holdsApipaIP := false, false
		for _, info := range epInfo.IfnameToIPMap {
			switch {
			case info.NICType == cns.ApipaNIC:
				container.apipaEndpointID = info.HnsEndpointID
				holdsApipaIP = holdsApipaIP || (info.NetworkContainerID != ncID &&
					slices.ContainsFunc(info.IPv4, func(ipNet net.IPNet) bool { return ipNet.IP.Equal(targetApipaIP) }))
			case isDelegatedNICType(info.NICType):
				container.endpointID, container.networkID = info.HnsEndpointID, info.HnsNetworkID
				holdsMAC = holdsMAC || normalizeMAC(info.MacAddress) == normalizeMAC(mac)
			}
		}
		if holdsMAC || holdsApipaIP {
			staleContainers = append(staleContainers, container)
		}
	}
	return staleContainers
}

// cleanupStaleSwiftV2Resources deletes the HNS objects and EndpointState of
// SwiftV2 containers left behind by a previous NC on the incoming NC's delegated
// NIC or APIPA IP.
func (service *HTTPRestService) cleanupStaleSwiftV2Resources(ncID, mac, apipaIP string) error {
	service.RLock()
	staleContainers := service.findStaleSwiftV2Containers(ncID, mac, apipaIP)
	service.RUnlock()
	if len(staleContainers) == 0 {
		return nil
	}

	var cleanedContainerIDs []string
	var errs []error
	for _, container := range staleContainers {
		if err := service.cleanupSwiftV2Container(container); err != nil {
			errs = append(errs, err)
			continue
		}
		cleanedContainerIDs = append(cleanedContainerIDs, container.containerID)
	}

	err := errors.Join(errs...)
	sendStaleHNSCleanupMetric(map[string]string{
		"ncID":         ncID,
		"mac":          mac,
		"apipaIP":      apipaIP,
		"containerIDs": strings.Join(cleanedContainerIDs, ","),
	}, err)
	return err
}

// cleanupSwiftV2Container deletes a container's APIPA endpoint, delegated NIC
// endpoint and network, then its EndpointState entry. On an HNS failure it keeps
// the entry, so the next NC create retries.
func (service *HTTPRestService) cleanupSwiftV2Container(container staleSwiftV2Container) error {
	logger.Printf("[Azure CNS] deleting stale HNS resources %+v", container) //nolint:staticcheck // TODO: migrate to zap logger
	for _, endpointID := range []string{container.apipaEndpointID, container.endpointID} {
		if endpointID == "" {
			continue
		}
		if err := defaultHNSClient.DeleteEndpointByID(endpointID); err != nil {
			return fmt.Errorf("container %s: %w", container.containerID, err)
		}
	}
	if container.networkID != "" {
		if err := defaultHNSClient.DeleteNetworkByID(container.networkID); err != nil {
			return fmt.Errorf("container %s: %w", container.containerID, err)
		}
	}

	service.Lock()
	err := service.deleteEndpointStateUntransacted(container.containerID)
	service.Unlock()
	// A concurrent CNI DEL may have already removed the container.
	if err != nil && !errors.Is(err, ErrEndpointStateNotFound) {
		return fmt.Errorf("remove container %s from EndpointState: %w", container.containerID, err)
	}
	return nil
}

func sendStaleHNSCleanupMetric(dimensions map[string]string, err error) {
	dimensions["result"], dimensions["errorMsg"] = "success", ""
	if err != nil {
		dimensions["result"], dimensions["errorMsg"] = "failure", err.Error()
	}
	logger.SendMetric(aitelemetry.Metric{ //nolint:staticcheck // TODO: migrate to zap logger
		Name:             logger.StaleHNSCleanupMetricStr,
		Value:            1.0,
		CustomDimensions: dimensions,
	})
}
