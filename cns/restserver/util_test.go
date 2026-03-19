package restserver

import (
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/common"
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
