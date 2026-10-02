package restserver

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/common"
	"github.com/Azure/azure-container-networking/cns/types"
	acn "github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const expiredEndpointID = "expired"

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

func TestRestoreStateReplaysAndPrunesEndpointDeleteIntents(t *testing.T) {
	mainStore := store.NewMockStore("")
	endpointStore := store.NewMockStore("")
	legacyContainerID := "12345678-1234-1234-1234-123456789abc"
	legacyEndpointID := "12345678-eth0"
	require.NoError(t, endpointStore.Write(EndpointStoreKey, map[string]*EndpointInfo{
		"current":         {PodName: "current-pod"},
		expiredEndpointID: {PodName: "expired-pod"},
		"zero":            {PodName: "zero-pod"},
		legacyEndpointID:  {PodName: "legacy-pod"},
		"untouched":       {PodName: "untouched-pod"},
	}))
	require.NoError(t, endpointStore.Write(EndpointDeleteIntentStoreKey, map[string]EndpointDeleteIntent{
		expiredEndpointID: {CreatedAt: time.Now().Add(-endpointDeleteIntentTTL - time.Minute)},
		"current":         {CreatedAt: time.Now()},
		"zero":            {},
		legacyContainerID: {CreatedAt: time.Now().Add(-endpointDeleteIntentTTL - time.Minute)},
	}))

	svc := HTTPRestService{
		Service: &cns.Service{
			Service: &common.Service{Options: map[string]interface{}{acn.OptManageEndpointState: true}},
		},
		store:                 mainStore,
		state:                 &httpRestServiceState{},
		EndpointStateStore:    endpointStore,
		EndpointState:         make(map[string]*EndpointInfo),
		EndpointDeleteIntents: make(map[string]EndpointDeleteIntent),
	}

	require.NoError(t, svc.restoreState())

	require.NotContains(t, svc.EndpointState, "current")
	require.NotContains(t, svc.EndpointState, expiredEndpointID)
	require.NotContains(t, svc.EndpointState, "zero")
	require.NotContains(t, svc.EndpointState, legacyEndpointID)
	require.Contains(t, svc.EndpointState, "untouched")
	require.Contains(t, svc.EndpointDeleteIntents, "current")
	require.NotContains(t, svc.EndpointDeleteIntents, expiredEndpointID)
	require.NotContains(t, svc.EndpointDeleteIntents, "zero")
	require.NotContains(t, svc.EndpointDeleteIntents, legacyContainerID)

	var storedEndpoints map[string]*EndpointInfo
	require.NoError(t, endpointStore.Read(EndpointStoreKey, &storedEndpoints))
	require.NotContains(t, storedEndpoints, "current")
	require.NotContains(t, storedEndpoints, expiredEndpointID)
	require.NotContains(t, storedEndpoints, "zero")
	require.NotContains(t, storedEndpoints, legacyEndpointID)
	require.Contains(t, storedEndpoints, "untouched")
}

func TestRestoreStateRetainsDeleteIntentWhenEndpointDeletionCannotBePersisted(t *testing.T) {
	mainStore := store.NewMockStore("")
	endpointStore := store.NewMockStore("")
	require.NoError(t, endpointStore.Write(EndpointStoreKey, map[string]*EndpointInfo{
		expiredEndpointID: {PodName: "expired-pod"},
	}))
	require.NoError(t, endpointStore.Write(EndpointDeleteIntentStoreKey, map[string]EndpointDeleteIntent{
		expiredEndpointID: {CreatedAt: time.Now().Add(-endpointDeleteIntentTTL - time.Minute)},
	}))

	svc := HTTPRestService{
		Service: &cns.Service{
			Service: &common.Service{Options: map[string]interface{}{acn.OptManageEndpointState: true}},
		},
		store:                 mainStore,
		state:                 &httpRestServiceState{},
		EndpointStateStore:    endpointWriteFailStore{KeyValueStore: endpointStore, err: errForcedEndpointStateWrite},
		EndpointState:         make(map[string]*EndpointInfo),
		EndpointDeleteIntents: make(map[string]EndpointDeleteIntent),
	}

	require.ErrorIs(t, svc.restoreState(), errForcedEndpointStateWrite)
	require.Contains(t, svc.EndpointState, expiredEndpointID)
	require.Contains(t, svc.EndpointDeleteIntents, expiredEndpointID)

	var storedEndpoints map[string]*EndpointInfo
	require.NoError(t, endpointStore.Read(EndpointStoreKey, &storedEndpoints))
	require.Contains(t, storedEndpoints, expiredEndpointID)
	var storedIntents map[string]EndpointDeleteIntent
	require.NoError(t, endpointStore.Read(EndpointDeleteIntentStoreKey, &storedIntents))
	require.Contains(t, storedIntents, expiredEndpointID)
}

func TestRestoreStateToleratesMissingEndpointDeleteIntents(t *testing.T) {
	mainStore := store.NewMockStore("")
	endpointStore := store.NewMockStore("")
	require.NoError(t, endpointStore.Write(EndpointStoreKey, map[string]*EndpointInfo{
		"container1": {PodName: "pod1"},
	}))

	svc := HTTPRestService{
		Service: &cns.Service{
			Service: &common.Service{Options: map[string]interface{}{acn.OptManageEndpointState: true}},
		},
		store:                 mainStore,
		state:                 &httpRestServiceState{},
		EndpointStateStore:    endpointStore,
		EndpointState:         make(map[string]*EndpointInfo),
		EndpointDeleteIntents: make(map[string]EndpointDeleteIntent),
	}

	require.NoError(t, svc.restoreState())
	require.Contains(t, svc.EndpointState, "container1")
	require.Empty(t, svc.EndpointDeleteIntents)
}

func TestRestoreStateFailsClosedWhenDeleteIntentsCannotBeRead(t *testing.T) {
	mainStore := store.NewMockStore("")
	endpointStore := store.NewMockStore("")
	require.NoError(t, endpointStore.Write(EndpointStoreKey, map[string]*EndpointInfo{}))
	svc := HTTPRestService{
		Service: &cns.Service{
			Service: &common.Service{Options: map[string]interface{}{acn.OptManageEndpointState: true}},
		},
		store: mainStore,
		state: &httpRestServiceState{},
		EndpointStateStore: keyReadFailStore{
			KeyValueStore: endpointStore,
			failKey:       EndpointDeleteIntentStoreKey,
			err:           errForcedDeleteIntentRead,
		},
		EndpointState: make(map[string]*EndpointInfo),
	}

	require.ErrorIs(t, svc.restoreState(), errForcedDeleteIntentRead)
}

var (
	errForcedEndpointStateRead = errors.New("forced endpoint state read failure")
	errForcedDeleteIntentRead  = errors.New("forced endpoint delete intent read failure")
)

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

func TestValidateUniqueSecondaryIPs(t *testing.T) {
	const id1, id2, id3 = "id1", "id2", "id3"
	ip := func(address string) cns.IPConfigurationStatus {
		return cns.IPConfigurationStatus{IPAddress: address}
	}
	secondary := func(address string) cns.SecondaryIPConfig {
		return cns.SecondaryIPConfig{IPAddress: address}
	}
	tests := []struct {
		name     string
		current  map[string]cns.IPConfigurationStatus
		existing map[string]cns.SecondaryIPConfig
		incoming map[string]cns.SecondaryIPConfig
		want     types.ResponseCode
	}{
		{
			name:     "unique IPs",
			current:  map[string]cns.IPConfigurationStatus{id2: ip("10.0.0.2")},
			incoming: map[string]cns.SecondaryIPConfig{id1: secondary("10.0.0.1")},
			want:     types.Success,
		},
		{
			name:     "duplicate IPs in the goal",
			incoming: map[string]cns.SecondaryIPConfig{id1: secondary("10.0.0.1"), id2: secondary("10.0.0.1")},
			want:     types.InconsistentIPConfigState,
		},
		{
			name:     "IP used by another IP ID in the pool",
			current:  map[string]cns.IPConfigurationStatus{id2: ip("10.0.0.1")},
			incoming: map[string]cns.SecondaryIPConfig{id1: secondary("10.0.0.1")},
			want:     types.InconsistentIPConfigState,
		},
		{
			name:     "IP ID changes IP",
			current:  map[string]cns.IPConfigurationStatus{id1: ip("10.0.0.1")},
			existing: map[string]cns.SecondaryIPConfig{id1: secondary("10.0.0.1")},
			incoming: map[string]cns.SecondaryIPConfig{id1: secondary("10.0.0.2")},
			want:     types.InconsistentIPConfigState,
		},
		{
			name:     "unchanged IP ID",
			current:  map[string]cns.IPConfigurationStatus{id1: ip("10.0.0.1")},
			existing: map[string]cns.SecondaryIPConfig{id1: secondary("10.0.0.1")},
			incoming: map[string]cns.SecondaryIPConfig{id1: secondary("10.0.0.1")},
			want:     types.Success,
		},
		{
			name:     "IP reused after the goal deletes its IP ID",
			current:  map[string]cns.IPConfigurationStatus{id1: ip("10.0.0.1")},
			existing: map[string]cns.SecondaryIPConfig{id1: secondary("10.0.0.1")},
			incoming: map[string]cns.SecondaryIPConfig{id2: secondary("10.0.0.1")},
			want:     types.Success,
		},
		{
			name:     "conflict only in the current pool",
			current:  map[string]cns.IPConfigurationStatus{id2: ip("10.0.0.1"), id3: ip("10.0.0.1")},
			incoming: map[string]cns.SecondaryIPConfig{id1: secondary("10.0.0.5")},
			want:     types.Success,
		},
		{
			name:     "unchanged IP ID with a conflict only in the current pool",
			current:  map[string]cns.IPConfigurationStatus{id1: ip("10.0.0.1"), id2: ip("10.0.0.1")},
			existing: map[string]cns.SecondaryIPConfig{id1: secondary("10.0.0.1")},
			incoming: map[string]cns.SecondaryIPConfig{id1: secondary("10.0.0.1")},
			want:     types.Success,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, message := validateUniqueSecondaryIPs(tt.current, tt.existing, tt.incoming)
			assert.Equal(t, tt.want, got, message)
		})
	}
}

func TestSaveNetworkContainerGoalStateSecondaryIPUniqueness(t *testing.T) {
	const nc1, nc2, id1, id2 = "nc1", "nc2", "id1", "id2"
	svc := getTestService(cns.KubernetesCRD)
	req1 := generateNetworkContainerRequest(map[string]cns.SecondaryIPConfig{id1: newSecondaryIPConfig("10.0.0.1", -1)}, nc1, "-1")
	require.Equal(t, types.Success, svc.CreateOrUpdateNetworkContainerInternal(req1))

	req2 := generateNetworkContainerRequest(map[string]cns.SecondaryIPConfig{id2: newSecondaryIPConfig("10.0.0.1", -1)}, nc2, "-1")
	require.Equal(t, types.InconsistentIPConfigState, svc.CreateOrUpdateNetworkContainerInternal(req2))
	assert.NotContains(t, svc.state.ContainerStatus, nc2)
	assert.NotContains(t, svc.PodIPConfigState, id2)

	svc.MustEnsureNoStaleNCs([]string{nc2})
	require.Equal(t, types.Success, svc.CreateOrUpdateNetworkContainerInternal(req2))
	assert.Equal(t, nc2, svc.PodIPConfigState[id2].NCID)
	assert.NotContains(t, svc.PodIPConfigState, id1)
}

func TestCreateOrUpdateNetworkContainersInternalIsAtomic(t *testing.T) {
	const nc1, nc2, id1, id2 = "nc1", "nc2", "id1", "id2"
	svc := getTestService(cns.KubernetesCRD)
	require.Equal(t, types.Success, svc.CreateOrUpdateNetworkContainerInternal(generateNetworkContainerRequest(map[string]cns.SecondaryIPConfig{id1: newSecondaryIPConfig("10.0.0.1", 2)}, nc1, "2")))
	require.Equal(t, types.Success, svc.CreateOrUpdateNetworkContainerInternal(generateNetworkContainerRequest(nil, nc2, "2")))

	// The second NC version decreases, so the first NC update must not remain.
	goals := []NetworkContainerGoal{
		{Request: generateNetworkContainerRequest(map[string]cns.SecondaryIPConfig{id1: newSecondaryIPConfig("10.0.0.1", 2), id2: newSecondaryIPConfig("10.0.0.2", 3)}, nc1, "3"), ValidateVersion: true},
		{Request: generateNetworkContainerRequest(nil, nc2, "1"), ValidateVersion: true},
	}
	require.Equal(t, types.UnsupportedNCVersion, svc.CreateOrUpdateNetworkContainersInternal(goals))
	assert.Equal(t, "2", svc.state.ContainerStatus[nc1].CreateNetworkContainerRequest.Version)
	assert.NotContains(t, svc.PodIPConfigState, id2)

	// A goal without NCs does not persist the state.
	svc.store = store.NewMockStore("")
	timeStamp := svc.state.TimeStamp
	require.Equal(t, types.Success, svc.CreateOrUpdateNetworkContainersInternal(nil))
	assert.Equal(t, timeStamp, svc.state.TimeStamp)
}

func TestCreateOrUpdateNetworkContainersInternalMovesIPBetweenNCs(t *testing.T) {
	const nc1, nc2, id1, id2 = "nc1", "nc2", "id1", "id2"
	svc := getTestService(cns.KubernetesCRD)
	require.Equal(t, types.Success, svc.CreateOrUpdateNetworkContainerInternal(generateNetworkContainerRequest(map[string]cns.SecondaryIPConfig{id1: newSecondaryIPConfig("10.0.0.1", -1)}, nc1, "-1")))
	require.Equal(t, types.Success, svc.CreateOrUpdateNetworkContainerInternal(generateNetworkContainerRequest(nil, nc2, "-1")))

	// NC2 is first in the goal and takes the IP that NC1 releases in the same goal.
	goals := []NetworkContainerGoal{
		{Request: generateNetworkContainerRequest(map[string]cns.SecondaryIPConfig{id2: newSecondaryIPConfig("10.0.0.1", -1)}, nc2, "-1")},
		{Request: generateNetworkContainerRequest(nil, nc1, "-1")},
	}
	require.Equal(t, types.Success, svc.CreateOrUpdateNetworkContainersInternal(goals))
	assert.Equal(t, nc2, svc.PodIPConfigState[id2].NCID)
	assert.NotContains(t, svc.PodIPConfigState, id1)

	// A goal that gives the IP to both NCs is rejected.
	goals = []NetworkContainerGoal{
		{Request: generateNetworkContainerRequest(map[string]cns.SecondaryIPConfig{id1: newSecondaryIPConfig("10.0.0.1", -1)}, nc1, "-1")},
		{Request: generateNetworkContainerRequest(map[string]cns.SecondaryIPConfig{id2: newSecondaryIPConfig("10.0.0.1", -1)}, nc2, "-1")},
	}
	require.Equal(t, types.InconsistentIPConfigState, svc.CreateOrUpdateNetworkContainersInternal(goals))
	assert.NotContains(t, svc.PodIPConfigState, id1)

	// A goal that lists one IP ID in two NCs is rejected, so the IP of the first listing cannot skip validation.
	goals = []NetworkContainerGoal{
		{Request: generateNetworkContainerRequest(map[string]cns.SecondaryIPConfig{id1: newSecondaryIPConfig("10.0.0.1", -1)}, nc1, "-1")},
		{Request: generateNetworkContainerRequest(map[string]cns.SecondaryIPConfig{id2: newSecondaryIPConfig("10.0.0.1", -1), id1: newSecondaryIPConfig("10.0.0.2", -1)}, nc2, "-1")},
	}
	require.Equal(t, types.InconsistentIPConfigState, svc.CreateOrUpdateNetworkContainersInternal(goals))
	assert.NotContains(t, svc.PodIPConfigState, id1)
}
