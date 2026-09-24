package restserver

import (
	"errors"
	"fmt"
	"net/netip"
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
	pendingReleaseIPConfig := cns.IPConfigurationStatus{ID: "ip1", NCID: nc1, IPAddress: "10.0.0.2"}
	pendingReleaseIPConfig.SetState(types.PendingRelease)
	orphanedAvailableIPConfig := cns.IPConfigurationStatus{ID: "orphaned", NCID: "deleted", IPAddress: "10.0.0.2"}
	orphanedAvailableIPConfig.SetState(types.Available)
	orphanedPendingReleaseIPConfig := cns.IPConfigurationStatus{ID: "orphaned", NCID: "deleted", IPAddress: "10.0.0.2"}
	orphanedPendingReleaseIPConfig.SetState(types.PendingRelease)
	orphanedAssignedIPConfig := cns.IPConfigurationStatus{ID: "orphaned", NCID: "deleted", IPAddress: "10.0.0.2"}
	orphanedAssignedIPConfig.SetState(types.Assigned)
	malformedAssignedIPConfig := cns.IPConfigurationStatus{ID: "ip1", NCID: nc1, IPAddress: "not-an-ip"}
	malformedAssignedIPConfig.SetState(types.Assigned)

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
			name:     "shared host primary",
			existing: []cns.CreateNetworkContainerRequest{ipUniquenessRequestWithHostPrimary(nc1, "10.0.0.1", map[string]string{"ip1": "10.0.0.2"})},
			incoming: ipUniquenessRequestWithHostPrimary(nc2, "10.0.0.1", map[string]string{"ip2": "10.0.0.3"}),
			wantCode: types.Success,
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
			name:             "pending release IP ID cannot change address",
			existing:         []cns.CreateNetworkContainerRequest{ipUniquenessRequest(nc1, "10.0.0.1", "", map[string]string{"ip1": "10.0.0.2"})},
			currentIPConfigs: map[string]cns.IPConfigurationStatus{"ip1": pendingReleaseIPConfig},
			incoming:         ipUniquenessRequest(nc1, "10.0.0.1", "", map[string]string{"ip1": "10.0.0.3"}),
			wantCode:         types.InconsistentIPConfigState,
		},
		{
			name:             "malformed assigned IP ID cannot change address",
			existing:         []cns.CreateNetworkContainerRequest{ipUniquenessRequest(nc1, "10.0.0.1", "", map[string]string{"ip1": "not-an-ip"})},
			currentIPConfigs: map[string]cns.IPConfigurationStatus{"ip1": malformedAssignedIPConfig},
			incoming:         ipUniquenessRequest(nc1, "10.0.0.1", "", map[string]string{"ip1": "10.0.0.3"}),
			wantCode:         types.InconsistentIPConfigState,
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
			wantCode: types.Success,
		},
		{
			name: "invalid cached pod IP state",
			currentIPConfigs: map[string]cns.IPConfigurationStatus{
				"ip1": {ID: "ip1", NCID: nc1, IPAddress: "not-an-ip"},
			},
			incoming: ipUniquenessRequest(nc2, "10.0.0.9", "", map[string]string{"ip2": "10.0.0.10"}),
			wantCode: types.Success,
		},
		{
			name: "orphaned available pool entry does not block reuse",
			currentIPConfigs: map[string]cns.IPConfigurationStatus{
				"orphaned": orphanedAvailableIPConfig,
			},
			incoming: ipUniquenessRequest(nc2, "10.0.0.9", "", map[string]string{"ip2": "10.0.0.2"}),
			wantCode: types.Success,
		},
		{
			name: "orphaned pending release entry does not block reuse",
			currentIPConfigs: map[string]cns.IPConfigurationStatus{
				"orphaned": orphanedPendingReleaseIPConfig,
			},
			incoming: ipUniquenessRequest(nc2, "10.0.0.9", "", map[string]string{"ip2": "10.0.0.2"}),
			wantCode: types.Success,
		},
		{
			name: "orphaned assigned pool entry blocks reuse",
			currentIPConfigs: map[string]cns.IPConfigurationStatus{
				"orphaned": orphanedAssignedIPConfig,
			},
			incoming: ipUniquenessRequest(nc2, "10.0.0.9", "", map[string]string{"ip2": "10.0.0.2"}),
			wantCode: types.InconsistentIPConfigState,
		},
		{
			name: "orphaned unknown pod IP state blocks reuse",
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
			name: "legacy duplicate secondary does not block unrelated update",
			existing: []cns.CreateNetworkContainerRequest{
				ipUniquenessRequest(nc1, "10.0.0.1", "", map[string]string{"ip1": "10.0.0.2"}),
				ipUniquenessRequest(nc2, "10.0.0.9", "", map[string]string{"ip2": "::ffff:10.0.0.2"}),
			},
			incoming: ipUniquenessRequest("nc3", "10.0.0.17", "", map[string]string{"ip3": "10.0.0.18"}),
			wantCode: types.Success,
		},
		{
			name: "unchanged primary still collides with cached secondary",
			existing: []cns.CreateNetworkContainerRequest{
				ipUniquenessRequest(nc1, "10.0.0.1", "", map[string]string{"ip1": "10.0.0.2"}),
				ipUniquenessRequest(nc2, "10.0.0.9", "", map[string]string{"ip2": "10.0.0.1"}),
			},
			incoming: ipUniquenessRequest(nc1, "10.0.0.1", "", map[string]string{"ip1": "10.0.0.3"}),
			wantCode: types.InconsistentIPConfigState,
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

func TestSaveNetworkContainerGoalStateUpdatesAvailableIPAddress(t *testing.T) {
	existing := ipUniquenessRequest("nc1", "10.0.0.1", "", map[string]string{"ip1": "10.0.0.2"})
	available := cns.IPConfigurationStatus{ID: "ip1", NCID: existing.NetworkContainerid, IPAddress: "10.0.0.2"}
	available.SetState(types.Available)
	service := &HTTPRestService{
		state: &httpRestServiceState{
			OrchestratorType: cns.KubernetesCRD,
			ContainerStatus: map[string]containerstatus{
				existing.NetworkContainerid: {ID: existing.NetworkContainerid, HostVersion: "1", CreateNetworkContainerRequest: existing},
			},
		},
		PodIPConfigState: map[string]cns.IPConfigurationStatus{"ip1": available},
	}

	updated := ipUniquenessRequest("nc1", "10.0.0.1", "", map[string]string{"ip1": "10.0.0.3"})
	updated.Version = "2"
	updatedIPConfig := updated.SecondaryIPConfigs["ip1"]
	updatedIPConfig.NCVersion = 2
	updated.SecondaryIPConfigs["ip1"] = updatedIPConfig
	responseCode, _ := service.saveNetworkContainerGoalState(updated)

	require.Equal(t, types.Success, responseCode)
	assert.Equal(t, "10.0.0.3", service.state.ContainerStatus["nc1"].CreateNetworkContainerRequest.SecondaryIPConfigs["ip1"].IPAddress)
	assert.Equal(t, 2, service.state.ContainerStatus["nc1"].CreateNetworkContainerRequest.SecondaryIPConfigs["ip1"].NCVersion)
	assert.Equal(t, "10.0.0.3", service.PodIPConfigState["ip1"].IPAddress)
	updatedStatus := service.PodIPConfigState["ip1"]
	assert.Equal(t, types.PendingProgramming, updatedStatus.GetState())

	service.MarkIpsAsAvailableUntransacted("nc1", 2)

	availableStatus := service.PodIPConfigState["ip1"]
	assert.Equal(t, types.Available, availableStatus.GetState())

	unrelated := ipUniquenessRequest("nc2", "10.0.0.9", "", map[string]string{"ip2": "10.0.0.10"})
	responseCode, _ = service.saveNetworkContainerGoalState(unrelated)

	assert.Equal(t, types.Success, responseCode)
	assert.Contains(t, service.state.ContainerStatus, "nc2")
}

func TestNetworkContainerIPValidatorChecksAllAddressIdentities(t *testing.T) {
	address := netip.MustParseAddr("10.0.0.2")
	validator := networkContainerIPValidator{
		addresses: map[netip.Addr][]networkContainerAddressIdentity{
			address: {
				{ncID: "nc1", ipID: "ip1", cached: true},
				{ncID: "deleted", ipID: "orphaned", cached: true},
			},
		},
	}

	responseCode, _ := validator.addAddress(address, networkContainerAddressIdentity{ncID: "nc1", ipID: "ip1"})

	assert.Equal(t, types.InconsistentIPConfigState, responseCode)
}

func TestSaveNetworkContainerGoalStateRepairsAvailableIPAddressAheadOfStoredGoal(t *testing.T) {
	existing := ipUniquenessRequest("nc1", "10.0.0.1", "", map[string]string{"ip1": "10.0.0.2"})
	ahead := cns.IPConfigurationStatus{ID: "ip1", NCID: existing.NetworkContainerid, IPAddress: "10.0.0.3"}
	ahead.SetState(types.Available)
	service := &HTTPRestService{
		state: &httpRestServiceState{
			OrchestratorType: cns.KubernetesCRD,
			ContainerStatus: map[string]containerstatus{
				existing.NetworkContainerid: {ID: existing.NetworkContainerid, HostVersion: "1", CreateNetworkContainerRequest: existing},
			},
		},
		PodIPConfigState: map[string]cns.IPConfigurationStatus{"ip1": ahead},
	}
	updated := ipUniquenessRequest("nc1", "10.0.0.1", "", map[string]string{"ip1": "10.0.0.3"})
	updated.Version = "2"
	updatedIPConfig := updated.SecondaryIPConfigs["ip1"]
	updatedIPConfig.NCVersion = 2
	updated.SecondaryIPConfigs["ip1"] = updatedIPConfig

	responseCode, _ := service.saveNetworkContainerGoalState(updated)

	require.Equal(t, types.Success, responseCode)
	assert.Equal(t, 2, service.state.ContainerStatus["nc1"].CreateNetworkContainerRequest.SecondaryIPConfigs["ip1"].NCVersion)
	updatedStatus := service.PodIPConfigState["ip1"]
	assert.Equal(t, types.PendingProgramming, updatedStatus.GetState())
}

func TestSaveNetworkContainerGoalStateReplacesOrphanedAvailableIPState(t *testing.T) {
	const incomingNCID = "nc2"

	tests := []struct {
		name          string
		orphanedNCID  string
		orphanedID    string
		orphanedState types.IPState
		incomingID    string
		incomingIP    string
	}{
		{
			name:          "same canonical address with different IP ID",
			orphanedID:    "orphaned",
			orphanedState: types.Available,
			incomingID:    "ip2",
			incomingIP:    "::ffff:10.0.0.2",
		},
		{
			name:          "same IP ID with different address",
			orphanedID:    "ip2",
			orphanedState: types.Available,
			incomingID:    "ip2",
			incomingIP:    "10.0.0.3",
		},
		{
			name:          "pending release IP ID with different address",
			orphanedID:    "ip2",
			orphanedState: types.PendingRelease,
			incomingID:    "ip2",
			incomingIP:    "10.0.0.3",
		},
		{
			name:          "same network container orphan",
			orphanedNCID:  incomingNCID,
			orphanedID:    "ip2",
			orphanedState: types.Available,
			incomingID:    "ip2",
			incomingIP:    "10.0.0.2",
		},
		{
			name:          "same network container pending release orphan with different IP ID",
			orphanedNCID:  incomingNCID,
			orphanedID:    "ip1",
			orphanedState: types.PendingRelease,
			incomingID:    "ip2",
			incomingIP:    "10.0.0.2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orphanedNCID := tt.orphanedNCID
			if orphanedNCID == "" {
				orphanedNCID = "deleted"
			}
			orphaned := cns.IPConfigurationStatus{ID: tt.orphanedID, NCID: orphanedNCID, IPAddress: "10.0.0.2"}
			orphaned.SetState(tt.orphanedState)
			service := &HTTPRestService{
				state: &httpRestServiceState{
					OrchestratorType: cns.KubernetesCRD,
					ContainerStatus:  map[string]containerstatus{},
				},
				PodIPConfigState: map[string]cns.IPConfigurationStatus{tt.orphanedID: orphaned},
			}
			incoming := ipUniquenessRequest(incomingNCID, "10.0.0.9", "", map[string]string{tt.incomingID: tt.incomingIP})

			responseCode, _ := service.saveNetworkContainerGoalState(incoming)

			require.Equal(t, types.Success, responseCode)
			assert.Len(t, service.PodIPConfigState, 1)
			if tt.orphanedID != tt.incomingID {
				assert.NotContains(t, service.PodIPConfigState, tt.orphanedID)
			}
			assert.Equal(t, incomingNCID, service.PodIPConfigState[tt.incomingID].NCID)
			assert.Equal(t, tt.incomingIP, service.PodIPConfigState[tt.incomingID].IPAddress)
			updatedStatus := service.PodIPConfigState[tt.incomingID]
			assert.Equal(t, types.PendingProgramming, updatedStatus.GetState())
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

func ipUniquenessRequestWithHostPrimary(ncID, primary string, secondaryAddresses map[string]string) cns.CreateNetworkContainerRequest {
	request := ipUniquenessRequest(ncID, primary, "", secondaryAddresses)
	request.HostPrimaryIP = primary
	return request
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

type failingWriteStore struct {
	store.KeyValueStore
	err error
}

var errForcedStateWrite = errors.New("forced state write failure")

func (s failingWriteStore) Write(string, interface{}) error {
	return s.err
}

//nolint:goconst // Keep each NC ID visible in its complete-goal scenario.
func TestApplyNNCGoalRejectsCompleteGoalBeforeMutation(t *testing.T) {
	service := &HTTPRestService{
		state: &httpRestServiceState{
			OrchestratorType: cns.KubernetesCRD,
			ContainerStatus:  map[string]containerstatus{},
		},
		PodIPConfigState: map[string]cns.IPConfigurationStatus{},
	}
	valid := ipUniquenessRequest("nc1", "10.0.0.1", "", map[string]string{"ip1": "10.0.0.2"})
	invalid := ipUniquenessRequest("nc2", "10.0.0.9", "", map[string]string{"ip2": ""})

	responseCode := service.ApplyNNCGoal(NNCGoal{
		NetworkContainerIDs: []string{"nc1", "nc2"},
		NetworkContainers: []NNCNetworkContainerGoal{
			{ValidateVersion: true, Request: valid},
			{ValidateVersion: true, Request: invalid},
		},
	})

	assert.Equal(t, types.InvalidSecondaryIPConfig, responseCode)
	assert.Empty(t, service.state.ContainerStatus)
	assert.Empty(t, service.PodIPConfigState)
}

func TestApplyNNCGoalRejectsDuplicateIncomingAddressBeforeMutation(t *testing.T) {
	service := &HTTPRestService{
		state: &httpRestServiceState{
			OrchestratorType: cns.KubernetesCRD,
			ContainerStatus:  map[string]containerstatus{},
		},
		PodIPConfigState: map[string]cns.IPConfigurationStatus{},
	}
	first := ipUniquenessRequest("nc1", "10.0.0.1", "", map[string]string{"ip1": "10.0.0.2"})
	second := ipUniquenessRequest("nc2", "10.0.0.9", "", map[string]string{"ip2": "::ffff:10.0.0.2"})

	responseCode := service.ApplyNNCGoal(NNCGoal{
		NetworkContainerIDs: []string{"nc1", "nc2"},
		NetworkContainers: []NNCNetworkContainerGoal{
			{ValidateVersion: true, Request: first},
			{ValidateVersion: true, Request: second},
		},
	})

	assert.Equal(t, types.InconsistentIPConfigState, responseCode)
	assert.Empty(t, service.state.ContainerStatus)
	assert.Empty(t, service.PodIPConfigState)
}

func TestApplyNNCGoalRejectsLaterVersionFailureBeforeMutation(t *testing.T) {
	first := ipUniquenessRequest("nc1", "10.0.0.1", "", map[string]string{"ip1": "10.0.0.2"})
	first.Version = "2"
	second := ipUniquenessRequest("nc2", "10.0.0.9", "", map[string]string{"ip2": "10.0.0.10"})
	second.Version = "2"
	firstIP := cns.IPConfigurationStatus{ID: "ip1", NCID: first.NetworkContainerid, IPAddress: "10.0.0.2"}
	firstIP.SetState(types.Available)
	secondIP := cns.IPConfigurationStatus{ID: "ip2", NCID: second.NetworkContainerid, IPAddress: "10.0.0.10"}
	secondIP.SetState(types.Available)
	service := &HTTPRestService{
		state: &httpRestServiceState{
			OrchestratorType: cns.KubernetesCRD,
			ContainerStatus: map[string]containerstatus{
				first.NetworkContainerid: {
					ID:                            first.NetworkContainerid,
					HostVersion:                   "2",
					CreateNetworkContainerRequest: first,
				},
				second.NetworkContainerid: {
					ID:                            second.NetworkContainerid,
					HostVersion:                   "2",
					CreateNetworkContainerRequest: second,
				},
			},
		},
		PodIPConfigState: map[string]cns.IPConfigurationStatus{"ip1": firstIP, "ip2": secondIP},
	}
	updatedFirst := ipUniquenessRequest("nc1", "10.0.0.1", "", map[string]string{"ip1": "10.0.0.3"})
	updatedFirst.Version = "3"
	regressedSecond := second
	regressedSecond.Version = "1"

	responseCode := service.ApplyNNCGoal(NNCGoal{
		NetworkContainerIDs: []string{"nc1", "nc2"},
		NetworkContainers: []NNCNetworkContainerGoal{
			{ValidateVersion: true, Request: updatedFirst},
			{ValidateVersion: true, Request: regressedSecond},
		},
	})

	assert.Equal(t, types.UnsupportedNCVersion, responseCode)
	assert.Equal(t, first, service.state.ContainerStatus["nc1"].CreateNetworkContainerRequest)
	assert.Equal(t, second, service.state.ContainerStatus["nc2"].CreateNetworkContainerRequest)
	assert.Equal(t, firstIP, service.PodIPConfigState["ip1"])
	assert.Equal(t, secondIP, service.PodIPConfigState["ip2"])
}

func TestApplyNNCGoalAppliesStaticNetworkContainer(t *testing.T) {
	existing := ipUniquenessRequest("nc1", "10.0.0.1", "", map[string]string{"ip1": "10.0.0.2"})
	existing.Version = "2"
	available := cns.IPConfigurationStatus{ID: "ip1", NCID: existing.NetworkContainerid, IPAddress: "10.0.0.2"}
	available.SetState(types.Available)
	service := &HTTPRestService{
		state: &httpRestServiceState{
			OrchestratorType: cns.KubernetesCRD,
			ContainerStatus: map[string]containerstatus{
				existing.NetworkContainerid: {
					ID:                            existing.NetworkContainerid,
					HostVersion:                   "2",
					CreateNetworkContainerRequest: existing,
				},
			},
		},
		PodIPConfigState: map[string]cns.IPConfigurationStatus{"ip1": available},
	}
	updated := ipUniquenessRequest("nc1", "10.0.0.1", "", map[string]string{"ip1": "10.0.0.3"})
	updated.Version = "1"

	responseCode := service.ApplyNNCGoal(NNCGoal{
		NetworkContainerIDs: []string{"nc1"},
		NetworkContainers:   []NNCNetworkContainerGoal{{ValidateVersion: false, Request: updated}},
	})

	require.Equal(t, types.Success, responseCode)
	assert.Equal(t, updated, service.state.ContainerStatus["nc1"].CreateNetworkContainerRequest)
	assert.Equal(t, "10.0.0.3", service.PodIPConfigState["ip1"].IPAddress)
}

func TestApplyNNCGoalSwapsAddressesAcrossNetworkContainers(t *testing.T) {
	first := ipUniquenessRequest("nc1", "10.0.0.1", "", map[string]string{"ip1": "10.0.0.2"})
	second := ipUniquenessRequest("nc2", "10.0.0.9", "", map[string]string{"ip2": "10.0.0.10"})
	firstIP := cns.IPConfigurationStatus{ID: "ip1", NCID: first.NetworkContainerid, IPAddress: "10.0.0.2"}
	firstIP.SetState(types.Available)
	secondIP := cns.IPConfigurationStatus{ID: "ip2", NCID: second.NetworkContainerid, IPAddress: "10.0.0.10"}
	secondIP.SetState(types.Available)
	service := &HTTPRestService{
		state: &httpRestServiceState{
			OrchestratorType: cns.KubernetesCRD,
			ContainerStatus: map[string]containerstatus{
				first.NetworkContainerid: {
					ID:                            first.NetworkContainerid,
					HostVersion:                   "1",
					CreateNetworkContainerRequest: first,
				},
				second.NetworkContainerid: {
					ID:                            second.NetworkContainerid,
					HostVersion:                   "1",
					CreateNetworkContainerRequest: second,
				},
			},
		},
		PodIPConfigState: map[string]cns.IPConfigurationStatus{"ip1": firstIP, "ip2": secondIP},
	}
	updatedFirst := ipUniquenessRequest("nc1", "10.0.0.1", "", map[string]string{"ip1": "10.0.0.10"})
	updatedSecond := ipUniquenessRequest("nc2", "10.0.0.9", "", map[string]string{"ip2": "10.0.0.2"})

	responseCode := service.ApplyNNCGoal(NNCGoal{
		NetworkContainerIDs: []string{"nc1", "nc2"},
		NetworkContainers: []NNCNetworkContainerGoal{
			{ValidateVersion: true, Request: updatedFirst},
			{ValidateVersion: true, Request: updatedSecond},
		},
	})

	require.Equal(t, types.Success, responseCode)
	assert.Equal(t, "10.0.0.10", service.PodIPConfigState["ip1"].IPAddress)
	assert.Equal(t, "10.0.0.2", service.PodIPConfigState["ip2"].IPAddress)
}

func TestApplyNNCGoalRetainsFilteredNetworkContainer(t *testing.T) {
	filtered := ipUniquenessRequest("filtered", "10.0.0.1", "", map[string]string{"ip1": "10.0.0.2"})
	service := &HTTPRestService{
		state: &httpRestServiceState{
			OrchestratorType: cns.KubernetesCRD,
			ContainerStatus: map[string]containerstatus{
				filtered.NetworkContainerid: {
					ID:                            filtered.NetworkContainerid,
					HostVersion:                   "1",
					CreateNetworkContainerRequest: filtered,
				},
			},
		},
		PodIPConfigState: map[string]cns.IPConfigurationStatus{},
	}

	responseCode := service.ApplyNNCGoal(NNCGoal{NetworkContainerIDs: []string{filtered.NetworkContainerid}})

	assert.Equal(t, types.Success, responseCode)
	assert.Contains(t, service.state.ContainerStatus, filtered.NetworkContainerid)
}

func TestApplyNNCGoalRejectsStaleNetworkContainerWithAssignedIP(t *testing.T) {
	stale := ipUniquenessRequest("stale", "10.0.0.1", "", map[string]string{"ip1": "10.0.0.2"})
	assigned := cns.IPConfigurationStatus{ID: "ip1", NCID: stale.NetworkContainerid, IPAddress: "10.0.0.2"}
	assigned.SetState(types.Assigned)
	service := &HTTPRestService{
		state: &httpRestServiceState{
			OrchestratorType: cns.KubernetesCRD,
			ContainerStatus: map[string]containerstatus{
				stale.NetworkContainerid: {
					ID:                            stale.NetworkContainerid,
					HostVersion:                   "1",
					CreateNetworkContainerRequest: stale,
				},
			},
		},
		PodIPConfigState: map[string]cns.IPConfigurationStatus{"ip1": assigned},
	}

	responseCode := service.ApplyNNCGoal(NNCGoal{NetworkContainerIDs: []string{}})

	assert.Equal(t, types.InconsistentIPConfigState, responseCode)
	assert.Contains(t, service.state.ContainerStatus, stale.NetworkContainerid)
	assert.Equal(t, assigned, service.PodIPConfigState["ip1"])
}

func TestApplyNNCGoalMovesIPIDFromRemovedNetworkContainer(t *testing.T) {
	stale := ipUniquenessRequest("stale", "10.0.0.1", "", map[string]string{"10.0.0.2": "10.0.0.2"})
	available := cns.IPConfigurationStatus{ID: "10.0.0.2", NCID: stale.NetworkContainerid, IPAddress: "10.0.0.2"}
	available.SetState(types.Available)
	service := &HTTPRestService{
		state: &httpRestServiceState{
			OrchestratorType: cns.KubernetesCRD,
			ContainerStatus: map[string]containerstatus{
				stale.NetworkContainerid: {
					ID:                            stale.NetworkContainerid,
					HostVersion:                   "1",
					CreateNetworkContainerRequest: stale,
				},
			},
		},
		PodIPConfigState: map[string]cns.IPConfigurationStatus{"10.0.0.2": available},
	}
	replacement := ipUniquenessRequest("replacement", "10.0.0.9", "", map[string]string{"10.0.0.2": "10.0.0.2"})

	responseCode := service.ApplyNNCGoal(NNCGoal{
		NetworkContainerIDs: []string{replacement.NetworkContainerid},
		NetworkContainers:   []NNCNetworkContainerGoal{{ValidateVersion: false, Request: replacement}},
	})

	require.Equal(t, types.Success, responseCode)
	assert.NotContains(t, service.state.ContainerStatus, stale.NetworkContainerid)
	assert.Equal(t, replacement, service.state.ContainerStatus[replacement.NetworkContainerid].CreateNetworkContainerRequest)
	assert.Equal(t, replacement.NetworkContainerid, service.PodIPConfigState["10.0.0.2"].NCID)
}

func TestApplyNNCGoalDoesNotPublishFailedPersistence(t *testing.T) {
	existing := ipUniquenessRequest("nc1", "10.0.0.1", "", map[string]string{"ip1": "10.0.0.2"})
	available := cns.IPConfigurationStatus{ID: "ip1", NCID: existing.NetworkContainerid, IPAddress: "10.0.0.2"}
	available.SetState(types.Available)
	service := &HTTPRestService{
		Service: &cns.Service{Service: &common.Service{Options: map[string]interface{}{}}},
		store: failingWriteStore{
			KeyValueStore: store.NewMockStore(""),
			err:           errForcedStateWrite,
		},
		state: &httpRestServiceState{
			OrchestratorType: cns.KubernetesCRD,
			ContainerStatus: map[string]containerstatus{
				existing.NetworkContainerid: {
					ID:                            existing.NetworkContainerid,
					HostVersion:                   "1",
					CreateNetworkContainerRequest: existing,
				},
			},
		},
		PodIPConfigState: map[string]cns.IPConfigurationStatus{"ip1": available},
	}
	updated := ipUniquenessRequest("nc1", "10.0.0.1", "", map[string]string{"ip1": "10.0.0.3"})
	updated.Version = "2"

	responseCode := service.ApplyNNCGoal(NNCGoal{
		NetworkContainerIDs: []string{"nc1"},
		NetworkContainers:   []NNCNetworkContainerGoal{{ValidateVersion: true, Request: updated}},
	})

	assert.Equal(t, types.UnexpectedError, responseCode)
	assert.Equal(t, existing, service.state.ContainerStatus["nc1"].CreateNetworkContainerRequest)
	assert.Equal(t, available, service.PodIPConfigState["ip1"])
}

func TestApplyNNCGoalPublishesReplacementAndStaleRemovalTogether(t *testing.T) {
	stale := ipUniquenessRequest("stale", "10.0.0.1", "", map[string]string{"stale-ip": "10.0.0.2"})
	existing := ipUniquenessRequest("nc1", "10.0.0.9", "", map[string]string{"ip1": "10.0.0.10"})
	service := &HTTPRestService{
		Service: &cns.Service{Service: &common.Service{Options: map[string]interface{}{}}},
		store:   store.NewMockStore(""),
		state: &httpRestServiceState{
			OrchestratorType: cns.KubernetesCRD,
			ContainerStatus: map[string]containerstatus{
				stale.NetworkContainerid: {
					ID:                            stale.NetworkContainerid,
					HostVersion:                   "1",
					CreateNetworkContainerRequest: stale,
				},
				existing.NetworkContainerid: {
					ID:                            existing.NetworkContainerid,
					HostVersion:                   "1",
					CreateNetworkContainerRequest: existing,
				},
			},
		},
		PodIPConfigState: map[string]cns.IPConfigurationStatus{},
	}
	updated := ipUniquenessRequest("nc1", "10.0.0.9", "", map[string]string{"ip1": "10.0.0.11"})
	updated.Version = "2"

	responseCode := service.ApplyNNCGoal(NNCGoal{
		NetworkContainerIDs: []string{"nc1"},
		NetworkContainers:   []NNCNetworkContainerGoal{{ValidateVersion: true, Request: updated}},
	})

	require.Equal(t, types.Success, responseCode)
	assert.NotContains(t, service.state.ContainerStatus, stale.NetworkContainerid)
	assert.Equal(t, updated, service.state.ContainerStatus["nc1"].CreateNetworkContainerRequest)

	var persisted httpRestServiceState
	require.NoError(t, service.store.Read(storeKey, &persisted))
	assert.NotContains(t, persisted.ContainerStatus, stale.NetworkContainerid)
	assert.Equal(t, updated.Version, persisted.ContainerStatus["nc1"].CreateNetworkContainerRequest.Version)
	assert.Equal(t, updated.SecondaryIPConfigs, persisted.ContainerStatus["nc1"].CreateNetworkContainerRequest.SecondaryIPConfigs)
}
