// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package networkcontainers

import (
	"errors"
	"fmt"
	"net"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/log"
)

// NetworkContainers can be used to perform operations on network containers.
type NetworkContainers struct {
	logpath string
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
func (cn *NetworkContainers) Update(createNetworkContainerRequest cns.CreateNetworkContainerRequest) error {
	log.Printf("[Azure CNS] NetworkContainers.Update called")
	err := createOrUpdateInterface(createNetworkContainerRequest)
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
