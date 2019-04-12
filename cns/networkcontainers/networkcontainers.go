// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package networkcontainers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/types"
	"io/ioutil"
	"net"
	"os"
	"os/exec"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/log"
	"github.com/containernetworking/cni/libcni"
	"github.com/containernetworking/cni/pkg/invoke"
	cniTypes "github.com/containernetworking/cni/pkg/types"
)

const (
	versionStr              = "cniVersion"
	pluginsStr              = "plugins"
	nameStr                 = "name"
	k8sPodNamespaceStr      = "K8S_POD_NAMESPACE"
	k8sPodNameStr           = "K8S_POD_NAME"
	k8sPodInfraContainerStr = "K8S_POD_INFRA_CONTAINER_ID"
	cniAdd                  = "ADD"
	cniDelete               = "DEL"
	cniUpdate               = "UPDATE"
)

// NetworkContainers can be used to perform operations on network containers.
type NetworkContainers struct {
	logpath string
}

// NetPluginConfiguration represent network plugin configuration that is used during CNI ADD/DELETE/UPDATE operation
type NetPluginConfiguration struct {
	path              string
	networkConfigPath string
}

// NewNetPluginConfiguration create a new netplugin configuration.
func NewNetPluginConfiguration(binPath, configPath string) *NetPluginConfiguration {
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

// This function gets the flattened network configuration (compliant with azure cni) in byte array format
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
	var flatNetConfigMap map[string]interface{}
	if pluginsSection, ok := configMap[pluginsStr]; ok && len(pluginsSection.([]interface{})) > 0 {
		flatNetConfigMap = pluginsSection.([]interface{})[0].(map[string]interface{})
	}

	if flatNetConfigMap == nil {
		msg := "[Azure CNS] " + pluginsStr + " section of the network configuration cannot be empty."
		log.Printf(msg)
		return nil, errors.New(msg)
	}

	// insert version and name fields
	flatNetConfigMap[versionStr] = configMap[versionStr].(string)
	flatNetConfigMap[nameStr] = configMap[nameStr].(string)

	// convert into bytes format
	netConfig, err := json.Marshal(flatNetConfigMap)
	if err != nil {
		log.Printf("[Azure CNS] Failed to marshal flat network configuration with error %v", err)
		return nil, err
	}

	return netConfig, nil
}

func args(action, path string, rt *libcni.RuntimeConf) *invoke.Args {
	return &invoke.Args{
		Command:     action,
		ContainerID: rt.ContainerID,
		NetNS:       rt.NetNS,
		PluginArgs:  rt.Args,
		IfName:      rt.IfName,
		Path:        path,
	}
}

// pluginErr - Check for command.Run() error and if that is nil, then we check for plugin error
func pluginErr(err error, output []byte) error {
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			emsg := types.Error{}
			if err := json.Unmarshal(output, &emsg); err != nil {
				emsg.Msg = fmt.Sprintf("netplugin failed but error parsing its diagnostic message %s: %+v", string(output), err)
			}

			return &emsg
		}
	} else if len(output) > 0 {
		var cniError cniTypes.Error
		if err = json.Unmarshal(output, &cniError); err == nil && cniError.Code != 0 {
			return fmt.Errorf("netplugin completed with error: %+v", cniError)
		}
	}

	return err
}

func execPlugin(rt *libcni.RuntimeConf, netconf []byte, operation, path string) error {
	switch operation {
	case cniAdd:
		fallthrough
	case cniDelete:
		fallthrough
	case cniUpdate:
		environ := args(operation, path, rt).AsEnv()
		log.Printf("[Azure CNS] CNI called with environ variables %v", environ)
		stdout := &bytes.Buffer{}
		command := exec.Command(path + string(os.PathSeparator) + "azure-vnet")
		command.Env = environ
		command.Stdin = bytes.NewBuffer(netconf)
		command.Stdout = stdout
		return pluginErr(command.Run(), stdout.Bytes())
	default:
		return fmt.Errorf("[Azure CNS] Invalid operation being passed to CNI: %s", operation)
	}
}

// Attach - attaches network container to network.
func (cn *NetworkContainers) Attach(podInfo cns.KubernetesPodInfo, dockerContainerid string, netPluginConfig *NetPluginConfiguration) error {
	log.Printf("[Azure CNS] NetworkContainers.Attach called")
	err := configureNetworkContainerNetworking(cniAdd, podInfo.PodName, podInfo.PodNamespace, dockerContainerid, netPluginConfig)
	log.Printf("[Azure CNS] NetworkContainers.Attach finished")
	return err
}

// Detach - detaches network container from network.
func (cn *NetworkContainers) Detach(podInfo cns.KubernetesPodInfo, dockerContainerid string, netPluginConfig *NetPluginConfiguration) error {
	log.Printf("[Azure CNS] NetworkContainers.Detach called")
	err := configureNetworkContainerNetworking(cniDelete, podInfo.PodName, podInfo.PodNamespace, dockerContainerid, netPluginConfig)
	log.Printf("[Azure CNS] NetworkContainers.Detach finished")
	return err
}
