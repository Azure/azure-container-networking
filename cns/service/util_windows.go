package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/wireserver"
	"github.com/Microsoft/hcsshim"
)

type wscliInterface interface {
	GetInterfaces(ctx context.Context) (*wireserver.GetInterfacesResult, error)
}

func setVFForAccelnetNICs() error {
	var wscli wscliInterface
	// SWIFT V2 mode for accelnet, supply the MAC address to the HNS
	macAddress, err := getPrimaryNICMACAddress(wscli)
	if err != nil {
		return err
	} 
	macAddresses := []string{macAddress}
	if _, err := hcsshim.SetNnvManagementMacAddresses(macAddresses); err != nil {
		logger.Errorf("Failed to set primary NIC MAC address: %v", err)
	}
	return nil
}

// getPrimaryNICMacAddress fetches the MAC address of the primary NIC on the node.
func getPrimaryNICMACAddress(wscli wscliInterface) (string, error) {
	res, err := wscli.GetInterfaces(context.TODO())
	if err != nil {
		return "", fmt.Errorf("failed to find primary interface info: %w", err)
	}
	var macAddress string
	for _, i := range res.Interface {
		// skip if not primary
		if !i.IsPrimary {
			continue
		}
		// skip if no subnets
		if len(i.IPSubnet) == 0 {
			continue
		}
		macAddress = i.MacAddress
	}

	if macAddress == "" {
		return "", errors.New("MAC address not found in wscli")
	}
	return macAddress, nil
}
