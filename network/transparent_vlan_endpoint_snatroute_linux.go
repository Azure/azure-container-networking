package network

import (
	"github.com/Azure/azure-container-networking/network/snat"
)

func (client *TransparentVlanEndpointClient) isSnatEnabled() bool {
	return client.enableSnatOnHost || client.allowInboundFromHostToNC || client.allowInboundFromNCToHost || client.enableSnatForDNS
}

func (client *TransparentVlanEndpointClient) NewSnatClient(snatBridgeIP, localIP string, epInfo *EndpointInfo) {
	if client.isSnatEnabled() {
		client.snatClient = snat.NewSnatClient(
			GetSnatHostIfName(epInfo),
			GetSnatContIfName(epInfo),
			localIP,
			snatBridgeIP,
			client.hostPrimaryMac.String(),
			epInfo.DNS.Servers,
			true,
			client.netlink,
			client.plClient,
		)
	}
}

func (client *TransparentVlanEndpointClient) AddSnatEndpoint() error {
	if client.isSnatEnabled() {
		if err := AddSnatEndpoint(&client.snatClient); err != nil {
			return err
		}
	}
	return nil
}

func (client *TransparentVlanEndpointClient) AddSnatEndpointRules() error {
	if client.isSnatEnabled() {
		// Add route for 169.254.169.54 in host via azure0, otherwise it will route via snat bridge
		if err := AddSnatEndpointRules(&client.snatClient, client.allowInboundFromHostToNC, client.allowInboundFromNCToHost, client.netlink, client.plClient); err != nil {
			return err
		}
	}

	return nil
}

func (client *TransparentVlanEndpointClient) MoveSnatEndpointToContainerNS(netnsPath string, nsID uintptr) error {
	if client.isSnatEnabled() {
		return MoveSnatEndpointToContainerNS(&client.snatClient, netnsPath, nsID)
	}

	return nil
}

func (client *TransparentVlanEndpointClient) SetupSnatContainerInterface() error {
	if client.isSnatEnabled() {
		return SetupSnatContainerInterface(&client.snatClient)
	}

	return nil
}

func (client *TransparentVlanEndpointClient) ConfigureSnatContainerInterface() error {
	if client.isSnatEnabled() {
		return ConfigureSnatContainerInterface(&client.snatClient)
	}

	return nil
}

func (client *TransparentVlanEndpointClient) DeleteSnatEndpoint() error {
	if client.isSnatEnabled() {
		return DeleteSnatEndpoint(&client.snatClient)
	}

	return nil
}

func (client *TransparentVlanEndpointClient) DeleteSnatEndpointRules() {
	DeleteSnatEndpointRules(&client.snatClient, client.allowInboundFromHostToNC, client.allowInboundFromNCToHost)
}
