// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package ovsctl

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/Azure/azure-container-networking/cni/log"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/platform"
	"go.uber.org/zap"
)

var logger = log.CNILogger.With(zap.String("component", "ovs"))

const (
	defaultMacForArpResponse = "12:34:56:78:9a:bc"
)

// Open flow rule priorities. Higher the number higher the priority
const (
	low  = 10
	mid  = 15
	high = 20
)

var errorMockOvsctl = errors.New("MockOvsctlError")

func newErrorOvsctl(errorString string) error {
	return fmt.Errorf("%w: %v", errorMockOvsctl, errorString)
}

type OvsInterface interface {
	// TODO: remove this interface after platform calls are mocked
	CreateOVSBridge(bridgeName string) error
	DeleteOVSBridge(bridgeName string) error
	AddPortOnOVSBridge(hostIfName string, bridgeName string, vlanID int) error
	GetOVSPortNumber(interfaceName string) (string, error)
	AddVMIpAcceptRule(bridgeName string, primaryIP string, mac string) error
	AddArpSnatRule(bridgeName string, mac string, macHex string, ofport string) error
	AddIPSnatRule(bridgeName string, ip net.IP, vlanID int, port string, mac string, outport string) error
	AddArpDnatRule(bridgeName string, port string, mac string) error
	AddFakeArpReply(bridgeName string, ip net.IP) error
	AddArpReplyRule(bridgeName string, port string, ip net.IP, mac string, vlanid int, mode string) error
	AddMacDnatRule(bridgeName string, port string, ip net.IP, mac string, vlanid int, containerPort string) error
	DeleteArpReplyRule(bridgeName string, port string, ip net.IP, vlanid int)
	DeleteIPSnatRule(bridgeName string, port string)
	DeleteMacDnatRule(bridgeName string, port string, ip net.IP, vlanid int)
	DeletePortFromOVS(bridgeName string, interfaceName string) error
}

type Ovsctl struct {
	execcli platform.ExecClient
}

func NewOvsctl() Ovsctl {
	return Ovsctl{execcli: platform.NewExecClient()}
}

func (o Ovsctl) CreateOVSBridge(bridgeName string) error {
	logger.Info("Creating OVS Bridge", zap.String("name", bridgeName))

	ovsCreateCmd := fmt.Sprintf("ovs-vsctl add-br %s", bridgeName)
	_, err := o.execcli.ExecuteCommand(ovsCreateCmd)
	if err != nil {
		logger.Error("Error while creating OVS bridge", zap.Error(err))
		return newErrorOvsctl(err.Error())
	}

	return nil
}

func (o Ovsctl) DeleteOVSBridge(bridgeName string) error {
	logger.Info("Deleting OVS Bridge", zap.String("name", bridgeName))

	ovsCreateCmd := fmt.Sprintf("ovs-vsctl del-br %s", bridgeName)
	_, err := o.execcli.ExecuteCommand(ovsCreateCmd)
	if err != nil {
		logger.Error("Error while deleting OVS bridge", zap.Error(err))
		return newErrorOvsctl(err.Error())
	}

	return nil
}

func (o Ovsctl) AddPortOnOVSBridge(hostIfName, bridgeName string, vlanID int) error {
	cmd := fmt.Sprintf("ovs-vsctl add-port %s %s", bridgeName, hostIfName)
	_, err := o.execcli.ExecuteCommand(cmd)
	if err != nil {
		logger.Error("Error while setting OVS as master to primary interface", zap.Error(err))
		return newErrorOvsctl(err.Error())
	}

	return nil
}

func (o Ovsctl) GetOVSPortNumber(interfaceName string) (string, error) {
	cmd := fmt.Sprintf("ovs-vsctl get Interface %s ofport", interfaceName)
	ofport, err := o.execcli.ExecuteCommand(cmd)
	if err != nil {
		logger.Error("Get ofport failed with", zap.Error(err))
		return "", newErrorOvsctl(err.Error())
	}

	return strings.Trim(ofport, "\n"), nil
}

func (o Ovsctl) AddVMIpAcceptRule(bridgeName, primaryIP, mac string) error {
	cmd := fmt.Sprintf("ovs-ofctl add-flow %s ip,nw_dst=%s,dl_dst=%s,priority=%d,actions=normal", bridgeName, primaryIP, mac, high)
	_, err := o.execcli.ExecuteCommand(cmd)
	if err != nil {
		logger.Error("Adding SNAT rule failed with", zap.Error(err))
		return newErrorOvsctl(err.Error())
	}

	return nil
}

func (o Ovsctl) AddArpSnatRule(bridgeName, mac, macHex, ofport string) error {
	cmd := fmt.Sprintf(`ovs-ofctl add-flow %v table=1,priority=%d,arp,arp_op=1,actions='mod_dl_src:%s,
		load:0x%s->NXM_NX_ARP_SHA[],output:%s'`, bridgeName, low, mac, macHex, ofport)
	_, err := o.execcli.ExecuteCommand(cmd)
	if err != nil {
		logger.Error("Adding ARP SNAT rule failed with", zap.Error(err))
		return newErrorOvsctl(err.Error())
	}

	return nil
}

// IP SNAT Rule - Change src mac to VM Mac for packets coming from container host veth port.
func (o Ovsctl) AddIPSnatRule(bridgeName string, ip net.IP, vlanID int, port, mac, outport string) error {
	var cmd string
	if outport == "" {
		outport = "normal"
	}

	commonPrefix := fmt.Sprintf("ovs-ofctl add-flow %v priority=%d,ip,nw_src=%s,in_port=%s,vlan_tci=0,actions=mod_dl_src:%s", bridgeName, high, ip.String(), port, mac)

	// This rule also checks if packets coming from right source ip based on the ovs port to prevent ip spoofing.
	// Otherwise it drops the packet.
	if vlanID != 0 {
		cmd = fmt.Sprintf("%s,mod_vlan_vid:%v,%v", commonPrefix, vlanID, outport)
	} else {
		cmd = fmt.Sprintf("%s,strip_vlan,%v", commonPrefix, outport)
	}

	_, err := o.execcli.ExecuteCommand(cmd)
	if err != nil {
		logger.Error("Adding IP SNAT rule failed with", zap.Error(err))
		return newErrorOvsctl(err.Error())
	}

	// Drop other packets which doesn't satisfy above condition
	cmd = fmt.Sprintf("ovs-ofctl add-flow %v priority=%d,ip,in_port=%s,actions=drop",
		bridgeName, low, port)
	_, err = o.execcli.ExecuteCommand(cmd)
	if err != nil {
		logger.Error("Dropping vlantag packet rule failed with", zap.Error(err))
		return newErrorOvsctl(err.Error())
	}

	return nil
}

func (o Ovsctl) AddArpDnatRule(bridgeName, port, mac string) error {
	// Add DNAT rule to forward ARP replies to container interfaces.
	cmd := fmt.Sprintf(`ovs-ofctl add-flow %s arp,arp_op=2,in_port=%s,actions='mod_dl_dst:ff:ff:ff:ff:ff:ff,
		load:0x%s->NXM_NX_ARP_THA[],normal'`, bridgeName, port, mac)
	_, err := o.execcli.ExecuteCommand(cmd)
	if err != nil {
		logger.Error("Adding DNAT rule failed with", zap.Error(err))
		return newErrorOvsctl(err.Error())
	}

	return nil
}

func (o Ovsctl) AddFakeArpReply(bridgeName string, ip net.IP) error {
	// If arp fields matches, set arp reply rule for the request
	macAddrHex := strings.Replace(defaultMacForArpResponse, ":", "", -1)
	ipAddrInt := common.IpToInt(ip)

	logger.Info("Adding ARP reply rule for IP", zap.String("address", ip.String()))
	cmd := fmt.Sprintf(`ovs-ofctl add-flow %s arp,arp_op=1,priority=%d,actions='load:0x2->NXM_OF_ARP_OP[],
			move:NXM_OF_ETH_SRC[]->NXM_OF_ETH_DST[],mod_dl_src:%s,
			move:NXM_NX_ARP_SHA[]->NXM_NX_ARP_THA[],move:NXM_OF_ARP_TPA[]->NXM_OF_ARP_SPA[],
			load:0x%s->NXM_NX_ARP_SHA[],load:0x%x->NXM_OF_ARP_TPA[],IN_PORT'`,
		bridgeName, high, defaultMacForArpResponse, macAddrHex, ipAddrInt)
	_, err := o.execcli.ExecuteCommand(cmd)
	if err != nil {
		logger.Error("[ovs] Adding ARP reply rule failed with", zap.Error(err))
		return newErrorOvsctl(err.Error())
	}

	return nil
}

func (o Ovsctl) AddArpReplyRule(bridgeName, port string, ip net.IP, mac string, vlanid int, mode string) error {
	ipAddrInt := common.IpToInt(ip)
	macAddrHex := strings.Replace(mac, ":", "", -1)

	logger.Info("Adding ARP reply rule to add vlan and forward packet to table 1 for port", zap.Int("vlanid", vlanid), zap.String("port", port))
	cmd := fmt.Sprintf(`ovs-ofctl add-flow %s arp,arp_op=1,in_port=%s,actions='mod_vlan_vid:%v,resubmit(,1)'`,
		bridgeName, port, vlanid)
	_, err := o.execcli.ExecuteCommand(cmd)
	if err != nil {
		logger.Error("Adding ARP reply rule failed with", zap.Error(err))
		return newErrorOvsctl(err.Error())
	}

	// If arp fields matches, set arp reply rule for the request
	logger.Info("Adding ARP reply rule for IP", zap.Any("address", ip), zap.Int("vlanid", vlanid))
	cmd = fmt.Sprintf(`ovs-ofctl add-flow %s table=1,arp,arp_tpa=%s,dl_vlan=%v,arp_op=1,priority=%d,actions='load:0x2->NXM_OF_ARP_OP[],
			move:NXM_OF_ETH_SRC[]->NXM_OF_ETH_DST[],mod_dl_src:%s,
			move:NXM_NX_ARP_SHA[]->NXM_NX_ARP_THA[],move:NXM_OF_ARP_SPA[]->NXM_OF_ARP_TPA[],
			load:0x%s->NXM_NX_ARP_SHA[],load:0x%x->NXM_OF_ARP_SPA[],strip_vlan,IN_PORT'`,
		bridgeName, ip.String(), vlanid, high, mac, macAddrHex, ipAddrInt)
	_, err = o.execcli.ExecuteCommand(cmd)
	if err != nil {
		logger.Error("Adding ARP reply rule failed with", zap.Error(err))
		return newErrorOvsctl(err.Error())
	}

	return nil
}

// Add MAC DNAT rule based on dst ip and vlanid
func (o Ovsctl) AddMacDnatRule(bridgeName, port string, ip net.IP, mac string, vlanid int, containerPort string) error {
	var cmd string
	// This rule changes the destination mac to speciifed mac based on the ip and vlanid.
	// and forwards the packet to corresponding container hostveth port

	commonPrefix := fmt.Sprintf("ovs-ofctl add-flow %s ip,nw_dst=%s,in_port=%s", bridgeName, ip.String(), port)
	if vlanid != 0 {
		cmd = fmt.Sprintf("%s,dl_vlan=%v,actions=mod_dl_dst:%s,strip_vlan,%s", commonPrefix, vlanid, mac, containerPort)
	} else {
		cmd = fmt.Sprintf("%s,actions=mod_dl_dst:%s,strip_vlan,%s", commonPrefix, mac, containerPort)
	}
	_, err := o.execcli.ExecuteCommand(cmd)
	if err != nil {
		logger.Error("Adding MAC DNAT rule failed with", zap.Error(err))
		return newErrorOvsctl(err.Error())
	}

	return nil
}

func (o Ovsctl) DeleteArpReplyRule(bridgeName, port string, ip net.IP, vlanid int) {
	cmd := fmt.Sprintf("ovs-ofctl del-flows %s arp,arp_op=1,in_port=%s",
		bridgeName, port)
	_, err := o.execcli.ExecuteCommand(cmd)
	if err != nil {
		logger.Error("[net] Deleting ARP reply rule failed with", zap.Error(err))
	}

	cmd = fmt.Sprintf("ovs-ofctl del-flows %s table=1,arp,arp_tpa=%s,dl_vlan=%v,arp_op=1",
		bridgeName, ip.String(), vlanid)
	_, err = o.execcli.ExecuteCommand(cmd)
	if err != nil {
		logger.Error("[net] Deleting ARP reply rule failed with", zap.Error(err))
	}
}

func (o Ovsctl) DeleteIPSnatRule(bridgeName, port string) {
	cmd := fmt.Sprintf("ovs-ofctl del-flows %v ip,in_port=%s",
		bridgeName, port)
	_, err := o.execcli.ExecuteCommand(cmd)
	if err != nil {
		logger.Error("Error while deleting ovs rule", zap.String("cmd", cmd), zap.Error(err))
	}
}

func (o Ovsctl) DeleteMacDnatRule(bridgeName, port string, ip net.IP, vlanid int) {
	var cmd string

	if vlanid != 0 {
		cmd = fmt.Sprintf("ovs-ofctl del-flows %s ip,nw_dst=%s,dl_vlan=%v,in_port=%s",
			bridgeName, ip.String(), vlanid, port)
	} else {
		cmd = fmt.Sprintf("ovs-ofctl del-flows %s ip,nw_dst=%s,in_port=%s",
			bridgeName, ip.String(), port)
	}

	_, err := o.execcli.ExecuteCommand(cmd)
	if err != nil {
		logger.Error("[net] Deleting MAC DNAT rule failed with", zap.Error(err))
	}
}

func (o Ovsctl) DeletePortFromOVS(bridgeName, interfaceName string) error {
	// Disconnect external interface from its bridge.
	cmd := fmt.Sprintf("ovs-vsctl del-port %s %s", bridgeName, interfaceName)
	_, err := o.execcli.ExecuteCommand(cmd)
	if err != nil {
		logger.Error("Failed to disconnect interface", zap.String("from", interfaceName), zap.Error(err))
		return newErrorOvsctl(err.Error())
	}

	return nil
}
