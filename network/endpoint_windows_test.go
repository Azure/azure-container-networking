// Copyright 2017 Microsoft. All rights reserved.
// MIT License

// +build windows

package network

import (
	"fmt"
	"github.com/Azure/azure-container-networking/network/hnswrapper"
	"net"
	"testing"
)

func TestNewAndDeleteEndpointImplHnsV2(t *testing.T){
	nw := &network{
		Endpoints: map[string]*endpoint{},
	}

	// this hnsv2 variable overwrites the package level variable in network
	// we do this to avoid passing around os specific objects in platform agnostic code
	hnsv2 = hnswrapper.Hnsv2wrapperFake{}

	epInfo := &EndpointInfo{
		Id:                 "753d3fb6-e9b3-49e2-a109-2acc5dda61f1",
		ContainerID:        "545055c2-1462-42c8-b222-e75d0b291632",
		NetNsPath:          "fakeNameSpace",
		IfName:             "eth0",
		Data:               make(map[string]interface{}),
		DNS: 	DNSInfo{
			Suffix: "10.0.0.0",
			Servers: []string{"10.0.0.1, 10.0.0.2"},
			Options: nil,
		},
		MacAddress: net.HardwareAddr("00:00:5e:00:53:01"),
	}
	endpoint,err := nw.newEndpointImplHnsV2(nil, epInfo)

	if err != nil {
		fmt.Printf("+%v", err)
		t.Fatal(err)
	}

	err = nw.deleteEndpointImplHnsV2(nil, endpoint)

	if err != nil {
		fmt.Printf("+%v", err)
		t.Fatal(err)
	}
}