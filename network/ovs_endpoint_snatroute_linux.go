package network

import (
	"net"

	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/network/networkutils"
	"github.com/Azure/azure-container-networking/network/snat"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

// Communication between ovs switch and snat bridge, master of azuresnatveth0 is snat bridge itself
const (
	azureSnatVeth0 = "azSnatveth0"
	azureSnatVeth1 = "azSnatveth1"
)

func (client *OVSEndpointClient) isSnatEnabled() bool {
	return client.enableSnatOnHost || client.allowInboundFromHostToNC || client.allowInboundFromNCToHost || client.enableSnatForDns
}

func (client *OVSEndpointClient) NewSnatClient(snatBridgeIP, localIP string, epInfo *EndpointInfo) {
	if client.isSnatEnabled() {
		client.snatClient = snat.NewSnatClient(
			GetSnatHostIfName(epInfo),
			GetSnatContIfName(epInfo),
			localIP,
			snatBridgeIP,
			client.hostPrimaryMac,
			epInfo.DNS.Servers,
			false,
			client.netlink,
			client.plClient,
		)
	}
}

func (client *OVSEndpointClient) AddSnatEndpoint() error {
	if client.isSnatEnabled() {
		if err := AddSnatEndpoint(&client.snatClient); err != nil {
			return err
		}
		// A lot of this code was in createSnatBridge initially and moved here since it is for ovs only

		snatClient := client.snatClient

		logger.Info("Drop ARP for snat bridge ip", zap.String("SnatBridgeIP", snatClient.SnatBridgeIP))
		if err := client.snatClient.DropArpForSnatBridgeApipaRange(snatClient.SnatBridgeIP, azureSnatVeth0); err != nil {
			return err
		}

		// Create a veth pair. One end of veth will be attached to ovs bridge and other end
		// of veth will be attached to linux bridge
		_, err := net.InterfaceByName(azureSnatVeth0)
		if err == nil {
			logger.Error("Azure snat veth already exists")
			return nil
		}

		vethLink := netlink.VEthLink{
			LinkInfo: netlink.LinkInfo{
				Type: netlink.LINK_TYPE_VETH,
				Name: azureSnatVeth0,
			},
			PeerName: azureSnatVeth1,
		}

		err = client.netlink.AddLink(&vethLink)
		if err != nil {
			logger.Error("Failed to create veth pair", zap.Error(err))
			return errors.Wrap(err, "failed to create veth pair")
		}
		nuc := networkutils.NewNetworkUtils(client.netlink, client.plClient)
		//nolint
		if err = nuc.DisableRAForInterface(azureSnatVeth0); err != nil {
			return err
		}

		//nolint
		if err = nuc.DisableRAForInterface(azureSnatVeth1); err != nil {
			return err
		}

		if err := client.netlink.SetLinkState(azureSnatVeth0, true); err != nil {
			return errors.Wrap(err, "failed to set azure snat veth 0 to up")
		}

		if err := client.netlink.SetLinkMaster(azureSnatVeth0, snat.SnatBridgeName); err != nil {
			return errors.Wrap(err, "failed to set snat veth 0 master to snat bridge")
		}

		if err := client.netlink.SetLinkState(azureSnatVeth1, true); err != nil {
			return errors.Wrap(err, "failed to set azure snat veth 1 to up")
		}

		if err := client.ovsctlClient.AddPortOnOVSBridge(azureSnatVeth1, client.bridgeName, 0); err != nil {
			return errors.Wrap(err, "failed to add port on OVS bridge")
		}
	}
	return nil
}

func (client *OVSEndpointClient) AddSnatEndpointRules() error {
	if client.isSnatEnabled() {
		// Add route for 169.254.169.254 in host via azure0, otherwise it will route via snat bridge
		if err := AddSnatEndpointRules(&client.snatClient, client.allowInboundFromHostToNC, client.allowInboundFromNCToHost, client.netlink, client.plClient); err != nil {
			return err
		}
		if err := AddStaticRoute(client.netlink, client.netioshim, snat.ImdsIP, client.bridgeName); err != nil {
			return err
		}
	}

	return nil
}

func (client *OVSEndpointClient) MoveSnatEndpointToContainerNS(netnsPath string, nsID uintptr) error {
	if client.isSnatEnabled() {
		return MoveSnatEndpointToContainerNS(&client.snatClient, netnsPath, nsID)
	}

	return nil
}

func (client *OVSEndpointClient) SetupSnatContainerInterface() error {
	if client.isSnatEnabled() {
		return SetupSnatContainerInterface(&client.snatClient)
	}

	return nil
}

func (client *OVSEndpointClient) ConfigureSnatContainerInterface() error {
	if client.isSnatEnabled() {
		return ConfigureSnatContainerInterface(&client.snatClient)
	}

	return nil
}

func (client *OVSEndpointClient) DeleteSnatEndpoint() error {
	if client.isSnatEnabled() {
		return DeleteSnatEndpoint(&client.snatClient)
	}

	return nil
}

func (client *OVSEndpointClient) DeleteSnatEndpointRules() {
	DeleteSnatEndpointRules(&client.snatClient, client.allowInboundFromHostToNC, client.allowInboundFromNCToHost)
}
