package restserver

import (
	"context"
	"net/netip"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/common"
	"github.com/Azure/azure-container-networking/cns/fakes"
	"github.com/Azure/azure-container-networking/cns/nodesubnet"
	acn "github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/nmagent"
	"github.com/Azure/azure-container-networking/store"
)

func GetRestServiceObjectForNodeSubnetTest(t *testing.T, generator CNIConflistGenerator) *HTTPRestService {
	config := &common.ServiceConfig{
		Name:        "test",
		Version:     "1.0",
		ChannelMode: "AzureHost",
		Store:       store.NewMockStore("test"),
	}
	interfaces := nmagent.Interfaces{
		Entries: []nmagent.Interface{
			{
				MacAddress: nmagent.MACAddress{0x00, 0x0D, 0x3A, 0xF9, 0xDC, 0xA6},
				IsPrimary:  true,
				InterfaceSubnets: []nmagent.InterfaceSubnet{
					{
						Prefix: "10.0.0.0/24",
						IPAddress: []nmagent.NodeIP{
							{
								Address:   nmagent.IPAddress(netip.AddrFrom4([4]byte{10, 0, 0, 4})),
								IsPrimary: true,
							},
							{
								Address:   nmagent.IPAddress(netip.AddrFrom4([4]byte{10, 0, 0, 52})),
								IsPrimary: false,
							},
							{
								Address:   nmagent.IPAddress(netip.AddrFrom4([4]byte{10, 0, 0, 63})),
								IsPrimary: false,
							},
							{
								Address:   nmagent.IPAddress(netip.AddrFrom4([4]byte{10, 0, 0, 45})),
								IsPrimary: false,
							},
						},
					},
				},
			},
		},
	}

	svc, err := cns.NewService(config.Name, config.Version, config.ChannelMode, config.Store)
	if err != nil {
		return nil
	}

	svc.SetOption(acn.OptCnsURL, "")
	svc.SetOption(acn.OptCnsPort, "")
	err = svc.Initialize(config)
	if err != nil {
		return nil
	}

	t.Cleanup(func() { svc.Uninitialize() })

	return &HTTPRestService{
		Service:                  svc,
		cniConflistGenerator:     generator,
		state:                    &httpRestServiceState{},
		PodIPConfigState:         make(map[string]cns.IPConfigurationStatus),
		PodIPIDByPodInterfaceKey: make(map[string][]string),
		nma: &fakes.NMAgentClientFake{
			GetInterfaceIPInfoF: func(_ context.Context) (nmagent.Interfaces, error) {
				return interfaces, nil
			},
		},
		wscli: &fakes.WireserverClientFake{},
	}
}

// GetNodesubnetIPFetcher gets the nodesubnet.IPFetcher from the HTTPRestService.
func (service *HTTPRestService) GetNodesubnetIPFetcher() *nodesubnet.IPFetcher {
	return service.nodesubnetIPFetcher
}
