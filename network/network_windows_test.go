package network

import (
	"fmt"
	"github.com/Azure/azure-container-networking/network/hnswrapper"
	uuid "github.com/satori/go.uuid"
	"testing"
)

func TestNewAndDeleteNetworkImplHnsV2(t *testing.T){
	nm := &networkManager{
		ExternalInterfaces: map[string]*externalInterface{},
	}

	hnsv2 = hnswrapper.Hnsv2wrapperFake{}

	nwInfo := &NetworkInfo{
		Id:           uuid.NewV4().String(),
		MasterIfName: "eth0",
		Mode: "bridge",
	}

	extInterface := &externalInterface{
		Name:    "eth0",
		Subnets: []string{"subnet1", "subnet2"},
	}

	network,err := nm.newNetworkImplHnsV2(nwInfo,extInterface)

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
