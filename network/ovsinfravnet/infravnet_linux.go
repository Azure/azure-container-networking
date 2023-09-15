// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package ovsinfravnet

import (
	"errors"
	"fmt"
	"net"

	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/network/log"
	"github.com/Azure/azure-container-networking/network/networkutils"
	"github.com/Azure/azure-container-networking/ovsctl"
	"github.com/Azure/azure-container-networking/platform"
	"go.uber.org/zap"
)

var (
	loggerName = "net"
	logger     = log.InitZapLogNet(loggerName)
)

const (
	azureInfraIfName = "eth2"
)

var errorOVSInfraVnetClient = errors.New("OVSInfraVnetClient Error")

func newErrorOVSInfraVnetClient(errStr string) error {
	return fmt.Errorf("%w : %s", errorOVSInfraVnetClient, errStr)
}

type OVSInfraVnetClient struct {
	hostInfraVethName      string
	ContainerInfraVethName string
	containerInfraMac      string
	netlink                netlink.NetlinkInterface
	plClient               platform.ExecClient
}

func NewInfraVnetClient(hostIfName, contIfName string, nl netlink.NetlinkInterface, plc platform.ExecClient) OVSInfraVnetClient {
	infraVnetClient := OVSInfraVnetClient{
		hostInfraVethName:      hostIfName,
		ContainerInfraVethName: contIfName,
		netlink:                nl,
		plClient:               plc,
	}

	logger.Info("Initialize new infravnet client", zap.Any("infraVnetClient", infraVnetClient))

	return infraVnetClient
}

func (client *OVSInfraVnetClient) CreateInfraVnetEndpoint(bridgeName string) error {
	ovs := ovsctl.NewOvsctl()
	epc := networkutils.NewNetworkUtils(client.netlink, client.plClient)
	if err := epc.CreateEndpoint(client.hostInfraVethName, client.ContainerInfraVethName, nil); err != nil {
		logger.Error("Creating infraep failed with", zap.Error(err))
		return err
	}

	logger.Info("Adding port master", zap.String("hostInfraVethName", client.hostInfraVethName), zap.String("bridgeName", bridgeName))
	if err := ovs.AddPortOnOVSBridge(client.hostInfraVethName, bridgeName, 0); err != nil {
		logger.Error("Adding infraveth to ovsbr failed with", zap.Error(err))
		return err
	}

	infraContainerIf, err := net.InterfaceByName(client.ContainerInfraVethName)
	if err != nil {
		logger.Error("InterfaceByName returns error for ifname with", zap.String("ContainerInfraVethName", client.ContainerInfraVethName),
			zap.Error(err))
		return err
	}

	client.containerInfraMac = infraContainerIf.HardwareAddr.String()

	return nil
}

func (client *OVSInfraVnetClient) CreateInfraVnetRules(
	bridgeName string,
	infraIP net.IPNet,
	hostPrimaryMac string,
	hostPort string,
) error {
	ovs := ovsctl.NewOvsctl()

	infraContainerPort, err := ovs.GetOVSPortNumber(client.hostInfraVethName)
	if err != nil {
		logger.Error("[ovs] Get ofport failed with", zap.Error(err))
		return err
	}

	// 0 signifies not to add vlan tag to this traffic
	if err := ovs.AddIPSnatRule(bridgeName, infraIP.IP, 0, infraContainerPort, hostPrimaryMac, hostPort); err != nil {
		logger.Error("[ovs] AddIpSnatRule failed with", zap.Error(err))
		return err
	}

	// 0 signifies not to match traffic based on vlan tag
	if err := ovs.AddMacDnatRule(bridgeName, hostPort, infraIP.IP, client.containerInfraMac, 0, infraContainerPort); err != nil {
		logger.Error("[ovs] AddMacDnatRule failed with", zap.Error(err))
		return err
	}

	return nil
}

func (client *OVSInfraVnetClient) MoveInfraEndpointToContainerNS(netnsPath string, nsID uintptr) error {
	logger.Info("[ovs] Setting link netns", zap.String("ContainerInfraVethName", client.ContainerInfraVethName),
		zap.Any("netnsPath", netnsPath))
	err := client.netlink.SetLinkNetNs(client.ContainerInfraVethName, nsID)
	if err != nil {
		return newErrorOVSInfraVnetClient(err.Error())
	}
	return nil
}

func (client *OVSInfraVnetClient) SetupInfraVnetContainerInterface() error {
	epc := networkutils.NewNetworkUtils(client.netlink, client.plClient)
	if err := epc.SetupContainerInterface(client.ContainerInfraVethName, azureInfraIfName); err != nil {
		return newErrorOVSInfraVnetClient(err.Error())
	}

	client.ContainerInfraVethName = azureInfraIfName

	return nil
}

func (client *OVSInfraVnetClient) ConfigureInfraVnetContainerInterface(infraIP net.IPNet) error {
	logger.Info("[ovs] Adding IP address to link", zap.String("IP", infraIP.String()), zap.String("ContainerInfraVethName", client.ContainerInfraVethName))
	err := client.netlink.AddIPAddress(client.ContainerInfraVethName, infraIP.IP, &infraIP)
	if err != nil {
		return newErrorOVSInfraVnetClient(err.Error())
	}
	return nil
}

func (client *OVSInfraVnetClient) DeleteInfraVnetRules(
	bridgeName string,
	infraIP net.IPNet,
	hostPort string,
) {
	ovs := ovsctl.NewOvsctl()

	logger.Info("[ovs] Deleting MAC DNAT rule for infravnet IP address", zap.String("IP", infraIP.IP.String()))
	ovs.DeleteMacDnatRule(bridgeName, hostPort, infraIP.IP, 0)

	logger.Info("[ovs] Get ovs port for infravnet interface", zap.String("hostInfraVethName", client.hostInfraVethName))
	infraContainerPort, err := ovs.GetOVSPortNumber(client.hostInfraVethName)
	if err != nil {
		logger.Error("[ovs] Get infravnet portnum failed with", zap.Error(err))
	}

	logger.Info("Deleting IP SNAT for infravnet port", zap.String("infraContainerPort", infraContainerPort))
	ovs.DeleteIPSnatRule(bridgeName, infraContainerPort)

	logger.Info("[ovs] Deleting infravnet interface", zap.String("hostInfraVethName", client.hostInfraVethName), zap.String("bridgeName", bridgeName))
	if err := ovs.DeletePortFromOVS(bridgeName, client.hostInfraVethName); err != nil {
		logger.Error("[ovs] Deletion of infravnet interface", zap.String("hostInfraVethName", client.hostInfraVethName), zap.String("bridgeName", bridgeName))
	}
}

func (client *OVSInfraVnetClient) DeleteInfraVnetEndpoint() error {
	logger.Info("[ovs] Deleting Infra veth pair", zap.String("hostInfraVethName", client.hostInfraVethName))
	err := client.netlink.DeleteLink(client.hostInfraVethName)
	if err != nil {
		logger.Error("[ovs] Failed to delete veth pair", zap.String("hostInfraVethName", client.hostInfraVethName),
			zap.Error(err))
		return newErrorOVSInfraVnetClient(err.Error())
	}

	return nil
}
