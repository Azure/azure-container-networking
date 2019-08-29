package network

import (
	"bytes"
	"os"
	"strings"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/ovsctl"
)

type OVSNetworkClient struct {
	bridgeName        string
	hostInterfaceName string
}

const (
	ovsConfigFile = "/etc/default/openvswitch-switch"
	ovsOpt        = "OVS_CTL_OPTS='--delete-bridges'"
)

func updateOVSConfig(option string) error {
	f, err := os.OpenFile(ovsConfigFile, os.O_APPEND|os.O_RDWR, 0666)
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

func NewOVSClient(bridgeName, hostInterfaceName string) *OVSNetworkClient {
	ovsClient := &OVSNetworkClient{
		bridgeName:        bridgeName,
		hostInterfaceName: hostInterfaceName,
	}

	return ovsClient
}

func (client *OVSNetworkClient) CreateBridge() error {
	var err error

	if err = ovsctl.CreateOVSBridge(client.bridgeName); err != nil {
		return err
	}

	defer func() {
		if err != nil {
			client.DeleteBridge()
		}
	}()

	return updateOVSConfig(ovsOpt)
}

func (client *OVSNetworkClient) DeleteBridge() error {
	if err := ovsctl.DeleteOVSBridge(client.bridgeName); err != nil {
		log.Printf("Deleting ovs bridge failed with error %v", err)
	}

	return nil
}

func (client *OVSNetworkClient) AddL2Rules(extIf *externalInterface) error {
	mac := extIf.MacAddress.String()
	macHex := strings.Replace(mac, ":", "", -1)

	ofport, err := ovsctl.GetOVSPortNumber(client.hostInterfaceName)
	if err != nil {
		return err
	}

	// Arp SNAT Rule
	log.Printf("[ovs] Adding ARP SNAT rule for egress traffic on interface %v", client.hostInterfaceName)
	if err := ovsctl.AddArpSnatRule(client.bridgeName, mac, macHex, ofport); err != nil {
		return err
	}

	log.Printf("[ovs] Adding DNAT rule for ingress ARP traffic on interface %v.", client.hostInterfaceName)
	if err := ovsctl.AddArpDnatRule(client.bridgeName, ofport, macHex); err != nil {
		return err
	}

	return nil
}

func (client *OVSNetworkClient) DeleteL2Rules(extIf *externalInterface) {
	ovsctl.DeletePortFromOVS(client.bridgeName, client.hostInterfaceName)
}

func (client *OVSNetworkClient) SetBridgeMasterToHostInterface() error {
	return ovsctl.AddPortOnOVSBridge(client.hostInterfaceName, client.bridgeName, 0)
}

func (client *OVSNetworkClient) SetHairpinOnHostInterface(enable bool) error {
	return nil
}
