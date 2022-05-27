// Copyright 2017 Microsoft. All rights reserved.
// MIT License

//go:build windows
// +build windows

package network

import (
	"fmt"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/network/hnswrapper"

	"github.com/Microsoft/hcsshim/hcn"
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
