// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package imdsclient

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"math"
	"net/http"
	"strings"

	"github.com/Azure/azure-container-networking/cns/logger"
)

// GetNetworkContainerInfoFromHost retrieves the programmed version of network container from Host.
func (imdsClient *ImdsClient) GetNetworkContainerInfoFromHost(networkContainerID string, primaryAddress string, authToken string, apiVersion string) (*ContainerVersion, error) {
	logger.Printf("[Azure CNS] GetNetworkContainerInfoFromHost")
	queryURL := fmt.Sprintf(hostQueryURLForProgrammedVersion,
		primaryAddress, networkContainerID, authToken, apiVersion)

	logger.Printf("[Azure CNS] Going to query Azure Host for container version @\n %v\n", queryURL)
	jsonResponse, err := http.Get(queryURL)
	if err != nil {
		return nil, err
	}

	defer jsonResponse.Body.Close()

	logger.Printf("[Azure CNS] Response received from Azure Host for NetworkManagement/interfaces: %v", jsonResponse.Body)

	var response containerVersionJsonResponse
	err = json.NewDecoder(jsonResponse.Body).Decode(&response)
	if err != nil {
		return nil, err
	}

	ret := &ContainerVersion{
		NetworkContainerID: response.NetworkContainerID,
		ProgrammedVersion:  response.ProgrammedVersion,
	}

	return ret, nil
}

// GetPrimaryInterfaceInfoFromHost retrieves subnet and gateway of primary NIC from Host.
func (imdsClient *ImdsClient) GetPrimaryInterfaceInfoFromHost() (*InterfaceInfo, error) {
	logger.Printf("[Azure CNS] GetPrimaryInterfaceInfoFromHost")

	interfaceInfo := &InterfaceInfo{}
	resp, err := http.Get(hostQueryURL)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	logger.Printf("[Azure CNS] Response received from NMAgent for get interface details: %v", resp.Body)

	var doc xmlDocument
	decoder := xml.NewDecoder(resp.Body)
	err = decoder.Decode(&doc)
	if err != nil {
		return nil, err
	}

	foundPrimaryInterface := false

	// For each interface.
	for _, i := range doc.Interface {
		// Find primary Interface.
		if i.IsPrimary {
			interfaceInfo.IsPrimary = true

			// Get the first subnet.
			for _, s := range i.IPSubnet {
				interfaceInfo.Subnet = s.Prefix
				malformedSubnetError := fmt.Errorf("Malformed subnet received from host %s", s.Prefix)

				st := strings.Split(s.Prefix, "/")
				if len(st) != 2 {
					return nil, malformedSubnetError
				}

				ip := strings.Split(st[0], ".")
				if len(ip) != 4 {
					return nil, malformedSubnetError
				}

				interfaceInfo.Gateway = fmt.Sprintf("%s.%s.%s.1", ip[0], ip[1], ip[2])
				for _, ip := range s.IPAddress {
					if ip.IsPrimary {
						interfaceInfo.PrimaryIP = ip.Address
					}
				}

				imdsClient.primaryInterface = interfaceInfo
				break
			}

			foundPrimaryInterface = true
			break
		}
	}

	var er error
	er = nil
	if !foundPrimaryInterface {
		er = fmt.Errorf("Unable to find primary NIC")
	}

	return interfaceInfo, er
}

// GetPrimaryInterfaceInfoFromMemory retrieves subnet and gateway of primary NIC that is saved in memory.
func (imdsClient *ImdsClient) GetPrimaryInterfaceInfoFromMemory() (*InterfaceInfo, error) {
	logger.Printf("[Azure CNS] GetPrimaryInterfaceInfoFromMemory")

	var iface *InterfaceInfo
	var err error
	if imdsClient.primaryInterface == nil {
		logger.Debugf("Azure-CNS] Primary interface in memory does not exist. Will get it from Host.")
		iface, err = imdsClient.GetPrimaryInterfaceInfoFromHost()
		if err != nil {
			logger.Errorf("[Azure-CNS] Unable to retrive primary interface info.")
		} else {
			logger.Debugf("Azure-CNS] Primary interface received from HOST: %+v.", iface)
		}
	} else {
		iface = imdsClient.primaryInterface
	}

	return iface, err
}

// GetNetworkContainerInfoFromHostWithoutToken is a temp implementation which will be removed once background thread
// updating host version is ready. Return max integer value to regress current AKS scenario
func (imdsClient *ImdsClient) GetNetworkContainerInfoFromHostWithoutToken() int {
	logger.Printf("[Azure CNS] GetNMagentVersionFromNMAgent")

	return math.MaxInt64
}
