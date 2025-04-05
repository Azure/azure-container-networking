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
			name:    "empty store",
			store:   store.NewMockStore(""),
			want:    map[string]cns.PodInfo{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
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
