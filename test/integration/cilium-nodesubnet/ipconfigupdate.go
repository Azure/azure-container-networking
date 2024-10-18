package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

func runCommand(command string) (string, error) {
	var out bytes.Buffer
	var stderr bytes.Buffer
	var err error
	for i := 0; i < 3; i++ {
		cmd := exec.Command("bash", "-c", command)
		cmd.Stdout = &out
		cmd.Stderr = &stderr
		err = cmd.Run()
		if err == nil {
			break
		}
	}

	if err != nil {
		return "", errors.Wrap(err, fmt.Sprintf("command %s failed ", command))
	}

	return out.String(), nil
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func main() {
	resourceGroup := os.Getenv("RESOURCE_GROUP")
	if resourceGroup == "" {
		fmt.Println("RESOURCE_GROUP environment variable is required")
		os.Exit(1)
	}

	secondaryConfigCountStr := os.Getenv("SECONDARY_CONFIG_COUNT")
	if secondaryConfigCountStr == "" {
		secondaryConfigCountStr = "64"
	}

	secondaryConfigCount, err := strconv.Atoi(secondaryConfigCountStr)
	if err != nil {
		fmt.Printf("Invalid value for SECONDARY_CONFIG_COUNT: %s\n", secondaryConfigCountStr)
		os.Exit(1)
	}

	command := fmt.Sprintf("az vmss list -g %s --query '[0].name' -o tsv", resourceGroup)
	result, err := runCommand(command)
	if err != nil {
		fmt.Printf("Command failed with error: %s\n", err)
		os.Exit(1)
	}
	vmssName := strings.TrimSpace(result)

	command = fmt.Sprintf("az vmss show -g %s -n %s", resourceGroup, vmssName)
	result, err = runCommand(command)
	if err != nil {
		fmt.Printf("Command failed with error: %s\n", err)
		os.Exit(1)
	}

	var vmssInfo map[string]interface{}
	err = json.Unmarshal([]byte(result), &vmssInfo)
	if err != nil {
		fmt.Printf("Failed to parse JSON: %s\n", err)
		os.Exit(1)
	}

	networkProfile := vmssInfo["virtualMachineProfile"].(map[string]interface{})["networkProfile"].(map[string]interface{})
	networkInterfaceConfigurations := networkProfile["networkInterfaceConfigurations"].([]interface{})

	var usedIPConfigNames []string
	var secondaryConfigs []interface{}

	for _, nicConfig := range networkInterfaceConfigurations {
		nicConfigMap := nicConfig.(map[string]interface{})
		ipConfigurations := nicConfigMap["ipConfigurations"].([]interface{})
		var primaryIPConfig map[string]interface{}
		for _, ipConfig := range ipConfigurations {
			ipConfigMap := ipConfig.(map[string]interface{})
			usedIPConfigNames = append(usedIPConfigNames, ipConfigMap["name"].(string))
			if ipConfigMap["primary"].(bool) {
				primaryIPConfig = ipConfigMap
			}
		}

		if primaryIPConfig != nil {
			for i := 2; i <= secondaryConfigCount+1; i++ {
				ipConfig := make(map[string]interface{})
				for k, v := range primaryIPConfig {
					if k == "loadBalancerBackendAddressPools" {
						continue
					}
					ipConfig[k] = v
				}

				ipConfigName := fmt.Sprintf("ipconfig%d", i)
				if !contains(usedIPConfigNames, ipConfigName) {
					ipConfig["name"] = ipConfigName
					ipConfig["primary"] = false
					usedIPConfigNames = append(usedIPConfigNames, ipConfigName)
					secondaryConfigs = append(secondaryConfigs, ipConfig)
				}
			}
		}

		nicConfigMap["ipConfigurations"] = append(ipConfigurations, secondaryConfigs...)
	}

	networkProfileJSON, err := json.Marshal(networkProfile)
	if err != nil {
		fmt.Printf("Failed to marshal JSON: %s\n", err)
		os.Exit(1)
	}

	escapedNetworkProfileJSON := strings.ReplaceAll(string(networkProfileJSON), `\`, `\\`)
	escapedNetworkProfileJSON = strings.ReplaceAll(escapedNetworkProfileJSON, `'`, `\'`)

	command = fmt.Sprintf("az vmss update -g %s -n %s --set virtualMachineProfile.networkProfile='%s'", resourceGroup, vmssName, escapedNetworkProfileJSON)
	fmt.Println("Command to update VMSS: ", command)
	_, err = runCommand(command)
	if err != nil {
		fmt.Printf("Command failed with error: %s\n", err)
		os.Exit(1)
	}

	command = fmt.Sprintf("az vmss update-instances -g %s -n %s --instance-ids '*'", resourceGroup, vmssName)
	fmt.Println("Command to update VMSS instances: ", command)
	_, err = runCommand(command)
	if err != nil {
		fmt.Printf("Command failed with error: %s\n", err)
		os.Exit(1)
	}
}
