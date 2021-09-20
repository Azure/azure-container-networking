//go:build !ignore_uncovered
// +build !ignore_uncovered

// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package fakes

import (
	"github.com/Azure/azure-container-networking/cns/imdsclient"
	"github.com/Azure/azure-container-networking/cns/logger"
)

var (
	HostPrimaryIpTest = "10.0.0.4"
	HostSubnetTest    = "10.0.0.0/24"
)

// ImdsClient can be used to connect to VM Host agent in Azure.
type ImdsClientTest struct{}

func NewFakeImdsClient() *ImdsClientTest {
	return &ImdsClientTest{}
}

// GetNetworkContainerInfoFromHost - Mock implementation to return Container version info.
func (imdsClient *ImdsClientTest) GetNetworkContainerInfoFromHost(networkContainerID string, primaryAddress string, authToken string, apiVersion string) (*imdsclient.ContainerVersion, error) {
	ret := &imdsclient.ContainerVersion{}

	return ret, nil
}

// GetPrimaryInterfaceInfoFromHost - Mock implementation to return Host interface info
func (imdsClient *ImdsClientTest) GetPrimaryInterfaceInfoFromHost() (*imdsclient.InterfaceInfo, error) {
	logger.Printf("[Azure CNS] GetPrimaryInterfaceInfoFromHost")

	interfaceInfo := &imdsclient.InterfaceInfo{
		Subnet:    HostSubnetTest,
		PrimaryIP: HostPrimaryIpTest,
	}

	return interfaceInfo, nil
}

// GetPrimaryInterfaceInfoFromMemory - Mock implementation to return host interface info
func (imdsClient *ImdsClientTest) GetPrimaryInterfaceInfoFromMemory() (*imdsclient.InterfaceInfo, error) {
	logger.Printf("[Azure CNS] GetPrimaryInterfaceInfoFromMemory")

	return imdsClient.GetPrimaryInterfaceInfoFromHost()
}

// GetNetworkContainerInfoFromHostWithoutToken - Mock implementation to return host NMAgent NC version
// Set it as 0 which is the same as default initial NC version for testing purpose
func (imdsClient *ImdsClientTest) GetNetworkContainerInfoFromHostWithoutToken() int {
	logger.Printf("[Azure CNS] get the NC version from NMAgent")

	return 0
}
