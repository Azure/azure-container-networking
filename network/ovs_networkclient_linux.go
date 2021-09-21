// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package network

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/network/epcommon"
	"github.com/Azure/azure-container-networking/ovsctl"
)

var errorOVSNetworkClient = errors.New("OVSNetworkClient Error")

func newErrorOVSNetworkClient(errStr string) error {
	return fmt.Errorf("%w : %s", errorOVSNetworkClient, errStr)
}

type OVSNetworkClient struct {
	bridgeName        string
	hostInterfaceName string
	ovsctlClient      ovsctl.OvsInterface
}

const (
	ovsConfigFile = "/etc/default/openvswitch-switch"
	ovsOpt        = "OVS_CTL_OPTS='--delete-bridges'"
)

func updateOVSConfig(option string) error {
	f, err := os.OpenFile(ovsConfigFile, os.O_APPEND|os.O_RDWR, 0o666)
	if err != nil {
		log.Printf("Error while opening ovs config %v", err)
		return err
	}

	defer f.Close()

	buf := new(bytes.Buffer)
	buf.ReadFrom(f)
	contents := buf.String()

	conSplit := strings.Split(contents, "\n")
	for _, existingOption := range conSplit {
		if option == existingOption {
			log.Printf("Not updating ovs config. Found option already written")
			return nil
		}
	}

	log.Printf("writing ovsconfig option %v", option)

	if _, err = f.WriteString(option); err != nil {
		log.Printf("Error while writing ovs config %v", err)
		return err
	}

	return nil
}

func (client *OVSNetworkClient) AddRoutes(nwInfo *NetworkInfo, interfaceName string) error {
	return nil
}

func NewOVSClient(bridgeName, hostInterfaceName string, ovsctlClient ovsctl.OvsInterface) *OVSNetworkClient {
	ovsClient := &OVSNetworkClient{
		bridgeName:        bridgeName,
		hostInterfaceName: hostInterfaceName,
		ovsctlClient:      ovsctlClient,
	}

	return ovsClient
}

func (client *OVSNetworkClient) CreateBridge() error {
	var err error

	if err = client.ovsctlClient.CreateOVSBridge(client.bridgeName); err != nil {
		return err
	}

	defer func() {
		if err != nil {
			client.DeleteBridge()
		}
	}()

	if err := epcommon.DisableRAForInterface(client.bridgeName); err != nil {
		return err
	}

	return updateOVSConfig(ovsOpt)
}

func (client *OVSNetworkClient) DeleteBridge() error {
	if err := client.ovsctlClient.DeleteOVSBridge(client.bridgeName); err != nil {
		log.Printf("Deleting ovs bridge failed with error %v", err)
	}

	return nil
}

func (client *OVSNetworkClient) AddL2Rules(extIf *externalInterface) error {
	mac := extIf.MacAddress.String()
	macHex := strings.Replace(mac, ":", "", -1)

	ofport, err := client.ovsctlClient.GetOVSPortNumber(client.hostInterfaceName)
	if err != nil {
		return err
	}

	// Arp SNAT Rule
	log.Printf("[ovs] Adding ARP SNAT rule for egress traffic on interface %v", client.hostInterfaceName)
	if err := client.ovsctlClient.AddArpSnatRule(client.bridgeName, mac, macHex, ofport); err != nil {
		return err
	}

	log.Printf("[ovs] Adding DNAT rule for ingress ARP traffic on interface %v.", client.hostInterfaceName)
	err = client.ovsctlClient.AddArpDnatRule(client.bridgeName, ofport, macHex)
	if err != nil {
		return newErrorOVSNetworkClient(err.Error())
	}

	return nil
}

func (client *OVSNetworkClient) DeleteL2Rules(extIf *externalInterface) {
	if err := client.ovsctlClient.DeletePortFromOVS(client.bridgeName, client.hostInterfaceName); err != nil {
		log.Printf("[ovs] Deletion of interface %v from bridge %v failed", client.hostInterfaceName, client.bridgeName)
	}
}

func (client *OVSNetworkClient) SetBridgeMasterToHostInterface() error {
	err := client.ovsctlClient.AddPortOnOVSBridge(client.hostInterfaceName, client.bridgeName, 0)
	if err != nil {
		return newErrorOVSNetworkClient(err.Error())
	}
	return nil
}

func (client *OVSNetworkClient) SetHairpinOnHostInterface(enable bool) error {
	return nil
}
