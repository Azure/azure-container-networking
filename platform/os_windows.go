// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package platform

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Microsoft/hcsshim"
)

const (
	// CNMRuntimePath is the path where CNM state files are stored.
	CNMRuntimePath = ""

	// CNIRuntimePath is the path where CNI state files are stored.
	CNIRuntimePath = ""

	// CNI runtime path on a Kubernetes cluster
	K8SCNIRuntimePath = "C:\\k\\azurecni\\bin"

	// Network configuration file path on a Kubernetes cluster
	K8SNetConfigPath = "C:\\k\\azurecni\\netconf"

	// CNSRuntimePath is the path where CNS state files are stored.
	CNSRuntimePath = ""

	// NPMRuntimePath is the path where NPM state files are stored.
	NPMRuntimePath = ""

	// DNCRuntimePath is the path where DNC state files are stored.
	DNCRuntimePath = ""
)

// GetOSInfo returns OS version information.
func GetOSInfo() string {
	return "windows"
}

// GetLastRebootTime returns the last time the system rebooted.
func GetLastRebootTime() (time.Time, error) {
	out, err := exec.Command("cmd", "/c", "wmic os get lastbootuptime").Output()
	if err != nil {
		log.Printf("Failed to query wmic os get lastbootuptime, err: %v", err)
		return time.Time{}.UTC(), err
	}

	lastBootupTime := strings.Split(strings.TrimSpace(string(out)), "\n")
	if strings.TrimSpace(lastBootupTime[0]) != "LastBootUpTime" || len(lastBootupTime) != 2 {
		log.Printf("Failed to retrieve boot time")
		return time.Time{}.UTC(), fmt.Errorf("Failed to retrieve boot time with 'wmic os get lastbootuptime'")
	}
	systemBootupTime := strings.Split(lastBootupTime[1], ".")[0]

	// The systembootuptime is in the format YYYYMMDDHHMMSS
	bootYear := systemBootupTime[0:4]
	bootMonth := systemBootupTime[4:6]
	bootDay := systemBootupTime[6:8]
	bootHour := systemBootupTime[8:10]
	bootMin := systemBootupTime[10:12]
	bootSec := systemBootupTime[12:14]
	systemBootTime := bootYear + "-" + bootMonth + "-" + bootDay + " " + bootHour + ":" + bootMin + ":" + bootSec

	log.Printf("Formatted Boot time: %s", systemBootTime)

	// Parse the boot time.
	layout := "2006-01-02 15:04:05"
	rebootTime, err := time.ParseInLocation(layout, systemBootTime, time.Local)
	if err != nil {
		log.Printf("Failed to parse boot time, err:%v", err)
		return time.Time{}.UTC(), err
	}

	return rebootTime.UTC(), nil
}

func ExecuteCommand(command string) (string, error) {
	log.Printf("[Azure-Utils] %s", command)

	var stderr bytes.Buffer
	var out bytes.Buffer
	cmd := exec.Command("cmd", "/c", command)
	cmd.Stderr = &stderr
	cmd.Stdout = &out

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("%s:%s", err.Error(), stderr.String())
	}

	return out.String(), nil
}

func SetOutboundSNAT(subnet string) error {
	return nil
}

// ClearNetworkConfiguration clears the azure-vnet.json contents.
// This will be called only when reboot is detected - This is windows specific
func ClearNetworkConfiguration() (bool, error) {
	jsonStore := CNIRuntimePath + "azure-vnet.json"
	log.Printf("Deleting the json store %s", jsonStore)
	cmd := exec.Command("cmd", "/c", "del", jsonStore)

	if err := cmd.Run(); err != nil {
		log.Printf("Error deleting the json store %s", jsonStore)
		return true, err
	}

	return true, nil
}

func KillProcessByName(processName string) {
	cmd := fmt.Sprintf("taskkill /IM %v /F", processName)
	ExecuteCommand(cmd)
}

// CreateExtHnsNetwork creates ext HNS network if not present.
func CreateExtHnsNetwork(createExtNetworkType string) error {
	if len(strings.TrimSpace(createExtNetworkType)) == 0 {
		return nil
	}

	if !strings.EqualFold(createExtNetworkType, "l2bridge") &&
		!strings.EqualFold(createExtNetworkType, "l2tunnel") {
		return fmt.Errorf("Invalid ext hns network type %s", createExtNetworkType)
	}

	log.Printf("[Azure CNS] CreateExtHnsNetwork")
	extHnsNetwork, _ := hcsshim.GetHNSNetworkByName("ext")

	if extHnsNetwork != nil {
		log.Printf("[Azure CNS] Found existing ext hns network with type: %s", extHnsNetwork.Type)
		if !strings.EqualFold(createExtNetworkType, extHnsNetwork.Type) {
			return fmt.Errorf("Network type mismatch with existing network: %s", extHnsNetwork.Type)
		}

		return nil
	}

	// create new hns network
	log.Printf("[Azure CNS] Creating ext hns network with type %s", createExtNetworkType)

	hnsNetwork := &hcsshim.HNSNetwork{
		Name: "ext",
		Type: createExtNetworkType,
	}

	hnsSubnet := hcsshim.Subnet{
		AddressPrefix:  "192.168.255.0/30",
		GatewayAddress: "192.168.255.1",
	}

	hnsNetwork.Subnets = append(hnsNetwork.Subnets, hnsSubnet)

	// Marshal the request.
	buffer, err := json.Marshal(hnsNetwork)
	if err != nil {
		return err
	}
	hnsRequest := string(buffer)

	// Create the HNS network.
	log.Printf("[Azure CNS] HNSNetworkRequest POST request:%+v", hnsRequest)
	hnsResponse, err := hcsshim.HNSNetworkRequest("POST", "", hnsRequest)
	log.Printf("[Azure CNS] HNSNetworkRequest POST response:%+v err:%v.", hnsResponse, err)

	return err
}
