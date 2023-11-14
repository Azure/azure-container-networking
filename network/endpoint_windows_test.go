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
)

func TestNewAndDeleteEndpointImplHnsV2(t *testing.T) {
	nw := &network{
		Endpoints: map[string]*endpoint{},
	}

	// this hnsv2 variable overwrites the package level variable in network
	// we do this to avoid passing around os specific objects in platform agnostic code
	Hnsv2 = hnswrapper.Hnsv2wrapperwithtimeout{
		Hnsv2: hnswrapper.NewHnsv2wrapperFake(),
	}

	epInfo := &EndpointInfo{
		Id:          "753d3fb6-e9b3-49e2-a109-2acc5dda61f1",
		ContainerID: "545055c2-1462-42c8-b222-e75d0b291632",
		NetNsPath:   "fakeNameSpace",
		IfName:      "eth0",
		Data:        make(map[string]interface{}),
		DNS: DNSInfo{
			Suffix:  "10.0.0.0",
			Servers: []string{"10.0.0.1, 10.0.0.2"},
			Options: nil,
		},
		MacAddress: net.HardwareAddr("00:00:5e:00:53:01"),
	}
	endpoint, err := nw.newEndpointImplHnsV2(nil, epInfo)
	if err != nil {
		fmt.Printf("+%v", err)
		t.Fatal(err)
	}

	err = nw.deleteEndpointImplHnsV2(endpoint)

	if err != nil {
		fmt.Printf("+%v", err)
		t.Fatal(err)
	}
}

func TestNewEndpointImplHnsv2Timesout(t *testing.T) {
	nw := &network{
		Endpoints: map[string]*endpoint{},
	}

	// this hnsv2 variable overwrites the package level variable in network
	// we do this to avoid passing around os specific objects in platform agnostic code

	hnsFake := hnswrapper.NewHnsv2wrapperFake()

	hnsFake.Delay = 10 * time.Second

	Hnsv2 = hnswrapper.Hnsv2wrapperwithtimeout{
		Hnsv2:          hnsFake,
		HnsCallTimeout: 5 * time.Second,
	}

	epInfo := &EndpointInfo{
		Id:          "753d3fb6-e9b3-49e2-a109-2acc5dda61f1",
		ContainerID: "545055c2-1462-42c8-b222-e75d0b291632",
		NetNsPath:   "fakeNameSpace",
		IfName:      "eth0",
		Data:        make(map[string]interface{}),
		DNS: DNSInfo{
			Suffix:  "10.0.0.0",
			Servers: []string{"10.0.0.1, 10.0.0.2"},
			Options: nil,
		},
		MacAddress: net.HardwareAddr("00:00:5e:00:53:01"),
	}
	_, err := nw.newEndpointImplHnsV2(nil, epInfo)

	if err == nil {
		t.Fatal("Failed to timeout HNS calls for creating endpoint")
	}
}

func TestDeleteEndpointImplHnsv2Timeout(t *testing.T) {
	nw := &network{
		Endpoints: map[string]*endpoint{},
	}

	Hnsv2 = hnswrapper.NewHnsv2wrapperFake()

	epInfo := &EndpointInfo{
		Id:          "753d3fb6-e9b3-49e2-a109-2acc5dda61f1",
		ContainerID: "545055c2-1462-42c8-b222-e75d0b291632",
		NetNsPath:   "fakeNameSpace",
		IfName:      "eth0",
		Data:        make(map[string]interface{}),
		DNS: DNSInfo{
			Suffix:  "10.0.0.0",
			Servers: []string{"10.0.0.1, 10.0.0.2"},
			Options: nil,
		},
		MacAddress: net.HardwareAddr("00:00:5e:00:53:01"),
	}
	endpoint, err := nw.newEndpointImplHnsV2(nil, epInfo)
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

	err = nw.deleteEndpointImplHnsV2(endpoint)

	if err == nil {
		t.Fatal("Failed to timeout HNS calls for deleting endpoint")
	}
}

func TestCreateEndpointImplHnsv1Timeout(t *testing.T) {
	nw := &network{
		Endpoints: map[string]*endpoint{},
	}

	hnsFake := hnswrapper.NewHnsv1wrapperFake()

	hnsFake.Delay = 10 * time.Second

	Hnsv1 = hnswrapper.Hnsv1wrapperwithtimeout{
		Hnsv1:          hnsFake,
		HnsCallTimeout: 5 * time.Second,
	}

	epInfo := &EndpointInfo{
		Id:          "753d3fb6-e9b3-49e2-a109-2acc5dda61f1",
		ContainerID: "545055c2-1462-42c8-b222-e75d0b291632",
		NetNsPath:   "fakeNameSpace",
		IfName:      "eth0",
		Data:        make(map[string]interface{}),
		DNS: DNSInfo{
			Suffix:  "10.0.0.0",
			Servers: []string{"10.0.0.1, 10.0.0.2"},
			Options: nil,
		},
		MacAddress: net.HardwareAddr("00:00:5e:00:53:01"),
	}
	_, err := nw.newEndpointImplHnsV1(epInfo, nil)

	if err == nil {
		t.Fatal("Failed to timeout HNS calls for creating endpoint")
	}
}

func TestDeleteEndpointImplHnsv1Timeout(t *testing.T) {
	nw := &network{
		Endpoints: map[string]*endpoint{},
	}

	Hnsv1 = hnswrapper.NewHnsv1wrapperFake()

	epInfo := &EndpointInfo{
		Id:          "753d3fb6-e9b3-49e2-a109-2acc5dda61f1",
		ContainerID: "545055c2-1462-42c8-b222-e75d0b291632",
		NetNsPath:   "fakeNameSpace",
		IfName:      "eth0",
		Data:        make(map[string]interface{}),
		DNS: DNSInfo{
			Suffix:  "10.0.0.0",
			Servers: []string{"10.0.0.1, 10.0.0.2"},
			Options: nil,
		},
		MacAddress: net.HardwareAddr("00:00:5e:00:53:01"),
	}
	endpoint, err := nw.newEndpointImplHnsV1(epInfo, nil)
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

	err = nw.deleteEndpointImplHnsV1(endpoint)

	if err == nil {
		t.Fatal("Failed to timeout HNS calls for deleting endpoint")
	}
}
