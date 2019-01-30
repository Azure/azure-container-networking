// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package networkcontainers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/types"
	"os"
	"os/exec"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/log"
	"github.com/containernetworking/cni/libcni"
	"github.com/containernetworking/cni/pkg/invoke"
)

func createOrUpdateInterface(createNetworkContainerRequest cns.CreateNetworkContainerRequest) error {
	return nil
}

func setWeakHostOnInterface(ipAddress string) error {
	return nil
}

func updateInterface(createNetworkContainerRequest cns.CreateNetworkContainerRequest, netpluginConfig *NetPluginConfiguration) error {
	log.Printf("[Azure CNS] update interface operation called.")

	// Currently update via CNI is only supported for ACI type
	if createNetworkContainerRequest.NetworkContainerType != cns.AzureContainerInstance {
		log.Printf("[Azure CNS] operation is only supported for AzureContainerInstance types.")
		return nil
	}

	if _, err := os.Stat(netpluginConfig.path); err != nil {
		if os.IsNotExist(err) {
			msg := "[Azure CNS] Unable to find " + netpluginConfig.path + ", cannot continue."
			log.Printf(msg)
			return errors.New(msg)
		}
	}

	var podInfo cns.KubernetesPodInfo
	err := json.Unmarshal(createNetworkContainerRequest.OrchestratorContext, &podInfo)
	if err != nil {
		log.Printf("[Azure CNS] Unmarshalling %s failed with error %v", createNetworkContainerRequest.NetworkContainerType, err)
		return err
	}

	log.Printf("[Azure CNS] Going to update networkign for the pod with Pod info %+v", podInfo)

	rt := &libcni.RuntimeConf{
		ContainerID: "", // Not needed for CNI update operation
		NetNS:       "", // Not needed for CNI update operation
		IfName:      createNetworkContainerRequest.NetworkContainerid,
		Args: [][2]string{
			{K8PodNameSpaceStr, podInfo.PodNamespace},
			{K8PodNameStr, podInfo.PodName},
		},
	}

	log.Printf("[Azure CNS] run time configuration for CNI plugin info %+v", rt)

	netConfig, err := getNetworkConfig(netpluginConfig.networkConfigPath)
	if err != nil {
		log.Printf("[Azure CNS] Failed to build network configuration with error %v", err)
		return err
	}

	log.Printf("[Azure CNS] network configuration info %v", string(netConfig))

	err = execPlugin(rt, netConfig, netpluginConfig.path)
	if err != nil {
		log.Printf("[Azure CNS] Failed to update network with error %v", err)
		return err
	}

	return nil
}

func deleteInterface(networkContainerID string) error {
	return nil
}

func execPlugin(rt *libcni.RuntimeConf, netconf []byte, path string) error {
	environ := args("UPDATE", rt).AsEnv()
	log.Printf("[Azure CNS] CNI called with environ variables %v", environ)
	stdout := &bytes.Buffer{}
	command := exec.Command(path)
	command.Env = environ
	command.Stdin = bytes.NewBuffer(netconf)
	command.Stdout = stdout
	command.Stderr = os.Stderr
	return pluginErr(command.Run(), stdout.Bytes())
}

// Environment variables
func args(action string, rt *libcni.RuntimeConf) *invoke.Args {
	return &invoke.Args{
		Command:     action,
		ContainerID: rt.ContainerID,
		NetNS:       rt.NetNS,
		PluginArgs:  rt.Args,
		IfName:      rt.IfName,
		Path:        "",
	}
}

func pluginErr(err error, output []byte) error {
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			emsg := types.Error{}
			if err := json.Unmarshal(output, &emsg); err != nil {
				emsg.Msg = fmt.Sprintf("netplugin failed but error parsing its diagnostic message %s: %+v", string(output), err)
			}

			return &emsg
		}
	}

	return err
}
