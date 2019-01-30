// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package networkcontainers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/log"
)

const (
	VersionStr        string = "cniVersion"
	PluginsStr        string = "plugins"
	NameStr           string = "name"
	K8PodNameSpaceStr string = "K8S_POD_NAMESPACE"
	K8PodNameStr      string = "K8S_POD_NAME"
)

// NetworkContainers can be used to perform operations on network containers.
type NetworkContainers struct {
	logpath string
}

// NetPluginConfiguration represent network plugin configuration that is used during Update operation
type NetPluginConfiguration struct {
	path              string
	networkConfigPath string
}

// NewNetPluginConfiguration create a new netplugin configuration.
func NewNetPluginConfiguration(binPath string, configPath string) *NetPluginConfiguration {
	return &NetPluginConfiguration{
		path:              binPath,
		networkConfigPath: configPath,
	}
}

func interfaceExists(iFaceName string) (bool, error) {
	_, err := net.InterfaceByName(iFaceName)
	if err != nil {
		errMsg := fmt.Sprintf("[Azure CNS] Unable to get interface by name %v, %v", iFaceName, err)
		log.Printf(errMsg)
		return false, errors.New(errMsg)
	}

	return true, nil
}

// Create creates a network container.
func (cn *NetworkContainers) Create(createNetworkContainerRequest cns.CreateNetworkContainerRequest) error {
	log.Printf("[Azure CNS] NetworkContainers.Create called")
	err := createOrUpdateInterface(createNetworkContainerRequest)
	if err == nil {
		err = setWeakHostOnInterface(createNetworkContainerRequest.PrimaryInterfaceIdentifier)
	}
	log.Printf("[Azure CNS] NetworkContainers.Create finished.")
	return err
}

// Update updates a network container.
func (cn *NetworkContainers) Update(createNetworkContainerRequest cns.CreateNetworkContainerRequest, netpluginConfig *NetPluginConfiguration) error {
	log.Printf("[Azure CNS] NetworkContainers.Update called")
	err := updateInterface(createNetworkContainerRequest, netpluginConfig)
	if err == nil {
		err = setWeakHostOnInterface(createNetworkContainerRequest.PrimaryInterfaceIdentifier)
	}
	log.Printf("[Azure CNS] NetworkContainers.Update finished.")
	return err
}

// Delete deletes a network container.
func (cn *NetworkContainers) Delete(networkContainerID string) error {
	log.Printf("[Azure CNS] NetworkContainers.Delete called")
	err := deleteInterface(networkContainerID)
	log.Printf("[Azure CNS] NetworkContainers.Delete finished.")
	return err
}

// This function gets the flattened network configuration (compliant with azure cni) in bytes array format
func getNetworkConfig(configFilePath string) ([]byte, error) {
	content, err := ioutil.ReadFile(configFilePath)
	if err != nil {
		return nil, err
	}

	var configMap map[string]interface{}
	if err = json.Unmarshal(content, &configMap); err != nil {
		log.Printf("[Azure CNS] Failed to unmarshal network configuration with error %v", err)
		return nil, err
	}

	// Get the plugins section
	pluginsSection := configMap[PluginsStr].([]interface{})
	flatNetConfigMap := pluginsSection[0].(map[string]interface{})

	if flatNetConfigMap == nil {
		msg := "[Azure CNS] " + PluginsStr + " section of the network configuration cannot be empty."
		log.Printf(msg)
		return nil, errors.New(msg)
	}

	// insert version and name fields
	flatNetConfigMap[VersionStr] = configMap[VersionStr].(string)
	flatNetConfigMap[NameStr] = configMap[NameStr].(string)

	// convert into bytes format
	netConfig, err := json.Marshal(flatNetConfigMap)
	if err != nil {
		log.Printf("[Azure CNS] Failed to marshal flat network configuration with error %v", err)
		return nil, err
	}

	return netConfig, nil
}
