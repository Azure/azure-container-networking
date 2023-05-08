// Copyright 2017 Microsoft. All rights reserved.
// MIT License

//go:build windows
// +build windows

package network

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/network/hnswrapper"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/Microsoft/hcsshim/hcn"
	"github.com/stretchr/testify/assert"
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

// test addNewNetRules to add net rules from NetworkInfo
func TestAddNewNetRules(t *testing.T) {
	cnt := 0
	plc := platform.NewMockExecClient(false)
	nm := &networkManager{
		ExternalInterfaces: map[string]*externalInterface{},
		plClient:           plc,
	}

	nwInfo := &NetworkInfo{
		Id:           "d3e97a83-ba4c-45d5-ba88-dc56757ece28",
		MasterIfName: "eth0",
		Mode:         "bridge",
		Subnets: []SubnetInfo{
			{
				Prefix: net.IPNet{
					IP:   net.IPv4(10, 0, 0, 1),
					Mask: net.IPv4Mask(255, 255, 0, 0),
				},
				Gateway: net.ParseIP("0.0.0.0"),
			},
			{
				Prefix: net.IPNet{
					IP:   net.ParseIP("ff02::fb"),
					Mask: net.CIDRMask(128, 128),
				},
				Gateway: net.ParseIP("::"),
			},
		},
	}

	// get each delete and add new rule entry
	ifName := "vEthernet (eth0)"
	var ipType, defaultHop string
	expectedCmds := make([]string, 0)
	expectedNumRules := 8
	for _, subnet := range nwInfo.Subnets {
		prefix := subnet.Prefix.String()
		ip, _, _ := net.ParseCIDR(prefix)
		if ip.To4() != nil {
			ipType = "ipv4"
			defaultHop = ipv4DefaultHop
		} else {
			ipType = "ipv6"
			defaultHop = ipv6DefaultHop
		}
		gateway := subnet.Gateway.String()
		netRouteCmd1 := fmt.Sprintf(netRouteCmd, ipType, "delete", prefix, ifName, defaultHop)
		expectedCmds = append(expectedCmds, netRouteCmd1)
		netRouteCmd2 := fmt.Sprintf(netRouteCmd, ipType, "add", prefix, ifName, defaultHop)
		expectedCmds = append(expectedCmds, netRouteCmd2)
		netRouteCmd3 := fmt.Sprintf(netRouteCmd, ipType, "delete", prefix, ifName, gateway)
		expectedCmds = append(expectedCmds, netRouteCmd3)
		netRouteCmd4 := fmt.Sprintf(netRouteCmd, ipType, "add", prefix, ifName, gateway)
		expectedCmds = append(expectedCmds, netRouteCmd4)
	}

	plc.SetExecCommand(func(cmd string) (string, error) {
		assert.Equal(t, expectedCmds[cnt], cmd)
		cnt++
		return "", nil
	})

	err := nm.addNewNetRules(nwInfo)
	if err != nil {
		t.Fatal("Failed to add/delete a new network rule")
	}

	if cnt != expectedNumRules {
		t.Fatalf("Failed to add/delete expected number %d of new network rules", expectedNumRules)
	}
}
