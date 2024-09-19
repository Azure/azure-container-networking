package hnsclient

import (
	"fmt"

	"github.com/Azure/azure-container-networking/cns"
)

// CreateDefaultExtNetwork creates the default ext network (if it doesn't exist already)
// to create external switch on windows platform.
// This is windows platform specific.
func CreateDefaultExtNetwork(networkType string) error {
	return fmt.Errorf("CreateDefaultExtNetwork shouldn't be called for linux platform")
}

// DeleteDefaultExtNetwork deletes the default HNS network.
// This is windows platform specific.
func DeleteDefaultExtNetwork() error {
	return fmt.Errorf("DeleteDefaultExtNetwork shouldn't be called for linux platform")
}

// CreateHnsNetwork creates the HNS network with the provided configuration
// This is windows platform specific.
func CreateHnsNetwork(nwConfig cns.CreateHnsNetworkRequest) error {
	return fmt.Errorf("CreateHnsNetwork shouldn't be called for linux platform")
}

// DeleteHnsNetwork deletes the HNS network with the provided name.
// This is windows platform specific.
func DeleteHnsNetwork(networkName string) error {
	return fmt.Errorf("DeleteHnsNetwork shouldn't be called for linux platform")
}

// CreateHostNCApipaEndpoint creates the endpoint in the apipa network
// for host container connectivity
// This is windows platform specific.
func CreateHostNCApipaEndpoint(
	networkContainerID string,
	localIPConfiguration cns.IPConfiguration,
	allowNCToHostCommunication bool,
	allowHostToNCCommunication bool,
	ncPolicies []cns.NetworkContainerRequestPolicies) (string, error) {
	return "", nil
}

// DeleteHostNCApipaEndpoint deletes the endpoint in the apipa network
// created for host container connectivity
// This is windows platform specific.
func DeleteHostNCApipaEndpoint(
	networkContainerID string) error {
	return nil
}
