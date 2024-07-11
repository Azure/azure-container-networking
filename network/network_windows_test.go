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

	nwInfo := &NetworkInfo{
		Id:           "d3e97a83-ba4c-45d5-ba88-dc56757ece28",
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
	nwInfo := &NetworkInfo{
		Id:           "azure-vlan1-172-28-1-0_24",
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

	nwInfo := &NetworkInfo{
		Id:           "d3e97a83-ba4c-45d5-ba88-dc56757ece28",
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

	nwInfo := &NetworkInfo{
		Id:           "d3e97a83-ba4c-45d5-ba88-dc56757ece28",
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

	nwInfo := &NetworkInfo{
		Id:           "d3e97a83-ba4c-45d5-ba88-dc56757ece28",
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

	nwInfo := &NetworkInfo{
		Id:           "d3e97a83-ba4c-45d5-ba88-dc56757ece28",
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

	nwInfo := &NetworkInfo{
		Id:           "d3f97a83-ba4c-45d5-ba88-dc56757ece28",
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

	nwInfo := &NetworkInfo{
		Id:           "d3f97a83-ba4c-45d5-ba88-dc56757ece28",
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
