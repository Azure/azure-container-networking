// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package networkcontainers

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/log"
)

func createOrUpdateInterface(createNetworkContainerRequest cns.CreateNetworkContainerRequest) error {
	exists, _ := interfaceExists(createNetworkContainerRequest.NetworkContainerid)

	if !exists {
		return createOrUpdateWithOperation(createNetworkContainerRequest, "CREATE")
	}

	return createOrUpdateWithOperation(createNetworkContainerRequest, "UPDATE")
}

func setWeakHostOnInterface(ipAddress string) error {
	interfaces, err := net.Interfaces()
	if err != nil {
		log.Printf("[Azure CNS] Unable to retrieve interfaces on machine. %+v", err)
		return err
	}

	var targetIface *net.Interface
	for _, iface := range interfaces {
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			addrStr := addr.String()
			ipv4Addr, _, err := net.ParseCIDR(addrStr)
			if err != nil {
				log.Printf("[Azure CNS] Unable to parse ip address on the interface %v.", err)
				continue
			}
			add := ipv4Addr.String()
			if strings.Compare(add, ipAddress) == 0 {
				targetIface = &iface
				break
			}
		}

		if targetIface != nil {
			break
		}
	}

	if targetIface == nil {
		errval := "[Azerrvalure CNS] Was not able to find the interface with ip " + ipAddress + " to enable weak host send/receive"
		log.Printf(errval)
		return errors.New(errval)
	}

	ethIndexString := strconv.Itoa(targetIface.Index)
	log.Printf("[Azure CNS] Going to setup weak host routing for interface with index[%v, %v]\n", targetIface.Index, ethIndexString)

	args := []string{"/C", "AzureNetworkContainer.exe", "/logpath", log.GetLogDirectory(),
		"/index",
		ethIndexString,
		"/operation",
		"WEAKHOSTROUTING",
		"/weakhostsend",
		"true",
		"/weakhostreceive",
		"true"}

	log.Printf("[Azure CNS] Going to enable weak host send/receive on interface: %v", args)
	c := exec.Command("cmd", args...)
	bytes, err := c.Output()

	if err == nil {
		log.Printf("[Azure CNS] Successfully updated weak host send/receive on interface %v.\n", string(bytes))
	} else {
		log.Printf("[Azure CNS] Received error while enable weak host send/receive on interface. %v - %v", err.Error(), string(bytes))
		return err
	}

	return nil
}

func createOrUpdateWithOperation(createNetworkContainerRequest cns.CreateNetworkContainerRequest, operation string) error {
	if _, err := os.Stat("./AzureNetworkContainer.exe"); err != nil {
		if os.IsNotExist(err) {
			return errors.New("[Azure CNS] Unable to find AzureNetworkContainer.exe. Cannot continue")
		}
	}

	if createNetworkContainerRequest.IPConfiguration.IPSubnet.IPAddress == "" {
		return errors.New("[Azure CNS] IPAddress in IPConfiguration of createNetworkContainerRequest is nil")
	}

	var dnsServers string

	for _, element := range createNetworkContainerRequest.IPConfiguration.DNSServers {
		dnsServers += element + ","
	}

	if dnsServers != "" && dnsServers[len(dnsServers)-1] == ',' {
		dnsServers = dnsServers[:len(dnsServers)-1]
	}

	ipv4AddrCidr := fmt.Sprintf("%v/%d", createNetworkContainerRequest.IPConfiguration.IPSubnet.IPAddress, createNetworkContainerRequest.IPConfiguration.IPSubnet.PrefixLength)
	log.Printf("[Azure CNS] Created ipv4Cidr as %v", ipv4AddrCidr)
	ipv4Addr, _, err := net.ParseCIDR(ipv4AddrCidr)
	ipv4NetInt := net.CIDRMask((int)(createNetworkContainerRequest.IPConfiguration.IPSubnet.PrefixLength), 32)
	log.Printf("[Azure CNS] Created netmask as %v", ipv4NetInt)
	ipv4NetStr := fmt.Sprintf("%d.%d.%d.%d", ipv4NetInt[0], ipv4NetInt[1], ipv4NetInt[2], ipv4NetInt[3])
	log.Printf("[Azure CNS] Created netmask in string format %v", ipv4NetStr)

	args := []string{"/C", "AzureNetworkContainer.exe", "/logpath", log.GetLogDirectory(),
		"/name",
		createNetworkContainerRequest.NetworkContainerid,
		"/operation",
		operation,
		"/ip",
		ipv4Addr.String(),
		"/netmask",
		ipv4NetStr,
		"/gateway",
		createNetworkContainerRequest.IPConfiguration.GatewayIPAddress,
		"/dns",
		dnsServers,
		"/weakhostsend",
		"true",
		"/weakhostreceive",
		"true"}

	log.Printf("[Azure CNS] Going to create/update network loopback adapter: %v", args)
	c := exec.Command("cmd", args...)
	bytes, err := c.Output()

	if err == nil {
		log.Printf("[Azure CNS] Successfully created network loopback adapter %v.\n", string(bytes))
	} else {
		log.Printf("Received error while Creating a Network Container %v %v", err.Error(), string(bytes))
	}

	return err
}

func deleteInterface(networkContainerID string) error {

	if _, err := os.Stat("./AzureNetworkContainer.exe"); err != nil {
		if os.IsNotExist(err) {
			return errors.New("[Azure CNS] Unable to find AzureNetworkContainer.exe. Cannot continue")
		}
	}

	if networkContainerID == "" {
		return errors.New("[Azure CNS] networkContainerID is nil")
	}

	args := []string{"/C", "AzureNetworkContainer.exe", "/logpath", log.GetLogDirectory(),
		"/name",
		networkContainerID,
		"/operation",
		"DELETE"}

	log.Printf("[Azure CNS] Going to delete network loopback adapter: %v", args)
	c := exec.Command("cmd", args...)
	bytes, err := c.Output()

	if err == nil {
		log.Printf("[Azure CNS] Successfully deleted network container %v.\n", string(bytes))
	} else {
		log.Printf("Received error while deleting a Network Container %v %v", err.Error(), string(bytes))
		return err
	}
	return nil
}
