package network

import (
	"fmt"

	"github.com/Azure/azure-container-networking/network/epcommon"
	"github.com/Azure/azure-container-networking/network/ovssnat"
)

func NewSnatClient(client *OVSEndpointClient, snatBridgeIP string, localIP string, epInfo *EndpointInfo) {
	if client.enableSnatOnHost || client.allowInboundFromHostToNC || client.allowInboundFromNCToHost || client.enableSnatForDns {
		hostIfName := fmt.Sprintf("%s%s", snatVethInterfacePrefix, epInfo.Id[:7])
		contIfName := fmt.Sprintf("%s%s-2", snatVethInterfacePrefix, epInfo.Id[:7])

		client.snatClient = ovssnat.NewSnatClient(hostIfName, contIfName, localIP, snatBridgeIP, client.hostPrimaryMac, epInfo.DNS.Servers)
	}
}

func AddSnatEndpoint(client *OVSEndpointClient) error {
	if client.enableSnatOnHost || client.allowInboundFromHostToNC || client.allowInboundFromNCToHost || client.enableSnatForDns {
		if err := client.snatClient.CreateSnatEndpoint(client.bridgeName); err != nil {
			return err
		}
	}

	return nil
}

func AddSnatEndpointRules(client *OVSEndpointClient) error {
	if client.enableSnatOnHost || client.allowInboundFromHostToNC || client.allowInboundFromNCToHost || client.enableSnatForDns {
		// Allow specific Private IPs via Snat Bridge
		if err := client.snatClient.AllowIPAddressesOnSnatBrdige(); err != nil {
			return err
		}

		// Block Private IPs via Snat Bridge
		if err := client.snatClient.BlockIPAddressesOnSnatBrdige(); err != nil {
			return err
		}

		// Add route for 169.254.169.54 in host via azure0, otherwise it will route via snat bridge
		if err := AddStaticRoute(ovssnat.ImdsIP, client.bridgeName); err != nil {
			return err
		}

		if err := epcommon.EnableIPForwarding(ovssnat.SnatBridgeName); err != nil {
			return err
		}

		if client.allowInboundFromHostToNC {
			if err := client.snatClient.AllowInboundFromHostToNC(); err != nil {
				return err
			}
		}

		if client.allowInboundFromNCToHost {
			return client.snatClient.AllowInboundFromNCToHost()
		}
	}

	return nil
}

func MoveSnatEndpointToContainerNS(client *OVSEndpointClient, netnsPath string, nsID uintptr) error {
	if client.enableSnatOnHost || client.allowInboundFromHostToNC || client.allowInboundFromNCToHost || client.enableSnatForDns {
		return client.snatClient.MoveSnatEndpointToContainerNS(netnsPath, nsID)
	}

	return nil
}

func SetupSnatContainerInterface(client *OVSEndpointClient) error {
	if client.enableSnatOnHost || client.allowInboundFromHostToNC || client.allowInboundFromNCToHost || client.enableSnatForDns {
		return client.snatClient.SetupSnatContainerInterface()
	}

	return nil
}

func ConfigureSnatContainerInterface(client *OVSEndpointClient) error {
	if client.enableSnatOnHost || client.allowInboundFromHostToNC || client.allowInboundFromNCToHost || client.enableSnatForDns {
		return client.snatClient.ConfigureSnatContainerInterface()
	}

	return nil
}

func DeleteSnatEndpoint(client *OVSEndpointClient) error {
	if client.enableSnatOnHost || client.allowInboundFromHostToNC || client.allowInboundFromNCToHost || client.enableSnatForDns {
		return client.snatClient.DeleteSnatEndpoint()
	}

	return nil
}

func DeleteSnatEndpointRules(client *OVSEndpointClient) {
	if client.allowInboundFromHostToNC {
		client.snatClient.DeleteInboundFromHostToNC()
	}

	if client.allowInboundFromNCToHost {
		client.snatClient.DeleteInboundFromNCToHost()
	}
}
