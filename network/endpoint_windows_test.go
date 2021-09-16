package network

import (
	"github.com/Azure/azure-container-networking/network/hnswrapper"
	uuid "github.com/satori/go.uuid"
	"net"
	"testing"
)

func TestNewAndDeleteEndpointImplHnsV2(t *testing.T){
	nw := &network{
		Endpoints: map[string]*endpoint{},
	}
	hnsv2 = hnswrapper.Hnsv2wrapperFake{}

	epInfo := &EndpointInfo{
		Id:                 uuid.NewV4().String(),
		ContainerID:        uuid.NewV4().String(),
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