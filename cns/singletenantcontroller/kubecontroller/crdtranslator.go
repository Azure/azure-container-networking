package kubecontroller

import (
	"fmt"
	"net"
	"strconv"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
)

// CRDStatusToNCRequest translates a crd status to createnetworkcontainer request
func CRDStatusToNCRequest(crdStatus nnc.NodeNetworkConfigStatus) (cns.CreateNetworkContainerRequest, error) {
	var (
		ncRequest         cns.CreateNetworkContainerRequest
		nc                nnc.NetworkContainer
		secondaryIPConfig cns.SecondaryIPConfig
		ipSubnet          cns.IPSubnet
		ipAssignment      nnc.IPAssignment
		err               error
		ip                net.IP
		ipNet             *net.IPNet
		size              int
		numNCsSupported   int
		numNCs            int
	)

	numNCsSupported = 1
	numNCs = len(crdStatus.NetworkContainers)

	// Right now we're only supporing one NC per node, but in the future we will support multiple NCs per node
	if numNCs > numNCsSupported {
		return ncRequest, fmt.Errorf("Number of network containers is not supported. Got %v number of ncs, supports %v", numNCs, numNCsSupported)
	}

	for _, nc = range crdStatus.NetworkContainers {
		ncRequest.SecondaryIPConfigs = make(map[string]cns.SecondaryIPConfig)
		ncRequest.NetworkContainerid = nc.ID
		ncRequest.NetworkContainerType = cns.Docker
		ncRequest.Version = strconv.FormatInt(nc.Version, 10)

		if ip = net.ParseIP(nc.PrimaryIP); ip == nil {
			return ncRequest, fmt.Errorf("Invalid PrimaryIP %s:", nc.PrimaryIP)
		}

		if _, ipNet, err = net.ParseCIDR(nc.SubnetAddressSpace); err != nil {
			return ncRequest, fmt.Errorf("Invalid SubnetAddressSpace %s:, err:%s", nc.SubnetAddressSpace, err)
		}

		size, _ = ipNet.Mask.Size()
		ipSubnet.IPAddress = ip.String()
		ipSubnet.PrefixLength = uint8(size)
		ncRequest.IPConfiguration.IPSubnet = ipSubnet
		ncRequest.IPConfiguration.GatewayIPAddress = nc.DefaultGateway
		var ncVersion int
		if ncVersion, err = strconv.Atoi(ncRequest.Version); err != nil {
			return ncRequest, fmt.Errorf("Invalid ncRequest.Version is %s in CRD, err:%s", ncRequest.Version, err)
		}

		for _, ipAssignment = range nc.IPAssignments {
			if ip = net.ParseIP(ipAssignment.IP); ip == nil {
				return ncRequest, fmt.Errorf("Invalid SecondaryIP %s:", ipAssignment.IP)
			}
			secondaryIPConfig = cns.SecondaryIPConfig{
				IPAddress: ip.String(),
				NCVersion: ncVersion,
			}
			ncRequest.SecondaryIPConfigs[ipAssignment.Name] = secondaryIPConfig
			logger.Debugf("Seconday IP Configs got set, name is %s, config is %v", ipAssignment.Name, secondaryIPConfig)
		}
		logger.Printf("Set NC request info with NetworkContainerid %s, NetworkContainerType %s, NC Version %s",
			ncRequest.NetworkContainerid, ncRequest.NetworkContainerType, ncRequest.Version)
	}

	//Only returning the first network container for now, later we will return a list
	return ncRequest, nil
}
