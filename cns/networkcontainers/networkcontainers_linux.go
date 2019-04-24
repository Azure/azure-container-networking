// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package networkcontainers

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/log"
	"github.com/containernetworking/cni/libcni"
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

	if netpluginConfig == nil {
		err := errors.New("Network plugin configuration cannot be nil.")
		log.Printf("[Azure CNS] Update interface failed with error %v", err)
		return err
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

	log.Printf("[Azure CNS] Going to update networking for the pod with Pod info %+v", podInfo)

	rt := &libcni.RuntimeConf{
		ContainerID: "", // Not needed for CNI update operation
		NetNS:       "", // Not needed for CNI update operation
		IfName:      createNetworkContainerRequest.NetworkContainerid,
		Args: [][2]string{
			{k8sPodNamespaceStr, podInfo.PodNamespace},
			{k8sPodNameStr, podInfo.PodName},
		},
	}

	log.Printf("[Azure CNS] run time configuration for CNI plugin info %+v", rt)

	netConfig, err := getNetworkConfig(netpluginConfig.networkConfigPath)
	if err != nil {
		log.Printf("[Azure CNS] Failed to build network configuration with error %v", err)
		return err
	}

	log.Printf("[Azure CNS] network configuration info %v", string(netConfig))

	err = execPlugin(rt, netConfig, cniUpdate, netpluginConfig.path)
	if err != nil {
		log.Printf("[Azure CNS] Failed to update network with error %v", err)
		return err
	}

	return nil
}

func deleteInterface(networkContainerID string) error {
	return nil
}

func configureNetworkContainerNetworking(operation, podName, podNamespace, dockerContainerid string, netPluginConfig *NetPluginConfiguration) (err error) {
	return fmt.Errorf("[Azure CNS] Operation is not supported in linux.")
}
