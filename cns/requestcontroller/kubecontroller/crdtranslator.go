package kubecontroller

import (
	"net"

	"github.com/Azure/azure-container-networking/cns"
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
		bits              int
	)

	ncRequest.SecondaryIPConfigs = make(map[string]cns.SecondaryIPConfig)

	for _, nc = range crdStatus.NetworkContainers {
		ncRequest.NetworkContainerid = nc.ID
		ncRequest.NetworkContainerType = cns.Docker

		// Convert "10.0.0.1/32" into "10.0.0.1" and 32
		if ip, ipNet, err = net.ParseCIDR(nc.PrimaryIP); err != nil {
			return ncRequest, err
		}
		_, bits = ipNet.Mask.Size()

		ipSubnet.IPAddress = ip.String()
		ipSubnet.PrefixLength = uint8(bits)
		ncRequest.IPConfiguration.IPSubnet = ipSubnet

		for _, ipAssignment = range nc.IPAssignments {
			if ip, ipNet, err = net.ParseCIDR(ipAssignment.IP); err != nil {
				return ncRequest, err
			}

			_, bits = ipNet.Mask.Size()

			ipSubnet.IPAddress = ip.String()
			ipSubnet.PrefixLength = uint8(bits)
			secondaryIPConfig = cns.SecondaryIPConfig{
				IPConfig: ipSubnet,
			}
			ncRequest.SecondaryIPConfigs[ipAssignment.Name] = secondaryIPConfig
		}
	}

	//Only returning the first network container for now, later we will return a list
	return ncRequest, nil
}

// CNSToCRDSpec translates CNS's list of Ips to be released and requested ip count into a CRD Spec
func CNSToCRDSpec(toBeDeletedSecondaryIPConfigs []cns.SecondaryIPConfig, ipCount int) (nnc.NodeNetworkConfigSpec, error) {
	var (
		spec              nnc.NodeNetworkConfigSpec
		secondaryIPConfig cns.SecondaryIPConfig
	)

	spec.RequestedIPCount = int64(ipCount)

	for _, secondaryIPConfig = range toBeDeletedSecondaryIPConfigs {
		spec.IPsNotInUse = append(spec.IPsNotInUse, secondaryIPConfig.IPConfig.IPAddress)
	}

	return spec, nil
}
