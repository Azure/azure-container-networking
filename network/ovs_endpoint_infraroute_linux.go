package network

import (
	"fmt"
	"net"

	"github.com/Azure/azure-container-networking/network/ovsinfravnet"
)

func NewInfraVnetClient(client *OVSEndpointClient, epID string) {
	if client.enableInfraVnet {
		hostIfName := fmt.Sprintf("%s%s", infraVethInterfacePrefix, epID)
		contIfName := fmt.Sprintf("%s%s-2", infraVethInterfacePrefix, epID)

		client.infraVnetClient = ovsinfravnet.NewInfraVnetClient(hostIfName, contIfName, client.netlink)
	}
}

func AddInfraVnetEndpoint(client *OVSEndpointClient) error {
	if client.enableInfraVnet {
		return client.infraVnetClient.CreateInfraVnetEndpoint(client.bridgeName)
	}

	return nil
}

func AddInfraEndpointRules(client *OVSEndpointClient, infraIP net.IPNet, hostPort string) error {
	if client.enableInfraVnet {
		return client.infraVnetClient.CreateInfraVnetRules(client.bridgeName, infraIP, client.hostPrimaryMac, hostPort)
	}

	return nil
}

func DeleteInfraVnetEndpointRules(client *OVSEndpointClient, ep *endpoint, hostPort string) {
	if client.enableInfraVnet {
		client.infraVnetClient.DeleteInfraVnetRules(client.bridgeName, ep.InfraVnetIP, hostPort)
	}
}

func MoveInfraEndpointToContainerNS(client *OVSEndpointClient, netnsPath string, nsID uintptr) error {
	if client.enableInfraVnet {
		return client.infraVnetClient.MoveInfraEndpointToContainerNS(netnsPath, nsID)
	}

	return nil
}

func SetupInfraVnetContainerInterface(client *OVSEndpointClient) error {
	if client.enableInfraVnet {
		return client.infraVnetClient.SetupInfraVnetContainerInterface()
	}

	return nil
}

func ConfigureInfraVnetContainerInterface(client *OVSEndpointClient, infraIP net.IPNet) error {
	if client.enableInfraVnet {
		return client.infraVnetClient.ConfigureInfraVnetContainerInterface(infraIP)
	}

	return nil
}

func DeleteInfraVnetEndpoint(client *OVSEndpointClient, epID string) error {
	if client.enableInfraVnet {
		return client.infraVnetClient.DeleteInfraVnetEndpoint()
	}

	return nil
}
