package restserver

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/common"
	"github.com/Azure/azure-container-networking/cns/types"
	acn "github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
		nilMainStore         bool
		writeMainState       bool
		manageEndpointState  bool
		nilEndpointStore     bool
		wantEndpointRestored bool
		wantErr              error
	}{
		{
			name:                 "endpoint state restored when main state read fails",
			manageEndpointState:  true,
			wantEndpointRestored: true,
		},
		{
			name:                 "endpoint state restored when main store is nil",
			nilMainStore:         true,
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
		{
			name:                "fails when endpoint state management has no store",
			manageEndpointState: true,
			nilEndpointStore:    true,
			wantErr:             ErrStoreEmpty,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mainStore store.KeyValueStore
			if !tt.nilMainStore {
				mainStore = store.NewMockStore("")
			}
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

			err := svc.restoreState()
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)

			if tt.wantEndpointRestored {
				require.Len(t, svc.EndpointState, 1)
				assert.Equal(t, "pod1", svc.EndpointState["container1"].PodName)
			} else {
				assert.Empty(t, svc.EndpointState)
			}
		})
	}
}

func TestRestoreStateFailsClosedWhenEndpointStateCannotBeRead(t *testing.T) {
	mainStore := store.NewMockStore("")
	endpointStore := store.NewMockStore("")
	svc := HTTPRestService{
		Service: &cns.Service{
			Service: &common.Service{Options: map[string]interface{}{acn.OptManageEndpointState: true}},
		},
		store: mainStore,
		state: &httpRestServiceState{},
		EndpointStateStore: keyReadFailStore{
			KeyValueStore: endpointStore,
			failKey:       EndpointStoreKey,
			err:           errForcedEndpointStateRead,
		},
		EndpointState: make(map[string]*EndpointInfo),
	}

	require.ErrorIs(t, svc.restoreState(), errForcedEndpointStateRead)
}

var errForcedEndpointStateRead = errors.New("forced endpoint state read failure")

type keyReadFailStore struct {
	store.KeyValueStore
	failKey string
	err     error
}

func (s keyReadFailStore) Read(key string, value interface{}) error {
	if key == s.failKey {
		return s.err
	}
	if err := s.KeyValueStore.Read(key, value); err != nil {
		return fmt.Errorf("reading key %q: %w", key, err)
	}
	return nil
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

//nolint:goconst // Keep each address visible in its uniqueness scenario.
func TestValidateNetworkContainerIPUniqueness(t *testing.T) {
	const (
		nc1 = "nc1"
		nc2 = "nc2"
	)

	assignedIPConfig := cns.IPConfigurationStatus{ID: "ip1", NCID: nc1, IPAddress: "10.0.0.2"}
	assignedIPConfig.SetState(types.Assigned)
	availableIPConfig := cns.IPConfigurationStatus{ID: "ip1", NCID: nc1, IPAddress: "10.0.0.2"}
	availableIPConfig.SetState(types.Available)

	tests := []struct {
		name               string
		existing           []cns.CreateNetworkContainerRequest
		currentIPConfigs   map[string]cns.IPConfigurationStatus
		incoming           cns.CreateNetworkContainerRequest
		allowSharedPrimary bool
		wantCode           types.ResponseCode
	}{
		{
			name:     "distinct addresses",
			existing: []cns.CreateNetworkContainerRequest{ipUniquenessRequest(nc1, "10.0.0.1", "", map[string]string{"ip1": "10.0.0.2"})},
			incoming: ipUniquenessRequest(nc2, "10.0.0.9", "", map[string]string{"ip2": "10.0.0.10"}),
			wantCode: types.Success,
		},
		{
			name:     "mapped IPv4 duplicate",
			existing: []cns.CreateNetworkContainerRequest{ipUniquenessRequest(nc1, "10.0.0.1", "", map[string]string{"ip1": "10.0.0.2"})},
			incoming: ipUniquenessRequest(nc2, "10.0.0.9", "", map[string]string{"ip2": "::ffff:10.0.0.2"}),
			wantCode: types.InconsistentIPConfigState,
		},
		{
			name:     "canonical IPv6 duplicate",
			existing: []cns.CreateNetworkContainerRequest{ipUniquenessRequest(nc1, "2001:db8::1", "", map[string]string{"ip1": "2001:db8::2"})},
			incoming: ipUniquenessRequest(nc2, "2001:db8::9", "", map[string]string{"ip2": "2001:0db8:0:0:0:0:0:2"}),
			wantCode: types.InconsistentIPConfigState,
		},
		{
			name:     "duplicate incoming secondaries",
			incoming: ipUniquenessRequest(nc1, "10.0.0.1", "", map[string]string{"ip1": "10.0.0.2", "ip2": "::ffff:10.0.0.2"}),
			wantCode: types.InconsistentIPConfigState,
		},
		{
			name:     "duplicate IPv4 primary",
			existing: []cns.CreateNetworkContainerRequest{ipUniquenessRequest(nc1, "10.0.0.1", "", map[string]string{"ip1": "10.0.0.2"})},
			incoming: ipUniquenessRequest(nc2, "10.0.0.1", "", map[string]string{"ip2": "10.0.0.3"}),
			wantCode: types.InconsistentIPConfigState,
		},
		{
			name:     "duplicate IPv6 primary",
			existing: []cns.CreateNetworkContainerRequest{ipUniquenessRequest(nc1, "10.0.0.1", "2001:db8::1", map[string]string{"ip1": "2001:db8::2"})},
			incoming: ipUniquenessRequest(nc2, "10.0.0.9", "2001:0db8:0:0:0:0:0:1", map[string]string{"ip2": "2001:db8::3"}),
			wantCode: types.InconsistentIPConfigState,
		},
		{
			name:     "primary collides with secondary across families",
			existing: []cns.CreateNetworkContainerRequest{ipUniquenessRequest(nc1, "2001:db8::1", "", map[string]string{"ip1": "10.0.0.2"})},
			incoming: ipUniquenessRequest(nc2, "10.0.0.2", "2001:db8::9", map[string]string{"ip2": "2001:db8::10"}),
			wantCode: types.InconsistentIPConfigState,
		},
		{
			name:     "secondary collides with primary across families",
			existing: []cns.CreateNetworkContainerRequest{ipUniquenessRequest(nc1, "10.0.0.1", "2001:db8::1", map[string]string{"ip1": "2001:db8::2"})},
			incoming: ipUniquenessRequest(nc2, "10.0.0.9", "", map[string]string{"ip2": "10.0.0.1"}),
			wantCode: types.InconsistentIPConfigState,
		},
		{
			name:     "same network container primary and secondary overlap",
			incoming: ipUniquenessRequest(nc1, "10.0.0.1", "", map[string]string{"primary": "10.0.0.1"}),
			wantCode: types.Success,
		},
		{
			name:     "shared primary across address families",
			existing: []cns.CreateNetworkContainerRequest{ipUniquenessRequest(nc1, "10.0.0.1", "", map[string]string{"ip1": "10.0.0.2"})},
			incoming: ipUniquenessRequest(nc2, "10.0.0.1", "2001:db8::1", map[string]string{"ip2": "2001:db8::2"}),
			wantCode: types.Success,
		},
		{
			name:     "contradictory family data collides conservatively",
			existing: []cns.CreateNetworkContainerRequest{ipUniquenessRequest(nc1, "10.0.0.1", "", map[string]string{"ip1": "10.0.0.2", "ip2": "2001:db8::2"})},
			incoming: ipUniquenessRequest(nc2, "10.0.0.1", "", map[string]string{"ip3": "10.0.0.3"}),
			wantCode: types.InconsistentIPConfigState,
		},
		{
			name:     "IP ID cannot move across network containers",
			existing: []cns.CreateNetworkContainerRequest{ipUniquenessRequest(nc1, "10.0.0.1", "", map[string]string{"shared": "10.0.0.2"})},
			incoming: ipUniquenessRequest(nc2, "10.0.0.9", "", map[string]string{"shared": "10.0.0.10"}),
			wantCode: types.InconsistentIPConfigState,
		},
		{
			name:     "same network container can replace an IP ID address",
			existing: []cns.CreateNetworkContainerRequest{ipUniquenessRequest(nc1, "10.0.0.1", "", map[string]string{"ip1": "10.0.0.2"})},
			incoming: ipUniquenessRequest(nc1, "10.0.0.1", "", map[string]string{"ip1": "10.0.0.3"}),
			wantCode: types.Success,
		},
		{
			name:             "assigned IP ID cannot change address",
			existing:         []cns.CreateNetworkContainerRequest{ipUniquenessRequest(nc1, "10.0.0.1", "", map[string]string{"ip1": "10.0.0.2"})},
			currentIPConfigs: map[string]cns.IPConfigurationStatus{"ip1": assignedIPConfig},
			incoming:         ipUniquenessRequest(nc1, "10.0.0.1", "", map[string]string{"ip1": "10.0.0.3"}),
			wantCode:         types.InconsistentIPConfigState,
		},
		{
			name:             "available IP ID can change address",
			existing:         []cns.CreateNetworkContainerRequest{ipUniquenessRequest(nc1, "10.0.0.1", "", map[string]string{"ip1": "10.0.0.2"})},
			currentIPConfigs: map[string]cns.IPConfigurationStatus{"ip1": availableIPConfig},
			incoming:         ipUniquenessRequest(nc1, "10.0.0.1", "", map[string]string{"ip1": "10.0.0.3"}),
			wantCode:         types.Success,
		},
		{
			name:     "invalid incoming primary",
			incoming: ipUniquenessRequest(nc1, "not-an-ip", "", map[string]string{"ip1": "10.0.0.2"}),
			wantCode: types.InvalidPrimaryIPConfig,
		},
		{
			name:     "IPv4 mapped address is invalid in IPv6 primary field",
			incoming: ipUniquenessRequest(nc1, "10.0.0.1", "::ffff:10.0.0.2", map[string]string{"ip1": "2001:db8::2"}),
			wantCode: types.InvalidPrimaryIPConfig,
		},
		{
			name:     "invalid incoming secondary",
			incoming: ipUniquenessRequest(nc1, "10.0.0.1", "", map[string]string{"ip1": "not-an-ip"}),
			wantCode: types.InvalidSecondaryIPConfig,
		},
		{
			name:     "scoped IPv6 incoming secondary",
			incoming: ipUniquenessRequest(nc1, "2001:db8::1", "", map[string]string{"ip1": "fe80::1%eth0"}),
			wantCode: types.InvalidSecondaryIPConfig,
		},
		{
			name:     "invalid cached secondary",
			existing: []cns.CreateNetworkContainerRequest{ipUniquenessRequest(nc1, "10.0.0.1", "", map[string]string{"ip1": "not-an-ip"})},
			incoming: ipUniquenessRequest(nc2, "10.0.0.9", "", map[string]string{"ip2": "10.0.0.10"}),
			wantCode: types.InconsistentIPConfigState,
		},
		{
			name: "invalid cached pod IP state",
			currentIPConfigs: map[string]cns.IPConfigurationStatus{
				"ip1": {ID: "ip1", NCID: nc1, IPAddress: "not-an-ip"},
			},
			incoming: ipUniquenessRequest(nc2, "10.0.0.9", "", map[string]string{"ip2": "10.0.0.10"}),
			wantCode: types.InconsistentIPConfigState,
		},
		{
			name: "pod IP state participates in collision detection",
			currentIPConfigs: map[string]cns.IPConfigurationStatus{
				"ip1": {ID: "ip1", NCID: nc1, IPAddress: "10.0.0.2"},
			},
			incoming: ipUniquenessRequest(nc2, "10.0.0.9", "", map[string]string{"ip2": "::ffff:10.0.0.2"}),
			wantCode: types.InconsistentIPConfigState,
		},
		{
			name: "legacy duplicate primary can be updated",
			existing: []cns.CreateNetworkContainerRequest{
				ipUniquenessRequest(nc1, "10.0.0.1", "", map[string]string{"ip1": "10.0.0.2"}),
				ipUniquenessRequest(nc2, "10.0.0.1", "", map[string]string{"ip2": "10.0.0.3"}),
			},
			incoming: ipUniquenessRequest(nc1, "10.0.0.1", "", map[string]string{"ip1": "10.0.0.4"}),
			wantCode: types.Success,
		},
		{
			name:               "Kubernetes shared primary compatibility",
			existing:           []cns.CreateNetworkContainerRequest{ipUniquenessRequest(nc1, "10.0.0.1", "", map[string]string{"ip1": "10.0.0.2"})},
			incoming:           ipUniquenessRequest(nc2, "10.0.0.1", "", map[string]string{"ip2": "10.0.0.3"}),
			allowSharedPrimary: true,
			wantCode:           types.Success,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			currentContainers := make(map[string]containerstatus, len(tt.existing))
			for i := range tt.existing {
				request := tt.existing[i]
				currentContainers[request.NetworkContainerid] = containerstatus{ID: request.NetworkContainerid, CreateNetworkContainerRequest: request}
			}

			responseCode, _ := validateNetworkContainerIPUniqueness(currentContainers, tt.currentIPConfigs, tt.incoming, tt.allowSharedPrimary)

			assert.Equal(t, tt.wantCode, responseCode)
		})
	}
}

func TestSaveNetworkContainerGoalStateRejectsDuplicateIPAddresses(t *testing.T) {
	for _, orchestratorType := range []string{cns.Kubernetes, cns.KubernetesCRD} {
		t.Run(orchestratorType, func(t *testing.T) {
			existing := ipUniquenessRequest("nc1", "10.0.0.1", "", map[string]string{"ip1": "10.0.0.2"})
			service := &HTTPRestService{
				state: &httpRestServiceState{
					OrchestratorType: orchestratorType,
					ContainerStatus: map[string]containerstatus{
						existing.NetworkContainerid: {ID: existing.NetworkContainerid, CreateNetworkContainerRequest: existing},
					},
				},
				PodIPConfigState: map[string]cns.IPConfigurationStatus{
					"ip1": {ID: "ip1", NCID: existing.NetworkContainerid, IPAddress: "10.0.0.2"},
				},
			}
			incoming := ipUniquenessRequest("nc2", "10.0.0.9", "", map[string]string{"ip2": "::ffff:10.0.0.2"})

			responseCode, _ := service.saveNetworkContainerGoalState(incoming)

			assert.Equal(t, types.InconsistentIPConfigState, responseCode)
			assert.Len(t, service.state.ContainerStatus, 1)
			assert.Equal(t, existing, service.state.ContainerStatus[existing.NetworkContainerid].CreateNetworkContainerRequest)
			assert.NotContains(t, service.state.ContainerStatus, incoming.NetworkContainerid)
			assert.Len(t, service.PodIPConfigState, 1)
			assert.Equal(t, "10.0.0.2", service.PodIPConfigState["ip1"].IPAddress)
			assert.NotContains(t, service.PodIPConfigState, "ip2")
		})
	}
}

func ipUniquenessRequest(ncID, primary, primaryV6 string, secondaryAddresses map[string]string) cns.CreateNetworkContainerRequest {
	secondaryIPConfigs := make(map[string]cns.SecondaryIPConfig, len(secondaryAddresses))
	for ipID, address := range secondaryAddresses {
		secondaryIPConfigs[ipID] = cns.SecondaryIPConfig{IPAddress: address, NCVersion: 1}
	}
	return cns.CreateNetworkContainerRequest{
		NetworkContainerType: cns.Docker,
		NetworkContainerid:   ncID,
		Version:              "1",
		IPConfiguration: cns.IPConfiguration{
			IPSubnet:   cns.IPSubnet{IPAddress: primary, PrefixLength: 24},
			IPSubnetV6: cns.IPSubnet{IPAddress: primaryV6, PrefixLength: 64},
		},
		SecondaryIPConfigs: secondaryIPConfigs,
	}
}
