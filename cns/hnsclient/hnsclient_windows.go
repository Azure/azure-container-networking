package hnsclient

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/networkcontainers"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/network/policy"
	"github.com/Microsoft/hcsshim"
	"github.com/Microsoft/hcsshim/hcn"
)

const (
	// Name of the external hns network
	ExtHnsNetworkName = "ext"

	// Address prefix for external hns network
	ExtHnsNetworkAddressPrefix = "192.168.255.0/30"

	// Gateway address for external hns network
	ExtHnsNetworkGwAddress = "192.168.255.1"

	// HNS network types
	hnsL2Bridge = "l2bridge"
	hnsL2Tunnel = "l2tunnel"

	// hcnSchemaVersionMajor indicates major version number for hcn schema
	hcnSchemaVersionMajor = 2

	// hcnSchemaVersionMinor indicates minor version number for hcn schema
	hcnSchemaVersionMinor = 0

	// hcnIpamTypeStatic indicates the static type of ipam
	hcnIpamTypeStatic = "Static"

	// hostNCApipaNetworkName indicates the name of the apipa network used for host container connectivity
	hostNCApipaNetworkName = "HostNCApipaNetwork"

	// hostNCApipaNetworkType indicates the type of hns network set up for host NC connectivity
	hostNCApipaNetworkType = hcn.L2Bridge

	// hostNCApipaEndpointName indicates the prefix for the name of the apipa endpoint used for
	// the host container connectivity
	hostNCApipaEndpointNamePrefix = "HostNCApipaEndpoint"

	// Name of the loopback adapter needed to create Host NC apipa network
	hostNCLoopbackAdapterName = "LoopbackAdapterHostNCConnectivity"

	// protocolTCP indicates the TCP protocol identifier in HCN
	protocolTCP = "6"

	// protocolUDP indicates the UDP protocol identifier in HCN
	protocolUDP = "17"

	// protocolICMPv4 indicates the ICMPv4 protocol identifier in HCN
	protocolICMPv4 = "1"
)

var (
	// Named Lock for network and endpoint creation/deletion
	namedLock = common.InitNamedLock()
)

// CreateHnsNetwork creates the HNS network with the provided configuration
func CreateHnsNetwork(nwConfig cns.CreateHnsNetworkRequest) error {
	log.Printf("[Azure CNS] CreateHnsNetwork")
	// Initialize HNS network.
	hnsNetwork := &hcsshim.HNSNetwork{
		Name:                 nwConfig.NetworkName,
		Type:                 nwConfig.NetworkType,
		NetworkAdapterName:   nwConfig.NetworkAdapterName,
		SourceMac:            nwConfig.SourceMac,
		DNSSuffix:            nwConfig.DNSSuffix,
		DNSServerList:        nwConfig.DNSServerList,
		DNSServerCompartment: nwConfig.DNSServerCompartment,
		ManagementIP:         nwConfig.ManagementIP,
		AutomaticDNS:         nwConfig.AutomaticDNS,
	}

	for _, policy := range nwConfig.Policies {
		hnsNetwork.Policies = append(hnsNetwork.Policies, policy)
	}

	for _, subnet := range nwConfig.Subnets {
		hnsSubnet := hcsshim.Subnet{
			AddressPrefix:  subnet.AddressPrefix,
			GatewayAddress: subnet.GatewayAddress,
		}

		hnsNetwork.Subnets = append(hnsNetwork.Subnets, hnsSubnet)
	}

	for _, macPool := range nwConfig.MacPools {
		hnsMacPool := hcsshim.MacPool{
			StartMacAddress: macPool.StartMacAddress,
			EndMacAddress:   macPool.EndMacAddress,
		}
		hnsNetwork.MacPools = append(hnsNetwork.MacPools, hnsMacPool)
	}

	return createHnsNetwork(hnsNetwork)
}

// DeleteHnsNetwork deletes the HNS network with the provided name
func DeleteHnsNetwork(networkName string) error {
	log.Printf("[Azure CNS] DeleteHnsNetwork")

	return deleteHnsNetwork(networkName)
}

// CreateDefaultExtNetwork creates default HNS network named ext (if it doesn't exist already)
// to create external switch on windows platform.
// This allows orchestrators to start CNS which pre-provisions the network so that the
// VM network blip / disconnect is avoided when calling cni add for the very first time.
func CreateDefaultExtNetwork(networkType string) error {
	networkType = strings.ToLower(strings.TrimSpace(networkType))
	if len(networkType) == 0 {
		return nil
	}

	if networkType != hnsL2Bridge && networkType != hnsL2Tunnel {
		return fmt.Errorf("Invalid hns network type %s", networkType)
	}

	log.Printf("[Azure CNS] CreateDefaultExtNetwork")
	extHnsNetwork, _ := hcsshim.GetHNSNetworkByName(ExtHnsNetworkName)

	if extHnsNetwork != nil {
		log.Printf("[Azure CNS] Found existing DefaultExtNetwork with type: %s", extHnsNetwork.Type)
		if !strings.EqualFold(networkType, extHnsNetwork.Type) {
			return fmt.Errorf("Network type mismatch with existing network: %s", extHnsNetwork.Type)
		}

		return nil
	}

	// create new hns network
	log.Printf("[Azure CNS] Creating DefaultExtNetwork with type %s", networkType)

	hnsNetwork := &hcsshim.HNSNetwork{
		Name: ExtHnsNetworkName,
		Type: networkType,
	}

	hnsSubnet := hcsshim.Subnet{
		AddressPrefix:  ExtHnsNetworkAddressPrefix,
		GatewayAddress: ExtHnsNetworkGwAddress,
	}

	hnsNetwork.Subnets = append(hnsNetwork.Subnets, hnsSubnet)

	return createHnsNetwork(hnsNetwork)
}

// DeleteDefaultExtNetwork deletes the default HNS network
func DeleteDefaultExtNetwork() error {
	log.Printf("[Azure CNS] DeleteDefaultExtNetwork")

	return deleteHnsNetwork(ExtHnsNetworkName)
}

// createHnsNetwork calls the hcshim to create the hns network
func createHnsNetwork(hnsNetwork *hcsshim.HNSNetwork) error {
	// Marshal the request.
	buffer, err := json.Marshal(hnsNetwork)
	if err != nil {
		return err
	}
	hnsRequest := string(buffer)

	// Create the HNS network.
	log.Printf("[Azure CNS] HNSNetworkRequest POST request:%+v", hnsRequest)
	hnsResponse, err := hcsshim.HNSNetworkRequest("POST", "", hnsRequest)
	log.Printf("[Azure CNS] HNSNetworkRequest POST response:%+v err:%v.", hnsResponse, err)

	return err
}

// deleteHnsNetwork calls HNS to delete the network with the provided name
func deleteHnsNetwork(networkName string) error {
	hnsNetwork, err := hcsshim.GetHNSNetworkByName(networkName)
	if err == nil {
		// Delete the HNS network.
		var hnsResponse *hcsshim.HNSNetwork
		log.Printf("[Azure CNS] HNSNetworkRequest DELETE id:%v", hnsNetwork.Id)
		hnsResponse, err = hcsshim.HNSNetworkRequest("DELETE", hnsNetwork.Id, "")
		log.Printf("[Azure CNS] HNSNetworkRequest DELETE response:%+v err:%v.", hnsResponse, err)
	}

	return err
}

func configureHostNCApipaNetwork(localIPConfiguration cns.IPConfiguration) (*hcn.HostComputeNetwork, error) {
	network := &hcn.HostComputeNetwork{
		Name: hostNCApipaNetworkName,
		Ipams: []hcn.Ipam{
			hcn.Ipam{
				Type: hcnIpamTypeStatic,
			},
		},
		SchemaVersion: hcn.SchemaVersion{
			Major: hcnSchemaVersionMajor,
			Minor: hcnSchemaVersionMinor,
		},
		Type:  hostNCApipaNetworkType,
		Flags: hcn.EnableNonPersistent, // Set up the network in non-persistent mode
	}

	if netAdapterNamePolicy, err := policy.GetHcnNetAdapterPolicy(hostNCLoopbackAdapterName); err == nil {
		network.Policies = append(network.Policies, netAdapterNamePolicy)
	} else {
		return nil, fmt.Errorf("Failed to serialize network adapter policy. Error: %v", err)
	}

	// Calculate subnet prefix
	var (
		subnetPrefix    net.IPNet
		subnetPrefixStr string
		ipAddr          net.IP
	)

	ipAddr = net.ParseIP(localIPConfiguration.IPSubnet.IPAddress)
	if ipAddr.To4() != nil {
		subnetPrefix = net.IPNet{Mask: net.CIDRMask(int(localIPConfiguration.IPSubnet.PrefixLength), 32)}
	} else if ipAddr.To16() != nil {
		subnetPrefix = net.IPNet{Mask: net.CIDRMask(int(localIPConfiguration.IPSubnet.PrefixLength), 128)}
	} else {
		return nil, fmt.Errorf("Failed get subnet prefix for localIPConfiguration: %+v", localIPConfiguration)
	}

	subnetPrefix.IP = ipAddr.Mask(subnetPrefix.Mask)
	subnetPrefixStr = subnetPrefix.IP.String() + "/" + strconv.Itoa(int(localIPConfiguration.IPSubnet.PrefixLength))

	subnet := hcn.Subnet{
		IpAddressPrefix: subnetPrefixStr,
		Routes: []hcn.Route{
			hcn.Route{
				NextHop:           localIPConfiguration.GatewayIPAddress,
				DestinationPrefix: "0.0.0.0/0",
			},
		},
	}

	network.Ipams[0].Subnets = append(network.Ipams[0].Subnets, subnet)

	log.Printf("[Azure CNS] Configured HostNCApipaNetwork: %+v", network)

	return network, nil
}

func createHostNCApipaNetwork(
	localIPConfiguration cns.IPConfiguration) (*hcn.HostComputeNetwork, error) {
	var (
		network *hcn.HostComputeNetwork
		err     error
	)

	namedLock.LockAcquire(hostNCApipaNetworkName)
	defer namedLock.LockRelease(hostNCApipaNetworkName)

	// Check if the network exists for host NC connectivity
	if network, err = hcn.GetNetworkByName(hostNCApipaNetworkName); err != nil {
		// If error is anything other than networkNotFound, mark this as error
		// TODO: why is following part not working?
		/*
			if _, networkNotFound := err.(hcn.NetworkNotFoundError); !networkNotFound {
				return nil, fmt.Errorf("[Azure CNS] ERROR: createApipaNetwork failed. Error with GetNetworkByName: %v", err)
			}
		*/

		// Network doesn't exist. Create one.
		if network, err = configureHostNCApipaNetwork(localIPConfiguration); err != nil {
			return nil, fmt.Errorf("Failed to configure network. Error: %v", err)
		}

		// Create loopback adapter needed for this HNS network
		if networkExists, _ := networkcontainers.InterfaceExists(hostNCLoopbackAdapterName); !networkExists {
			ipconfig := cns.IPConfiguration{
				IPSubnet: cns.IPSubnet{
					IPAddress:    localIPConfiguration.GatewayIPAddress,
					PrefixLength: localIPConfiguration.IPSubnet.PrefixLength,
				},
				GatewayIPAddress: localIPConfiguration.GatewayIPAddress,
			}

			if err = networkcontainers.CreateLoopbackAdapter(
				hostNCLoopbackAdapterName,
				ipconfig,
				false, /* Flag to setWeakHostOnInterface */
				"" /* Empty primary Interface Identifier as setWeakHostOnInterface is not needed*/); err != nil {
				return nil, fmt.Errorf("Failed to create loopback adapter. Error: %v", err)
			}
		}

		// Create the HNS network.
		log.Printf("[Azure CNS] Creating HostNCApipaNetwork: %+v", network)

		if network, err = network.Create(); err != nil {
			return nil, err
		}

		log.Printf("[Azure CNS] Successfully created apipa network for host container connectivity: %+v", network)
	} else {
		log.Printf("[Azure CNS] Found existing HostNCApipaNetwork: %+v", network)
	}

	return network, err
}

func addAclToEndpointPolicy(
	aclPolicySetting hcn.AclPolicySetting,
	endpointPolicies []hcn.EndpointPolicy) error {
	var (
		rawJSON []byte
		err     error
	)

	if rawJSON, err = json.Marshal(aclPolicySetting); err != nil {
		return fmt.Errorf("Failed to marshal endpoint ACL: %+v", aclPolicySetting)
	}

	endpointPolicy := hcn.EndpointPolicy{
		Type:     hcn.ACL,
		Settings: rawJSON,
	}

	endpointPolicies = append(endpointPolicies, endpointPolicy)

	return nil
}

func configureHostNCApipaEndpoint(
	endpointName string,
	networkID string,
	localIPConfiguration cns.IPConfiguration) (*hcn.HostComputeEndpoint, error) {
	var err error
	endpoint := &hcn.HostComputeEndpoint{
		Name:               endpointName,
		HostComputeNetwork: networkID,
		SchemaVersion: hcn.SchemaVersion{
			Major: hcnSchemaVersionMajor,
			Minor: hcnSchemaVersionMinor,
		},
	}

	networkContainerApipaIP := localIPConfiguration.IPSubnet.IPAddress
	hostApipaIP := localIPConfiguration.GatewayIPAddress
	protocolList := fmt.Sprintf("%s,%s,%s", protocolICMPv4, protocolTCP, protocolUDP)

	// Endpoint ACL to block all outbound traffic from the Apipa IP of the container
	outBlockAll := hcn.AclPolicySetting{
		Protocols:      protocolList,
		Action:         hcn.ActionTypeBlock,
		Direction:      hcn.DirectionTypeOut,
		LocalAddresses: networkContainerApipaIP,
		RuleType:       hcn.RuleTypeSwitch,
		Priority:       2000,
	}

	if err = addAclToEndpointPolicy(outBlockAll, endpoint.Policies); err != nil {
		return nil, err
	}

	// Endpoint ACL to allow the outbound traffic from the Apipa IP of the container to
	// Apipa IP of the host only
	outAllowToHostOnly := hcn.AclPolicySetting{
		Protocols:       protocolList,
		Action:          hcn.ActionTypeAllow,
		Direction:       hcn.DirectionTypeOut,
		LocalAddresses:  networkContainerApipaIP,
		RemoteAddresses: hostApipaIP,
		RuleType:        hcn.RuleTypeSwitch,
		Priority:        200,
	}

	if err = addAclToEndpointPolicy(outAllowToHostOnly, endpoint.Policies); err != nil {
		return nil, err
	}

	// Endpoint ACL to block all inbound traffic to the Apipa IP of the container
	inBlockAll := hcn.AclPolicySetting{
		Protocols:      protocolList,
		Action:         hcn.ActionTypeBlock,
		Direction:      hcn.DirectionTypeIn,
		LocalAddresses: networkContainerApipaIP,
		RuleType:       hcn.RuleTypeSwitch,
		Priority:       2000,
	}

	if err = addAclToEndpointPolicy(inBlockAll, endpoint.Policies); err != nil {
		return nil, err
	}

	// Endpoint ACL to allow the inbound traffic from the apipa IP of the host to
	// the apipa IP of the container only
	inAllowFromHostOnly := hcn.AclPolicySetting{
		Protocols:       protocolList,
		Action:          hcn.ActionTypeAllow,
		Direction:       hcn.DirectionTypeIn,
		LocalAddresses:  networkContainerApipaIP,
		RemoteAddresses: hostApipaIP,
		RuleType:        hcn.RuleTypeSwitch,
		Priority:        200,
	}

	if err = addAclToEndpointPolicy(inAllowFromHostOnly, endpoint.Policies); err != nil {
		return nil, err
	}

	hcnRoute := hcn.Route{
		NextHop:           hostApipaIP,
		DestinationPrefix: "0.0.0.0/0",
	}

	endpoint.Routes = append(endpoint.Routes, hcnRoute)

	ipConfiguration := hcn.IpConfig{
		IpAddress:    networkContainerApipaIP,
		PrefixLength: localIPConfiguration.IPSubnet.PrefixLength,
	}

	endpoint.IpConfigurations = append(endpoint.IpConfigurations, ipConfiguration)

	log.Printf("[Azure CNS] Configured HostNCApipaEndpoint: %+v", endpoint)

	return endpoint, nil
}

// CreateHostNCApipaEndpoint creates the endpoint in the apipa network for host container connectivity
func CreateHostNCApipaEndpoint(
	networkContainerID string,
	localIPConfiguration cns.IPConfiguration) (string, error) {
	var (
		network      *hcn.HostComputeNetwork
		endpoint     *hcn.HostComputeEndpoint
		endpointName = getHostNCApipaEndpointName(networkContainerID)
		err          error
	)

	namedLock.LockAcquire(endpointName)
	defer namedLock.LockRelease(endpointName)

	// Return if the endpoint already exists
	if endpoint, err = hcn.GetEndpointByName(endpointName); err != nil {
		// TODO: these are failing due to hcn bug https://github.com/microsoft/hcsshim/pull/519/files
		// If error is anything other than EndpointNotFoundError, return error.
		if _, endpointNotFound := err.(hcn.EndpointNotFoundError); !endpointNotFound {
			return "", fmt.Errorf("ERROR: Failed to query endpoint using GetEndpointByName "+
				"due to error: %v", err)
		}
	}

	if endpoint != nil {
		log.Debugf("[Azure CNS] Found existing endpoint: %+v", endpoint)
		return endpoint.Id, nil
	}

	if network, err = createHostNCApipaNetwork(localIPConfiguration); err != nil {
		log.Errorf("[Azure CNS] Failed to create HostNCApipaNetwork. Error: %v", err)
		return "", err
	}

	if endpoint, err = configureHostNCApipaEndpoint(endpointName, network.Id, localIPConfiguration); err != nil {
		log.Errorf("[Azure CNS] Failed to configure HostNCApipaEndpoint: %s. Error: %v", endpointName, err)
		return "", err
	}

	// Create the apipa endpoint
	log.Printf("[Azure CNS] Creating HostNCApipaEndpoint for host container connectivity: %+v", endpoint)
	if endpoint, err = endpoint.Create(); err != nil {
		err = fmt.Errorf("Failed to create HostNCApipaEndpoint: %s. Error: %v", endpointName, err)
		log.Errorf("[Azure CNS] %s", err.Error())
		return "", err
	}

	log.Printf("[Azure CNS] Successfully created HostNCApipaEndpoint: %+v", endpoint)

	return endpoint.Id, nil
}

func getHostNCApipaEndpointName(
	networkContainerID string) string {
	return hostNCApipaEndpointNamePrefix + "-" + networkContainerID
}

func deleteNetworkHnsV2(
	networkID string) error {
	var (
		network *hcn.HostComputeNetwork
		err     error
	)

	if network, err = hcn.GetNetworkByID(networkID); err != nil {
		// If error is anything other than NetworkNotFoundError, return error.
		// else log the error but don't return error because network is already deleted.
		if _, networkNotFound := err.(hcn.NetworkNotFoundError); !networkNotFound {
			return fmt.Errorf("[Azure CNS] deleteNetworkHnsV2 failed due to "+
				"error with GetNetworkByID: %v", err)
		}

		log.Errorf("[Azure CNS] Delete called on the Network: %s which doesn't exist. Error: %v",
			networkID, err)

		return nil
	}

	if err = network.Delete(); err != nil {
		return fmt.Errorf("Failed to delete network: %+v. Error: %v", network, err)
	}

	log.Errorf("[Azure CNS] Successfully deleted network: %+v", network)

	return nil
}

func deleteEndpointByNameHnsV2(
	endpointName string) error {
	var (
		endpoint *hcn.HostComputeEndpoint
		err      error
	)

	// Check if the endpoint exists
	if endpoint, err = hcn.GetEndpointByName(endpointName); err != nil {
		// If error is anything other than EndpointNotFoundError, return error.
		// else log the error but don't return error because endpoint is already deleted.
		if _, endpointNotFound := err.(hcn.EndpointNotFoundError); !endpointNotFound {
			return fmt.Errorf("[Azure CNS] deleteEndpointHnsV2 failed due to "+
				"error with GetEndpointByName: %v", err)
		}

		log.Errorf("[Azure CNS] Delete called on the Endpoint: %s which doesn't exist. Error: %v",
			endpointName, err)

		return nil
	}

	if err = endpoint.Delete(); err != nil {
		return fmt.Errorf("Failed to delete endpoint: %+v. Error: %v", endpoint, err)
	}

	log.Errorf("[Azure CNS] Successfully deleted endpoint: %+v", endpoint)

	return nil
}

// DeleteHostNCApipaEndpoint deletes the endpoint in the apipa network created for host container connectivity
func DeleteHostNCApipaEndpoint(
	networkContainerID string) error {
	endpointName := getHostNCApipaEndpointName(networkContainerID)

	namedLock.LockAcquire(endpointName)
	defer namedLock.LockRelease(endpointName)

	if err := deleteEndpointByNameHnsV2(endpointName); err != nil {
		log.Errorf("[Azure CNS] Failed to delete HostNCApipaEndpoint: %s. Error: %v", endpointName, err)
		return err
	}

	log.Debugf("[Azure CNS] Successfully deleted HostNCApipaEndpoint: %v", endpointName)

	namedLock.LockAcquire(hostNCApipaNetworkName)
	defer namedLock.LockRelease(hostNCApipaNetworkName)

	// Check if hostNCApipaNetworkName has any endpoints left
	if network, err := hcn.GetNetworkByName(hostNCApipaNetworkName); err == nil {
		var endpoints []hcn.HostComputeEndpoint
		if endpoints, err = hcn.ListEndpointsOfNetwork(network.Id); err != nil {
			log.Errorf("[Azure CNS] Failed to list endpoints in the network: %s. Error: %v",
				hostNCApipaNetworkName, err)
			return nil
		}

		// Delete network if it doesn't have any endpoints
		if len(endpoints) == 0 {
			if err = deleteNetworkHnsV2(network.Id); err == nil {
				// Delete the loopback adapter created for this network
				networkcontainers.DeleteLoopbackAdapter(hostNCLoopbackAdapterName)
			}
		}
	}

	return nil
}
