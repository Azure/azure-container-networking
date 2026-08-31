// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package state

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/wireserver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These fixtures mirror restserver's unexported persisted types so json.Marshal
// emits the capitalized Go-default keys used by the legacy files.
type capitalizedCNSStateFixture struct { //nolint:musttag // Go-default field names reproduce the capitalized legacy wire format.
	Location                         string
	NetworkType                      string
	OrchestratorType                 string
	NodeID                           string
	Initialized                      bool
	ContainerIDByOrchestratorContext map[string]*capitalizedNCListFixture
	ContainerStatus                  map[string]capitalizedContainerStatusFixture
	Networks                         map[string]*capitalizedNetworkInfoFixture
	TimeStamp                        time.Time
	PnpIDByMacAddress                map[string]string
}

type capitalizedNCListFixture string

type capitalizedContainerStatusFixture struct { //nolint:musttag // Go-default field names reproduce the capitalized legacy wire format.
	ID                            string
	VMVersion                     string
	HostVersion                   string
	CreateNetworkContainerRequest cns.CreateNetworkContainerRequest
	VfpUpdateComplete             bool
}

type capitalizedNetworkInfoFixture struct { //nolint:musttag // Go-default field names reproduce the capitalized legacy wire format.
	NetworkName string
	NicInfo     *wireserver.InterfaceInfo
	Options     map[string]any
}

type capitalizedEndpointFixture struct { //nolint:musttag // Go-default field names reproduce the capitalized legacy wire format.
	PodName       string
	PodNamespace  string
	IfnameToIPMap map[string]*capitalizedIPInfoFixture
}

type capitalizedIPInfoFixture struct { //nolint:musttag // Go-default field names reproduce the capitalized legacy wire format.
	IPv4               []net.IPNet
	IPv6               []net.IPNet `json:",omitempty"`
	HnsEndpointID      string      `json:",omitempty"`
	HnsNetworkID       string      `json:",omitempty"`
	HostVethName       string      `json:",omitempty"`
	MacAddress         string      `json:",omitempty"`
	NetworkContainerID string      `json:",omitempty"`
	NICType            cns.NICType
}

func TestImportLegacyCapitalizedRestserverKeys(t *testing.T) {
	request := completeNetworkContainerRequest()
	request.NetworkContainerid = importNC1
	request.Version = "17"
	request.AuthorizationToken = "legacy-secret"
	request.SecondaryIPConfigs = map[string]cns.SecondaryIPConfig{
		importIPv4: {IPAddress: importIPv4Address, NCVersion: 17},
		importIPv6: {IPAddress: importIPv6Address, NCVersion: 17},
	}
	ncList := capitalizedNCListFixture(importNC1)
	cnsData, err := json.Marshal(map[string]any{
		"ContainerNetworkService": capitalizedCNSStateFixture{
			Location:         importLocation,
			NetworkType:      importNetworkType,
			OrchestratorType: importOrchestratorType,
			NodeID:           "capitalized-node",
			Initialized:      true,
			ContainerIDByOrchestratorContext: map[string]*capitalizedNCListFixture{
				"pod-capitalizedns-capitalized": &ncList,
			},
			ContainerStatus: map[string]capitalizedContainerStatusFixture{
				importNC1: {
					ID:                            importNC1,
					VMVersion:                     "17",
					HostVersion:                   "16",
					CreateNetworkContainerRequest: request,
					VfpUpdateComplete:             true,
				},
			},
			Networks: map[string]*capitalizedNetworkInfoFixture{
				"capitalized-network": {
					NetworkName: "capitalized-network",
					NicInfo: &wireserver.InterfaceInfo{
						Subnet:       importSubnetCIDR,
						Gateway:      importGatewayIP,
						PrimaryIP:    "10.0.0.2",
						SecondaryIPs: []string{"10.0.0.3"},
						IsPrimary:    true,
					},
					Options: map[string]any{"mode": "legacy"},
				},
			},
			TimeStamp: testNow,
			PnpIDByMacAddress: map[string]string{
				importPnPMAC: "PCI\\VEN_CAPITALIZED",
			},
		},
	})
	require.NoError(t, err)

	endpointData, err := json.Marshal(map[string]any{
		"Endpoints": map[string]*capitalizedEndpointFixture{
			"capitalized-container": {
				PodName:      "pod-capitalized",
				PodNamespace: "ns-capitalized",
				IfnameToIPMap: map[string]*capitalizedIPInfoFixture{
					importIfname: {
						IPv4:               []net.IPNet{mustIPNet(t, importIPv4Address+"/24")},
						IPv6:               []net.IPNet{mustIPNet(t, importIPv6Address+"/64")},
						HnsEndpointID:      "hns-endpoint-capitalized",
						HnsNetworkID:       "hns-network-capitalized",
						HostVethName:       "veth-capitalized",
						MacAddress:         importMAC,
						NetworkContainerID: importNC1,
						NICType:            cns.InfraNIC,
					},
				},
			},
		},
	})
	require.NoError(t, err)

	for _, key := range []string{
		"Location",
		"NetworkType",
		"OrchestratorType",
		"NodeID",
		"Initialized",
		"ContainerIDByOrchestratorContext",
		"ContainerStatus",
		"Networks",
		"TimeStamp",
		"PnpIDByMacAddress",
		"CreateNetworkContainerRequest",
		"AuthorizationToken",
		"SecondaryIPConfigs",
	} {
		assert.Contains(t, string(cnsData), `"`+key+`"`)
	}
	for _, key := range []string{
		"PodName",
		"PodNamespace",
		"IfnameToIPMap",
		importIPv4Key,
		importIPv6Key,
		"HnsEndpointID",
		"HnsNetworkID",
		"HostVethName",
		"MacAddress",
		"NetworkContainerID",
		"NICType",
	} {
		assert.Contains(t, string(endpointData), `"`+key+`"`)
	}
	assert.NotContains(t, string(cnsData), `"location"`)
	assert.NotContains(t, string(endpointData), `"podName"`)
	assert.NotContains(t, string(endpointData), `"hnsEndpointID"`)

	db, _ := openTestDB(t)
	opts := writeLegacyImportFiles(t, cnsData, endpointData, true)
	changed, err := db.ImportLegacy(context.Background(), opts)
	require.NoError(t, err)
	require.True(t, changed)

	snapshot := requireValidSnapshot(t, db)
	assert.Equal(t, "westus2", snapshot.Metadata.Location)
	assert.Equal(t, "capitalized-node", snapshot.Metadata.NodeID)
	assert.Equal(t, testNow, snapshot.Metadata.TimeStamp)
	assert.True(t, snapshot.Metadata.LegacyImportComplete)

	record := snapshot.NetworkContainers[importNC1]
	expectedRequest := request
	expectedRequest.AuthorizationToken = ""
	expectedRequest.SecondaryIPConfigs = nil
	assert.Equal(t, "17", record.VMVersion)
	assert.Equal(t, "16", record.HostVersion)
	assert.True(t, record.VFPUpdateComplete)
	assert.Equal(t, expectedRequest, record.Request)
	assert.Equal(t, map[string]IPRecord{
		importIPv4: {ID: importIPv4, IPAddress: "10.0.0.4", NCID: importNC1, NCVersion: 17},
		importIPv6: {ID: importIPv6, IPAddress: importIPv6Address, NCID: importNC1, NCVersion: 17},
	}, snapshot.IPs)

	assert.Equal(t, []string{importNC1}, snapshot.OrchestratorContexts["pod-capitalizedns-capitalized"])
	assert.Equal(t, "PCI\\VEN_CAPITALIZED", snapshot.PnPIDByMAC["00:11:22:33:44:55"])
	network := snapshot.Networks["capitalized-network"]
	assert.Equal(t, "10.0.0.0/24", network.NicInfo.Subnet)
	assert.Equal(t, "legacy", network.Options["mode"])

	endpoint := snapshot.Endpoints["capitalized-container"]
	assert.Equal(t, "pod-capitalized", endpoint.PodName)
	assert.Equal(t, "ns-capitalized", endpoint.PodNamespace)
	info := endpoint.IfnameToIPMap["eth0"]
	assert.Equal(t, "hns-endpoint-capitalized", info.HNSEndpointID)
	assert.Equal(t, "hns-network-capitalized", info.HNSNetworkID)
	assert.Equal(t, "veth-capitalized", info.HostVethName)
	assert.Equal(t, "00:11:22:33:44:55", info.MACAddress)
	assert.Equal(t, importNC1, info.NetworkContainerID)
	assert.Equal(t, cns.InfraNIC, info.NICType)
	require.Len(t, info.IPv4, 1)
	require.Len(t, info.IPv6, 1)
	assert.Equal(t, "10.0.0.4/24", info.IPv4[0].String())
	assert.Equal(t, "fd00::4/64", info.IPv6[0].String())

	assignment := snapshot.Assignments["capitalized-container"]
	assert.Equal(t, "capitalized-container", assignment.Pod.PodKey)
	assert.Equal(t, "capitalized-container", assignment.Pod.InfraContainerID)
	assert.Equal(t, "pod-capitalized", assignment.Pod.PodName)
	assert.Equal(t, "ns-capitalized", assignment.Pod.PodNamespace)
	assert.ElementsMatch(t, []string{importIPv4, importIPv6}, assignment.IPIDs)
	assert.Equal(t, "capitalized-container", snapshot.IPOwners[importIPv4])
	assert.Equal(t, "capitalized-container", snapshot.IPOwners[importIPv6])
}
