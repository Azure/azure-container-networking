package cns

import (
	"net"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/restserver"
	"github.com/Azure/azure-container-networking/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCNSPodInfoProvider(t *testing.T) {
	goodStore := store.NewMockStore("")
	goodEndpointState := make(map[string]*restserver.EndpointInfo)
	endpointInfo := &restserver.EndpointInfo{PodName: "goldpinger-deploy-bbbf9fd7c-z8v4l", PodNamespace: "default", IfnameToIPMap: make(map[string]*restserver.IPInfo)}
	endpointInfo.IfnameToIPMap["eth0"] = &restserver.IPInfo{IPv4: []net.IPNet{{IP: net.IPv4(10, 241, 0, 65), Mask: net.IPv4Mask(255, 255, 255, 0)}}}

	goodEndpointState["0a4917617e15d24dc495e407d8eb5c88e4406e58fa209e4eb75a2c2fb7045eea"] = endpointInfo
	err := goodStore.Write(restserver.EndpointStoreKey, goodEndpointState)
	if err != nil {
		t.Fatalf("Error writing to store: %v", err)
	}

	// swiftV2Store holds a SwiftV2 multitenant endpoint with both an InfraNIC (allocated from the
	// NNC IP pool) and a delegated FrontendNIC (owned by the per-pod MTPNC, not the NNC). Only the
	// InfraNIC IP must be returned for IPAM reconciliation; including the FrontendNIC IP would make
	// reconcile fail to map it to a NetworkContainer and abort CNS initialization.
	swiftV2Store := store.NewMockStore("")
	swiftV2EndpointState := make(map[string]*restserver.EndpointInfo)
	swiftV2EndpointInfo := &restserver.EndpointInfo{PodName: "vfpod1", PodNamespace: "default", IfnameToIPMap: make(map[string]*restserver.IPInfo)}
	swiftV2EndpointInfo.IfnameToIPMap["eth0"] = &restserver.IPInfo{IPv4: []net.IPNet{{IP: net.IPv4(10, 226, 0, 52), Mask: net.IPv4Mask(255, 255, 0, 0)}}, NICType: cns.InfraNIC}
	swiftV2EndpointInfo.IfnameToIPMap["Ethernet 4"] = &restserver.IPInfo{IPv4: []net.IPNet{{IP: net.IPv4(172, 25, 0, 7), Mask: net.IPv4Mask(255, 255, 0, 0)}}, NICType: cns.NodeNetworkInterfaceFrontendNIC}
	swiftV2EndpointState["cd97a4018a"] = swiftV2EndpointInfo
	if err := swiftV2Store.Write(restserver.EndpointStoreKey, swiftV2EndpointState); err != nil {
		t.Fatalf("Error writing to store: %v", err)
	}

	tests := []struct {
		name    string
		store   store.KeyValueStore
		want    map[string]cns.PodInfo
		wantErr bool
	}{
		{
			name:  "good",
			store: goodStore,
			want: map[string]cns.PodInfo{"10.241.0.65": cns.NewPodInfo("0a4917617e15d24dc495e407d8eb5c88e4406e58fa209e4eb75a2c2fb7045eea",
				"0a4917617e15d24dc495e407d8eb5c88e4406e58fa209e4eb75a2c2fb7045eea", "goldpinger-deploy-bbbf9fd7c-z8v4l", "default")},
			wantErr: false,
		},
		{
			name:  "swiftv2 delegated nic excluded",
			store: swiftV2Store,
			want: map[string]cns.PodInfo{"10.226.0.52": cns.NewPodInfo("cd97a4018a",
				"cd97a4018a", "vfpod1", "default")},
			wantErr: false,
		},
		{
			name:    "empty store",
			store:   store.NewMockStore(""),
			want:    map[string]cns.PodInfo{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := podInfoProvider(tt.store)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			podInfoByIP, _ := got.PodInfoByIP()
			assert.Equal(t, tt.want, podInfoByIP)
		})
	}
}
