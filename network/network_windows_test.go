// Copyright 2017 Microsoft. All rights reserved.
// MIT License

//go:build windows
// +build windows

package network

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/network/hnswrapper"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/Microsoft/hcsshim/hcn"
)

var (
	errTestFailure     = errors.New("test failure")
	failedCaseReturn   = "false"
	succededCaseReturn = "true"
)

func TestNewAndDeleteNetworkImplHnsV2(t *testing.T) {
	nm := &networkManager{
		ExternalInterfaces: map[string]*externalInterface{},
	}

	// this hnsv2 variable overwrites the package level variable in network
	// we do this to avoid passing around os specific objects in platform agnostic code
	Hnsv2 = hnswrapper.NewHnsv2wrapperFake()

	nwInfo := &EndpointInfo{
		NetworkID:    "d3e97a83-ba4c-45d5-ba88-dc56757ece28",
		MasterIfName: "eth0",
		Mode:         "bridge",
	}

	extInterface := &externalInterface{
		Name:    "eth0",
		Subnets: []string{"subnet1", "subnet2"},
	}

	network, err := nm.newNetworkImplHnsV2(nwInfo, extInterface)
	if err != nil {
		fmt.Printf("+%v", err)
		t.Fatal(err)
	}

	if network.HnsId == "" {
		t.Fatal("hns network id not populated in network struct")
	}
	if nwInfo.HNSNetworkID == "" {
		t.Fatal("hns network id not populated")
	}

	err = nm.deleteNetworkImplHnsV2(network)
	if err != nil {
		fmt.Printf("+%v", err)
		t.Fatal(err)
	}
}

func TestSuccesfulNetworkCreationWhenAlreadyExists(t *testing.T) {
	nm := &networkManager{
		ExternalInterfaces: map[string]*externalInterface{},
	}

	// this hnsv2 variable overwrites the package level variable in network
	// we do this to avoid passing around os specific objects in platform agnostic code
	Hnsv2 = hnswrapper.NewHnsv2wrapperFake()

	network := &hcn.HostComputeNetwork{
		Name: "azure-vlan1-172-28-1-0_24",
	}

	_, err := Hnsv2.CreateNetwork(network)

	// network name is derived from network info id
	nwInfo := &EndpointInfo{
		NetworkID:    "azure-vlan1-172-28-1-0_24",
		MasterIfName: "eth0",
		Mode:         "bridge",
	}

	extInterface := &externalInterface{
		Name:    "eth0",
		Subnets: []string{"subnet1", "subnet2"},
	}

	_, err = nm.newNetworkImplHnsV2(nwInfo, extInterface)
	if err != nil {
		fmt.Printf("+%v", err)
		t.Fatal(err)
	}
}

func TestNewNetworkImplHnsV2WithTimeout(t *testing.T) {
	nm := &networkManager{
		ExternalInterfaces: map[string]*externalInterface{},
	}

	hnsFake := hnswrapper.NewHnsv2wrapperFake()

	hnsFake.Delay = 15 * time.Second

	Hnsv2 = hnswrapper.Hnsv2wrapperwithtimeout{
		Hnsv2:          hnsFake,
		HnsCallTimeout: 10 * time.Second,
	}

	nwInfo := &EndpointInfo{
		NetworkID:    "d3e97a83-ba4c-45d5-ba88-dc56757ece28",
		MasterIfName: "eth0",
		Mode:         "bridge",
	}

	extInterface := &externalInterface{
		Name:    "eth0",
		Subnets: []string{"subnet1", "subnet2"},
	}

	_, err := nm.newNetworkImplHnsV2(nwInfo, extInterface)

	if err == nil {
		t.Fatal("Failed to timeout HNS calls for creating network")
	}
}

func TestDeleteNetworkImplHnsV2WithTimeout(t *testing.T) {
	nm := &networkManager{
		ExternalInterfaces: map[string]*externalInterface{},
	}

	nwInfo := &EndpointInfo{
		NetworkID:    "d3e97a83-ba4c-45d5-ba88-dc56757ece28",
		MasterIfName: "eth0",
		Mode:         "bridge",
	}

	extInterface := &externalInterface{
		Name:    "eth0",
		Subnets: []string{"subnet1", "subnet2"},
	}

	Hnsv2 = hnswrapper.NewHnsv2wrapperFake()

	network, err := nm.newNetworkImplHnsV2(nwInfo, extInterface)
	if err != nil {
		fmt.Printf("+%v", err)
		t.Fatal(err)
	}

	hnsFake := hnswrapper.NewHnsv2wrapperFake()

	hnsFake.Delay = 10 * time.Second

	Hnsv2 = hnswrapper.Hnsv2wrapperwithtimeout{
		Hnsv2:          hnsFake,
		HnsCallTimeout: 5 * time.Second,
	}

	err = nm.deleteNetworkImplHnsV2(network)

	if err == nil {
		t.Fatal("Failed to timeout HNS calls for deleting network")
	}
}

func TestNewNetworkImplHnsV1WithTimeout(t *testing.T) {
	nm := &networkManager{
		ExternalInterfaces: map[string]*externalInterface{},
	}

	hnsFake := hnswrapper.NewHnsv1wrapperFake()

	hnsFake.Delay = 10 * time.Second

	Hnsv1 = hnswrapper.Hnsv1wrapperwithtimeout{
		Hnsv1:          hnsFake,
		HnsCallTimeout: 5 * time.Second,
	}

	nwInfo := &EndpointInfo{
		NetworkID:    "d3e97a83-ba4c-45d5-ba88-dc56757ece28",
		MasterIfName: "eth0",
		Mode:         "bridge",
	}

	extInterface := &externalInterface{
		Name:    "eth0",
		Subnets: []string{"subnet1", "subnet2"},
	}

	_, err := nm.newNetworkImplHnsV1(nwInfo, extInterface)

	if err == nil {
		t.Fatal("Failed to timeout HNS calls for creating network")
	}
}

func TestDeleteNetworkImplHnsV1WithTimeout(t *testing.T) {
	nm := &networkManager{
		ExternalInterfaces: map[string]*externalInterface{},
	}

	nwInfo := &EndpointInfo{
		NetworkID:    "d3e97a83-ba4c-45d5-ba88-dc56757ece28",
		MasterIfName: "eth0",
		Mode:         "bridge",
	}

	extInterface := &externalInterface{
		Name:    "eth0",
		Subnets: []string{"subnet1", "subnet2"},
	}

	Hnsv1 = hnswrapper.NewHnsv1wrapperFake()

	network, err := nm.newNetworkImplHnsV1(nwInfo, extInterface)
	if err != nil {
		fmt.Printf("+%v", err)
		t.Fatal(err)
	}

	hnsFake := hnswrapper.NewHnsv1wrapperFake()

	hnsFake.Delay = 10 * time.Second

	Hnsv1 = hnswrapper.Hnsv1wrapperwithtimeout{
		Hnsv1:          hnsFake,
		HnsCallTimeout: 5 * time.Second,
	}

	err = nm.deleteNetworkImplHnsV1(network)

	if err == nil {
		t.Fatal("Failed to timeout HNS calls for deleting network")
	}
}

func TestAddIPv6DefaultRoute(t *testing.T) {
	_, ipnetv4, _ := net.ParseCIDR("10.240.0.0/12")
	_, ipnetv6, _ := net.ParseCIDR("fc00::/64")

	nm := &networkManager{
		ExternalInterfaces: map[string]*externalInterface{},
		plClient:           platform.NewMockExecClient(false),
	}

	networkSubnetInfo := []SubnetInfo{
		{
			Family:  platform.AfINET,
			Gateway: net.ParseIP("10.240.0.1"),
			Prefix:  *ipnetv4,
		},
		{
			Family:  platform.AfINET6,
			Gateway: net.ParseIP("fc00::1"),
			Prefix:  *ipnetv6,
		},
	}

	nwInfo := &EndpointInfo{
		NetworkID:    "d3f97a83-ba4c-45d5-ba88-dc56757ece28",
		MasterIfName: "eth0",
		Mode:         "bridge",
		Subnets:      networkSubnetInfo,
	}

	extInterface := &externalInterface{
		Name:    "eth0",
		Subnets: []string{"subnet1", "subnet2"},
	}

	Hnsv2 = hnswrapper.NewHnsv2wrapperFake()

	// check if network can be successfully created
	_, err := nm.newNetworkImplHnsV2(nwInfo, extInterface)
	if err != nil {
		t.Fatalf("Failed to create network due to error:%+v", err)
	}
}

func TestFailToAddIPv6DefaultRoute(t *testing.T) {
	_, ipnetv4, _ := net.ParseCIDR("10.240.0.0/12")
	_, ipnetv6, _ := net.ParseCIDR("fc00::/64")

	nm := &networkManager{
		ExternalInterfaces: map[string]*externalInterface{},
		plClient:           platform.NewMockExecClient(true), // return mock exec error
	}

	networkSubnetInfo := []SubnetInfo{
		{
			Family:  platform.AfINET,
			Gateway: net.ParseIP("10.240.0.1"),
			Prefix:  *ipnetv4,
		},
		{
			Family:  platform.AfINET6,
			Gateway: net.ParseIP("fc00::1"),
			Prefix:  *ipnetv6,
		},
	}

	nwInfo := &EndpointInfo{
		NetworkID:    "d3f97a83-ba4c-45d5-ba88-dc56757ece28",
		MasterIfName: "eth0",
		Mode:         "bridge",
		Subnets:      networkSubnetInfo,
	}

	extInterface := &externalInterface{
		Name:    "eth0",
		Subnets: []string{"subnet1", "subnet2"},
	}

	Hnsv2 = hnswrapper.NewHnsv2wrapperFake()

	// check if network is failed to create
	_, err := nm.newNetworkImplHnsV2(nwInfo, extInterface)
	if err == nil {
		t.Fatal("Network should not be created")
	}
}

func TestAddIPv6DefaultRouteHappyPath(t *testing.T) {
	mockExecClient := platform.NewMockExecClient(false)

	nm := &networkManager{
		plClient: mockExecClient,
	}

	// happy path
	mockExecClient.SetPowershellCommandResponder(func(cmd string) (string, error) {
		if strings.Contains(cmd, "Get-NetIPInterface") || strings.Contains(cmd, "Remove-NetRoute") {
			return succededCaseReturn, nil
		}

		// fail secondary command execution and successfully execute remove-netRoute command
		if strings.Contains(cmd, "Get-NetRoute") {
			return failedCaseReturn, errTestFailure
		}

		return "", nil
	})

	err := nm.addIPv6DefaultRoute()
	if err != nil {
		t.Fatal("Failed to test happy path")
	}
}

func TestAddIPv6DefaultRouteUnhappyPathGetNetInterface(t *testing.T) {
	mockExecClient := platform.NewMockExecClient(false)

	nm := &networkManager{
		plClient: mockExecClient,
	}

	// failed to execute Get-NetIPInterface command to find interface index
	mockExecClient.SetPowershellCommandResponder(func(cmd string) (string, error) {
		if strings.Contains(cmd, "Get-NetIPInterface") {
			return failedCaseReturn, errTestFailure
		}
		return "", nil
	})

	err := nm.addIPv6DefaultRoute()
	if err == nil {
		t.Fatal("Failed to test unhappy path with failing to execute get-netIPInterface command")
	}
}

func TestAddIPv6DefaultRouteUnhappyPathAddRoute(t *testing.T) {
	mockExecClient := platform.NewMockExecClient(false)

	nm := &networkManager{
		plClient: mockExecClient,
	}

	mockExecClient.SetPowershellCommandResponder(func(cmd string) (string, error) {
		if strings.Contains(cmd, "Get-NetIPInterface") {
			return succededCaseReturn, nil
		}

		// fail secondary command execution and failed to execute remove-netRoute command
		if strings.Contains(cmd, "Get-NetRoute") {
			return failedCaseReturn, errTestFailure
		}

		if strings.Contains(cmd, "Remove-NetRoute") {
			return failedCaseReturn, errTestFailure
		}
		return "", nil
	})

	err := nm.addIPv6DefaultRoute()
	if err == nil {
		t.Fatal("Failed to test unhappy path with failing to add default route command")
	}
}

func TestNewNetworkImplHnsV2ForBackendNIC(t *testing.T) {
	pnpID := "PCI\\VEN_15B3&DEV_101C&SUBSYS_000715B3&REV_00\\5&8c5acce&0&0"

	nm := &networkManager{
		ExternalInterfaces: map[string]*externalInterface{},
		plClient:           platform.NewMockExecClient(false),
	}

	nwInfo := &EndpointInfo{
		NetworkID:    "d3f97a83-ba4c-45d5-ba88-dc56757ece28",
		MasterIfName: "ib1",
		Mode:         "transparent",
		NICType:      cns.BackendNIC,
		PnPID:        pnpID,
	}

	extInterface := &externalInterface{
		Name: "eth1",
	}

	Hnsv2 = hnswrapper.NewHnsv2wrapperFake()

	// should return nil if nicType is BackendNIC when creating network
	network, err := nm.newNetworkImplHnsV2(nwInfo, extInterface)
	if network != nil || err != nil {
		t.Fatal("HNS network is created with BackendNIC interface")
	}
}

// mock hns network creation and deletion for DelegatedNIC
func TestNewAndDeleteNetworkImplHnsV2ForDelegated(t *testing.T) {
	nm := &networkManager{
		ExternalInterfaces: map[string]*externalInterface{},
	}

	// this hnsv2 variable overwrites the package level variable in network
	// we do this to avoid passing around os specific objects in platform agnostic code
	Hnsv2 = hnswrapper.NewHnsv2wrapperFake()

	nwInfo := &EndpointInfo{
		NetworkID:    "d3e97a83-ba4c-45d5-ba88-dc56757ece28",
		MasterIfName: "eth0",
		Mode:         "bridge",
		NICType:      cns.NodeNetworkInterfaceFrontendNIC,
		MacAddress:   net.HardwareAddr("12:34:56:78:9a:bc"),
	}

	extInterface := &externalInterface{
		Name:    "eth0",
		Subnets: []string{"subnet1", "subnet2"},
	}

	network, err := nm.newNetworkImplHnsV2(nwInfo, extInterface)
	if err != nil {
		fmt.Printf("+%v", err)
		t.Fatal(err)
	}

	err = nm.deleteNetworkImpl(network, cns.NodeNetworkInterfaceFrontendNIC)
	if err != nil {
		fmt.Printf("+%v", err)
		t.Fatal(err)
	}
}

func TestSkipNetworkDeletion(t *testing.T) {
	nm := &networkManager{
		ExternalInterfaces: map[string]*externalInterface{},
	}

	// should return nil if nicType is Backend
	err := nm.deleteNetworkImpl(nil, cns.BackendNIC)
	if err != nil {
		fmt.Printf("+%v", err)
		t.Fatal(err)
	}
}

func TestTransparentNetworkCreationForDelegated(t *testing.T) {
	nm := &networkManager{
		ExternalInterfaces: map[string]*externalInterface{},
	}

	// this hnsv2 variable overwrites the package level variable in network
	// we do this to avoid passing around os specific objects in platform agnostic code
	Hnsv2 = hnswrapper.NewHnsv2wrapperFake()

	nwInfo := &EndpointInfo{
		NetworkID:    "d3e97a83-ba4c-45d5-ba88-dc56757ece28",
		MasterIfName: "eth0",
		Mode:         "bridge",
		NICType:      cns.NodeNetworkInterfaceFrontendNIC,
	}

	extInterface := &externalInterface{
		Name:    "eth1",
		Subnets: []string{"subnet1", "subnet2"},
	}

	_, err := nm.newNetworkImplHnsV2(nwInfo, extInterface)
	if err != nil {
		fmt.Printf("+%v", err)
		t.Fatal(err)
	}

	// create a network again with same name and it should return error for transparent network
	_, err = nm.newNetworkImplHnsV2(nwInfo, extInterface)
	if err == nil {
		t.Fatal("network creation does not return error")
	}
}

// Test Configure HNC network for infraNIC ensuring the hcn network type is always l2 bridge
func TestConfigureHCNNetworkInfraNIC(t *testing.T) {
	expectedHcnNetworkType := hcn.L2Bridge

	nm := &networkManager{
		ExternalInterfaces: map[string]*externalInterface{},
	}

	extIf := externalInterface{
		Name: "eth0",
	}

	nwInfo := &EndpointInfo{
		AdapterName:  "eth0",
		NetworkID:    "d3e97a83-ba4c-45d5-ba88-dc56757ece28",
		MasterIfName: "eth0",
		NICType:      cns.InfraNIC,
		IfIndex:      1,
		EndpointID:   "753d3fb6-e9b3-49e2-a109-2acc5dda61f1",
		ContainerID:  "545055c2-1462-42c8-b222-e75d0b291632",
		NetNsPath:    "fakeNameSpace",
		IfName:       "eth0",
		Data:         make(map[string]interface{}),
		EndpointDNS: DNSInfo{
			Suffix:  "10.0.0.0",
			Servers: []string{"10.0.0.1, 10.0.0.2"},
			Options: nil,
		},
		HNSNetworkID: "853d3fb6-e9b3-49e2-a109-2acc5dda61f1",
	}

	hostComputeNetwork, err := nm.configureHcnNetwork(nwInfo, &extIf)
	if err != nil {
		t.Fatalf("Failed to configure hcn network for infraNIC interface due to: %v", err)
	}

	if hostComputeNetwork.Type != expectedHcnNetworkType {
		t.Fatalf("Host network mode is not configured as %v mode when interface NIC type is infraNIC", expectedHcnNetworkType)
	}
}

// Test Configure HCN Network for Swiftv2 DelegatedNIC HostComputeNetwork fields
func TestConfigureHCNNetworkSwiftv2DelegatedNIC(t *testing.T) {
	expectedSwiftv2NetworkMode := hcn.Transparent
	expectedSwifv2NetworkFlags := hcn.EnableNonPersistent | hcn.DisableHostPort | hcn.EnableIov

	nm := &networkManager{
		ExternalInterfaces: map[string]*externalInterface{},
	}

	extIf := externalInterface{
		Name: "eth1",
	}

	nwInfo := &EndpointInfo{
		AdapterName:  "eth1",
		NetworkID:    "d3e97a83-ba4c-45d5-ba88-dc56757ece28",
		MasterIfName: "eth1",
		Mode:         "bridge",
		NICType:      cns.NodeNetworkInterfaceFrontendNIC,
	}

	hostComputeNetwork, err := nm.configureHcnNetwork(nwInfo, &extIf)
	if err != nil {
		t.Fatalf("Failed to configure hcn network for delegatedVMNIC interface due to: %v", err)
	}

	if hostComputeNetwork.Type != expectedSwiftv2NetworkMode {
		t.Fatalf("host network mode is not configured as %v mode when interface NIC type is delegatedVMNIC", expectedSwiftv2NetworkMode)
	}

	// make sure network type is transparent and flags is 9224
	// TODO: check if this is expected for both delegated&accelnet
	if hostComputeNetwork.Flags != expectedSwifv2NetworkFlags {
		t.Fatalf("host network flags is not configured as %v when interface NIC type is delegatedVMNIC", expectedSwifv2NetworkFlags)
	}
}
