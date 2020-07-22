package kubecontroller

import (
	"net"
	"strconv"
	"strings"

	"github.com/Azure/azure-container-networking/cns"
	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
)

// CRDStatusToNCRequest translates a crd status to createnetworkcontainer request
func CRDStatusToNCRequest(crdStatus *nnc.NodeNetworkConfigStatus) (*cns.CreateNetworkContainerRequest, error) {
	var (
		createNCRequest   cns.CreateNetworkContainerRequest
		nc                nnc.NetworkContainer
		secondaryIPConfig cns.SecondaryIPConfig
		ipSubnet          cns.IPSubnet
		ipAssignment      nnc.IPAssignment
		prefixLength      int
	)

	createNCRequest.SecondaryIPConfigs = make(map[string]cns.SecondaryIPConfig)

	for _, nc = range crdStatus.NetworkContainers {
		createNCRequest.NetworkContainerid = nc.ID
		createNCRequest.PrimaryInterfaceIdentifier = nc.PrimaryIP
		prefixLength = maskStringToPrefixLength(nc.Netmask)

		for _, ipAssignment = range nc.IPAssignments {
			ipSubnet.IPAddress = ipAssignment.IP
			ipSubnet.PrefixLength = uint8(prefixLength)
			secondaryIPConfig = cns.SecondaryIPConfig{
				IPConfig: ipSubnet,
			}
			createNCRequest.SecondaryIPConfigs[ipAssignment.Name] = secondaryIPConfig
		}
	}

	//Only returning the first network container for now, later we will return a list
	return &createNCRequest, nil
}

// CNSToCRDSpec translates CNS's list of Ips to be released and requested ip count into a CRD Spec
func CNSToCRDSpec(secondaryIPConfigs []cns.SecondaryIPConfig, ipCount int) (*nnc.NodeNetworkConfigSpec, error) {
	var (
		spec              nnc.NodeNetworkConfigSpec
		secondaryIPConfig cns.SecondaryIPConfig
	)

	spec.RequestedIPCount = int64(ipCount)

	for _, secondaryIPConfig = range secondaryIPConfigs {
		spec.IPsNotInUse = append(spec.IPsNotInUse, secondaryIPConfig.IPConfig.IPAddress)
	}

	return &spec, nil
}

func maskStringToBytes(mask string) []byte {
	pieces := strings.Split(mask, ".")
	b := make([]byte, 4)
	for i, piece := range pieces {
		j, _ := strconv.Atoi(piece)
		b[i] = byte(j)
	}
	return b
}

func maskStringToPrefixLength(mask string) int {
	var (
		netmask      net.IPMask
		prefixLength int
	)

	netmask = maskStringToBytes(mask)
	prefixLength, _ = netmask.Size()

	return prefixLength
}
