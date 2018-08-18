package network

import (
	"fmt"

	"github.com/Azure/azure-container-networking/network/ovssnat"
)

func NewSnatClient(client *OVSEndpointClient, epInfo *EndpointInfo) {
	if client.enableSnatOnHost {
		var localIP, snatBridgeIP string

		hostIfName := fmt.Sprintf("%s%s", snatVethInterfacePrefix, epInfo.Id[:7])
		contIfName := fmt.Sprintf("%s%s-2", snatVethInterfacePrefix, epInfo.Id[:7])

		if _, ok := epInfo.Data[LocalIPKey]; ok {
			localIP = epInfo.Data[LocalIPKey].(string)
		}

		if _, ok := epInfo.Data[SnatBridgeIPKey]; ok {
			snatBridgeIP = epInfo.Data[SnatBridgeIPKey].(string)
		}

		client.snatClient = ovssnat.NewSnatClient(hostIfName, contIfName, localIP, snatBridgeIP, epInfo.DNS.Servers)
	}
}

func AddSnatEndpoint(client *OVSEndpointClient) error {
	if client.enableSnatOnHost {
		return client.snatClient.CreateSnatEndpoint(client.bridgeName)
	}

	return nil
}

func AddSnatEndpointRules(client *OVSEndpointClient) error {
	if client.enableSnatOnHost {
		if err := client.snatClient.AddPrivateIPBlockRule(); err != nil {
			return err
		}

		return AddStaticRoute(ovssnat.ImdsIP, client.bridgeName)
	}

	return nil
}

func MoveSnatEndpointToContainerNS(client *OVSEndpointClient, netnsPath string, nsID uintptr) error {
	if client.enableSnatOnHost {
		return client.snatClient.MoveSnatEndpointToContainerNS(netnsPath, nsID)
	}

	return nil
}

func SetupSnatContainerInterface(client *OVSEndpointClient) error {
	if client.enableSnatOnHost {
		return client.snatClient.SetupSnatContainerInterface()
	}

	return nil
}

func ConfigureSnatContainerInterface(client *OVSEndpointClient) error {
	if client.enableSnatOnHost {
		return client.snatClient.ConfigureSnatContainerInterface()
	}

	return nil
}

func DeleteSnatEndpoint(client *OVSEndpointClient) error {
	if client.enableSnatOnHost {
		return client.snatClient.DeleteSnatEndpoint()
	}

	return nil
}
