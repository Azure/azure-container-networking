package restserver

import (
	"strconv"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/common"
	"github.com/Azure/azure-container-networking/cns/types"
	acn "github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/Azure/azure-container-networking/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	versionValidationNCID               = "version-validation-nc"
	versionValidationSecondaryIP        = "10.0.0.6"
	versionValidationChangedSecondaryIP = "10.0.0.7"
	versionValidationDNSServer          = "10.0.0.10"
	versionValidationMACAddress         = "00:11:22:33:44:55"
	versionValidationInitialVersion     = 2
)

func TestSaveNetworkContainerGoalStateRejectsVersionRegression(t *testing.T) {
	svc, committed := newVersionValidationService(t)
	incoming := cloneCreateNetworkContainerRequest(committed)
	incoming.Version = "1"
	for ipID, config := range incoming.SecondaryIPConfigs {
		config.NCVersion = 1
		incoming.SecondaryIPConfigs[ipID] = config
	}

	returnCode, message := svc.saveNetworkContainerGoalState(incoming, true)

	assert.Equal(t, types.UnsupportedNCVersion, returnCode)
	assert.Contains(t, message, "version regressed from 2 to 1")
	assertCommittedGoalVersion(t, svc, "2")
}

func TestSaveNetworkContainerGoalStateAcceptsVersionAdvance(t *testing.T) {
	svc, committed := newVersionValidationService(t)
	incoming := cloneCreateNetworkContainerRequest(committed)
	incoming.Version = "3"
	for ipID, config := range incoming.SecondaryIPConfigs {
		config.NCVersion = 3
		incoming.SecondaryIPConfigs[ipID] = config
	}

	returnCode, message := svc.saveNetworkContainerGoalState(incoming, true)

	assert.Equal(t, types.Success, returnCode)
	assert.Empty(t, message)
	assertCommittedGoalVersion(t, svc, "3")
}

func TestSaveNetworkContainerGoalStateAcceptsIdenticalVersionReplay(t *testing.T) {
	svc, committed := newVersionValidationService(t)

	returnCode, message := svc.saveNetworkContainerGoalState(cloneCreateNetworkContainerRequest(committed), true)

	assert.Equal(t, types.Success, returnCode)
	assert.Empty(t, message)
	assertCommittedGoalVersion(t, svc, "2")
}

func TestSaveNetworkContainerGoalStateRejectsEqualVersionGoalDrift(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*cns.CreateNetworkContainerRequest)
	}{
		{
			name: "host primary IP",
			mutate: func(req *cns.CreateNetworkContainerRequest) {
				req.HostPrimaryIP = "10.1.0.11"
			},
		},
		{
			name: "network container type",
			mutate: func(req *cns.CreateNetworkContainerRequest) {
				req.NetworkContainerType = cns.Kubernetes
			},
		},
		{
			name: "primary subnet",
			mutate: func(req *cns.CreateNetworkContainerRequest) {
				req.IPConfiguration.IPSubnet.IPAddress = "10.0.1.5"
			},
		},
		{
			name: "IPv6 subnet",
			mutate: func(req *cns.CreateNetworkContainerRequest) {
				req.IPConfiguration.IPSubnetV6.IPAddress = "fd00::6"
			},
		},
		{
			name: "DNS servers",
			mutate: func(req *cns.CreateNetworkContainerRequest) {
				req.IPConfiguration.DNSServers = []string{"10.0.0.11"}
			},
		},
		{
			name: "gateway",
			mutate: func(req *cns.CreateNetworkContainerRequest) {
				req.IPConfiguration.GatewayIPAddress = "10.0.0.2"
			},
		},
		{
			name: "IPv6 gateway",
			mutate: func(req *cns.CreateNetworkContainerRequest) {
				req.IPConfiguration.GatewayIPv6Address = "fd00::2"
			},
		},
		{
			name: "NC status",
			mutate: func(req *cns.CreateNetworkContainerRequest) {
				req.NCStatus = v1alpha.NCStatus("updated")
			},
		},
		{
			name: "network interface",
			mutate: func(req *cns.CreateNetworkContainerRequest) {
				req.NetworkInterfaceInfo.MACAddress = "00:11:22:33:44:66"
			},
		},
		{
			name: "secondary IP address",
			mutate: func(req *cns.CreateNetworkContainerRequest) {
				config := req.SecondaryIPConfigs["ip-id-1"]
				config.IPAddress = versionValidationChangedSecondaryIP
				req.SecondaryIPConfigs["ip-id-1"] = config
			},
		},
		{
			name: "secondary IP ID",
			mutate: func(req *cns.CreateNetworkContainerRequest) {
				config := req.SecondaryIPConfigs["ip-id-1"]
				delete(req.SecondaryIPConfigs, "ip-id-1")
				req.SecondaryIPConfigs["replacement-id"] = config
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, committed := newVersionValidationService(t)
			incoming := cloneCreateNetworkContainerRequest(committed)
			tt.mutate(&incoming)
			before := cloneCreateNetworkContainerRequest(svc.state.ContainerStatus[versionValidationNCID].CreateNetworkContainerRequest)
			beforeIPCount := len(svc.PodIPConfigState)

			returnCode, message := svc.saveNetworkContainerGoalState(incoming, true)

			assert.Equal(t, types.InconsistentIPConfigState, returnCode)
			assert.Contains(t, message, "goal changed without version advance")
			assert.Equal(t, before, svc.state.ContainerStatus[versionValidationNCID].CreateNetworkContainerRequest)
			assert.Len(t, svc.PodIPConfigState, beforeIPCount)
			assertCommittedGoalVersion(t, svc, "2")
		})
	}
}

func newVersionValidationService(t *testing.T) (*HTTPRestService, cns.CreateNetworkContainerRequest) {
	t.Helper()

	svc := &HTTPRestService{
		store: store.NewMockStore(""),
		state: &httpRestServiceState{
			OrchestratorType: cns.KubernetesCRD,
			ContainerStatus:  make(map[string]containerstatus),
		},
		PodIPConfigState: make(map[string]cns.IPConfigurationStatus),
	}
	req := cns.CreateNetworkContainerRequest{
		HostPrimaryIP:        "10.1.0.10",
		Version:              "2",
		NetworkContainerType: cns.Docker,
		NetworkContainerid:   versionValidationNCID,
		IPConfiguration: cns.IPConfiguration{
			IPSubnet: cns.IPSubnet{
				IPAddress:    primaryIP,
				PrefixLength: 24,
			},
			IPSubnetV6: cns.IPSubnet{
				IPAddress:    "fd00::5",
				PrefixLength: 64,
			},
			DNSServers:         []string{versionValidationDNSServer},
			GatewayIPAddress:   "10.0.0.1",
			GatewayIPv6Address: "fd00::1",
		},
		SecondaryIPConfigs: map[string]cns.SecondaryIPConfig{
			"ip-id-1": {
				IPAddress: versionValidationSecondaryIP,
				NCVersion: versionValidationInitialVersion,
			},
		},
		NCStatus: v1alpha.NCStatus("ready"),
		NetworkInterfaceInfo: cns.NetworkInterfaceInfo{
			MACAddress: versionValidationMACAddress,
		},
	}
	req.Version = strconv.Itoa(versionValidationInitialVersion)

	returnCode, message := svc.saveNetworkContainerGoalState(req, true)
	require.Equal(t, types.Success, returnCode)
	require.Empty(t, message)

	return svc, req
}

func cloneCreateNetworkContainerRequest(req cns.CreateNetworkContainerRequest) cns.CreateNetworkContainerRequest {
	cloned := req
	cloned.IPConfiguration.DNSServers = append([]string(nil), req.IPConfiguration.DNSServers...)
	cloned.SecondaryIPConfigs = make(map[string]cns.SecondaryIPConfig, len(req.SecondaryIPConfigs))
	for ipID, config := range req.SecondaryIPConfigs {
		cloned.SecondaryIPConfigs[ipID] = config
	}
	return cloned
}

func assertCommittedGoalVersion(t *testing.T, svc *HTTPRestService, want string) {
	t.Helper()

	assert.Equal(t, want, svc.state.ContainerStatus[versionValidationNCID].CreateNetworkContainerRequest.Version)

	var persisted httpRestServiceState
	require.NoError(t, svc.store.Read(storeKey, &persisted))
	assert.Equal(t, want, persisted.ContainerStatus[versionValidationNCID].CreateNetworkContainerRequest.Version)
}

func TestAreNCsPresent(t *testing.T) {
	present := ncList("present")
	tests := []struct {
		name    string
		service HTTPRestService
		want    bool
	}{
		{
			name: "container status present",
			service: HTTPRestService{
				state: &httpRestServiceState{
					ContainerStatus: map[string]containerstatus{
						"nc1": {},
					},
				},
			},
			want: true,
		},
		{
			name: "containerIDByOrchestorContext present",
			service: HTTPRestService{
				state: &httpRestServiceState{
					ContainerIDByOrchestratorContext: map[string]*ncList{
						"nc1": &present,
					},
				},
			},
			want: true,
		},
		{
			name: "neither containerStatus nor containerIDByOrchestratorContext present",
			service: HTTPRestService{
				state: &httpRestServiceState{},
			},
			want: false,
		},
	}
	for _, tt := range tests { //nolint:govet // this mutex copy is to keep a local reference to this variable in the test func closure, and is ok
		tt := tt //nolint:govet // this mutex copy is to keep a local reference to this variable in the test func closure, and is ok
		t.Run(tt.name, func(t *testing.T) {
			got := tt.service.areNCsPresent()
			assert.Equal(t, got, tt.want)
		})
	}
}

// test to add unique nc to ncList for Add() method
func TestAddNCs(t *testing.T) {
	var ncs ncList

	tests := []struct {
		name string
		want ncList
	}{
		{
			name: "test add NCs",
			want: "swift_1abc,swift_2abc,swift_3abc",
		},
		{
			name: "test add duplicated NCs",
			want: "swift_1abc,swift_2abc,swift_3abc",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ncs.Add("swift_1abc")
			ncs.Add("swift_2abc")
			ncs.Add("swift_3abc")
			// test if added nc will be combined to one string with "," separated
			assert.Equal(t, tt.want, ncs)

			// test if duplicated nc("swift_3abc") cannot be added to ncList
			ncs.Add("swift_3abc")
			assert.Equal(t, tt.want, ncs)
		})
	}
}

// test to check if ncList contains specific NC for Containers() method
func TestContainsNC(t *testing.T) {
	var ncs ncList

	tests := []struct {
		name  string
		want1 bool
		want2 bool
	}{
		{
			name:  "test NC is in ncList",
			want1: true,
			want2: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ncs.Add("swift_1abc")
			ncs.Add("swift_2abc")
			assert.Equal(t, tt.want1, ncs.Contains("swift_1abc"))
			assert.Equal(t, tt.want2, ncs.Contains("swift_3abc"))
		})
	}
}

func TestRestoreState(t *testing.T) {
	tests := []struct {
		name                 string
		writeMainState       bool
		manageEndpointState  bool
		nilEndpointStore     bool
		wantEndpointRestored bool
	}{
		{
			name:                 "endpoint state restored when main state read fails",
			manageEndpointState:  true,
			wantEndpointRestored: true,
		},
		{
			name:                 "endpoint state restored when main state succeeds",
			writeMainState:       true,
			manageEndpointState:  true,
			wantEndpointRestored: true,
		},
		{
			name:                 "skips endpoint state when OptManageEndpointState not set",
			wantEndpointRestored: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mainStore := store.NewMockStore("")
			if tt.writeMainState {
				require.NoError(t, mainStore.Write(storeKey, &httpRestServiceState{}))
			}

			var endpointStore store.KeyValueStore
			if !tt.nilEndpointStore {
				endpointStore = store.NewMockStore("")
				require.NoError(t, endpointStore.Write(EndpointStoreKey, map[string]*EndpointInfo{
					"container1": {PodName: "pod1"},
				}))
			}

			options := map[string]interface{}{}
			if tt.manageEndpointState {
				options[acn.OptManageEndpointState] = true
			}

			svc := HTTPRestService{
				Service: &cns.Service{
					Service: &common.Service{Options: options},
				},
				store:              mainStore,
				state:              &httpRestServiceState{},
				EndpointStateStore: endpointStore,
				EndpointState:      make(map[string]*EndpointInfo),
			}

			svc.restoreState()

			if tt.wantEndpointRestored {
				require.Len(t, svc.EndpointState, 1)
				assert.Equal(t, "pod1", svc.EndpointState["container1"].PodName)
			} else {
				assert.Empty(t, svc.EndpointState)
			}
		})
	}
}

// test to check if nc can be deleted from ncList for Delete() method
func TestDeleteNCs(t *testing.T) {
	var ncs ncList

	tests := []struct {
		name  string
		want1 ncList
		want2 ncList
		want3 ncList
		want4 ncList
	}{
		{
			name:  "test to delete NC from ncList",
			want1: "swift_1abc,swift_3abc,swift_4abc",
			want2: "swift_3abc,swift_4abc",
			want3: "swift_3abc",
			want4: "",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ncs.Add("swift_1abc")
			ncs.Add("swift_2abc")
			ncs.Add("swift_3abc")
			ncs.Add("swift_4abc")

			// remove "swift_2abc" from ncList
			ncs.Delete("swift_2abc")
			assert.Equal(t, tt.want1, ncs)

			// remove "swift_1abc" from ncList
			ncs.Delete("swift_1abc")
			assert.Equal(t, tt.want2, ncs)

			// remove "swift_4abc" from ncList
			ncs.Delete("swift_4abc")
			assert.Equal(t, tt.want3, ncs)

			// remove "swift_3abc" from ncList and check if ncList become ""
			ncs.Delete("swift_3abc")
			assert.Equal(t, tt.want4, ncs)
		})
	}
}
