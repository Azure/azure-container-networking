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
	"sync"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/log"
	"github.com/containernetworking/cni/libcni"
)

var loopbackOperationLock = &sync.Mutex{}

func createOrUpdateInterface(createNetworkContainerRequest cns.CreateNetworkContainerRequest) error {
	// Create Operation is only supported for WebApps only on Windows
	if createNetworkContainerRequest.NetworkContainerType != cns.WebApps {
		log.Printf("[Azure CNS] Operation not supported for container type %v", createNetworkContainerRequest.NetworkContainerType)
		return nil
	}

	if exists, _ := interfaceExists(createNetworkContainerRequest.NetworkContainerid); !exists {
		return createOrUpdateWithOperation(createNetworkContainerRequest, "CREATE")
	}

	return createOrUpdateWithOperation(createNetworkContainerRequest, "UPDATE")
}

func updateInterface(createNetworkContainerRequest cns.CreateNetworkContainerRequest, netpluginConfig *NetPluginConfiguration) error {
	return nil
}

func setWeakHostOnInterface(ipAddress, ncID string) error {
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

	args := []string{"/C", "AzureNetworkContainer.exe", "/logpath", log.GetLogDirectory(),
		"/index",
		ethIndexString,
		"/operation",
		"WEAKHOSTROUTING",
		"/weakhostsend",
		"true",
		"/weakhostreceive",
		"true"}

	log.Printf("[Azure CNS] Going to enable weak host send/receive on interface: %v for NC: %s", args, ncID)
	c := exec.Command("cmd", args...)

	bytes, err := c.Output()

	if err == nil {
		log.Printf("[Azure CNS] Successfully updated weak host send/receive for NC: %s on interface %v",
			ncID, string(bytes))
	} else {
		log.Printf("[Azure CNS] Failed to update weak host send/receive for NC: %s. Error: %v. Output: %v",
			ncID, err.Error(), string(bytes))
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
		"/weakhostsend",
		"true",
		"/weakhostreceive",
		"true"}

	c := exec.Command("cmd", args...)

	loopbackOperationLock.Lock()
	log.Printf("[Azure CNS] Going to create/update network loopback adapter: %v", args)
	bytes, err := c.Output()
	if err == nil {
		err = setWeakHostOnInterface(createNetworkContainerRequest.PrimaryInterfaceIdentifier,
			createNetworkContainerRequest.NetworkContainerid)
	}
	loopbackOperationLock.Unlock()

	if err == nil {
		log.Printf("[Azure CNS] Successfully created network loopback adapter for NC: %s. Output:%v.",
			createNetworkContainerRequest.NetworkContainerid, string(bytes))
	} else {
		log.Printf("Failed to create/update Network Container: %s. Error: %v. Output: %v",
			createNetworkContainerRequest.NetworkContainerid, err.Error(), string(bytes))
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

	c := exec.Command("cmd", args...)

	loopbackOperationLock.Lock()
	log.Printf("[Azure CNS] Going to delete network loopback adapter: %v", args)
	bytes, err := c.Output()
	loopbackOperationLock.Unlock()

	if err == nil {
		log.Printf("[Azure CNS] Successfully deleted network container: %s. Output: %v.",
			networkContainerID, string(bytes))
	} else {
		log.Printf("Failed to delete Network Container: %s. Error:%v. Output:%v",
			networkContainerID, err.Error(), string(bytes))
		return err
	}
	return nil
}

func configureNetworkContainerNetworking(operation, podName, podNamespace, dockerContainerid string, netPluginConfig *NetPluginConfiguration) (err error) {
	cniRtConf := &libcni.RuntimeConf{
		ContainerID: dockerContainerid,
		NetNS:       "none",
		IfName:      "eth0",
		Args: [][2]string{
			{k8sPodNamespaceStr, podNamespace},
			{k8sPodNameStr, podName}}}
	log.Printf("[Azure CNS] run time conf info %+v", cniRtConf)

	netConfig, err := getNetworkConfig(netPluginConfig.networkConfigPath)
	if err != nil {
		log.Printf("[Azure CNS] Failed to build network configuration with error %v", err)
		return err
	}

	log.Printf("[Azure CNS] network configuration info %v", string(netConfig))

	if err = execPlugin(cniRtConf, netConfig, operation, netPluginConfig.path); err != nil {
		log.Printf("[Azure CNS] Failed to invoke CNI with %s operation on docker container %s with error %v", operation, dockerContainerid, err)
	}

	return err
}
