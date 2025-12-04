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

func runAzCommand(params ...string) (string, error) {
	var out bytes.Buffer
	var stderr bytes.Buffer
	var err error
	fmt.Println("Running Azure CLI command ", strings.Join(params, " "))
	for i := 0; i < 3; i++ {
		cmd := exec.Command("az", params...)
		cmd.Stdout = &out
		cmd.Stderr = &stderr
		err = cmd.Run()
		if err == nil {
			break
		}
	}

	if err != nil {
		return "", errors.Wrap(err, "command failed "+stderr.String())
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

	result, err := runAzCommand("vmss", "list", "-g", resourceGroup, "--query", "[0].name", "-o", "tsv")
	if err != nil {
		fmt.Printf("Command failed with error: %s\n", err)
		os.Exit(1)
	}
	vmssName := strings.TrimSpace(result)

	result, err = runAzCommand("vmss", "show", "-g", resourceGroup, "-n", vmssName)
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
			ipConfigurations = []interface{}{}
			for i := 1; i <= secondaryConfigCount+1; i++ {
				ipConfig := make(map[string]interface{})
				for k, v := range primaryIPConfig {
					// Skip nil values.
					if v == nil {
						continue
					}

					// only the primary config needs loadBalancerBackendAddressPools. Azure doesn't allow
					// secondary IP configs to be associated load balancer backend pools.
					if i > 1 && k == "loadBalancerBackendAddressPools" {
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
				} else {
					ipConfigurations = append(ipConfigurations, ipConfig)
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

	_, err = runAzCommand("vmss", "update", "-g", resourceGroup, "-n", vmssName, "--set", fmt.Sprintf("virtualMachineProfile.networkProfile=%s", networkProfileJSON))
	if err != nil {
		fmt.Printf("Command failed with error: %s\n", err)
		os.Exit(1)
	}

	_, err = runAzCommand("vmss", "update-instances", "-g", resourceGroup, "-n", vmssName, "--instance-ids", "*")
	if err != nil {
		fmt.Printf("Command failed with error: %s\n", err)
		os.Exit(1)
	}
}
