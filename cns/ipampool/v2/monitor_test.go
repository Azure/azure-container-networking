package v2

import (
	"context"
	"math/rand"
	"net/netip"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/exp/maps"
)

type ipStateStoreMock struct {
	podIPConfigState map[string]cns.IPConfigurationStatus
	markCalls        []int
	err              error
}

func (m *ipStateStoreMock) GetPendingReleaseIPConfigs() []cns.IPConfigurationStatus {
	return maps.Values(m.pendingReleaseIPConfigMap())
}

func (m *ipStateStoreMock) GetPodIPConfigState() map[string]cns.IPConfigurationStatus {
	state := make(map[string]cns.IPConfigurationStatus, len(m.podIPConfigState))
	maps.Copy(state, m.podIPConfigState)
	return state
}

func (m *ipStateStoreMock) MarkNIPsPendingRelease(n int) (map[string]cns.IPConfigurationStatus, error) {
	m.markCalls = append(m.markCalls, n)
	if n <= 0 {
		return m.pendingReleaseIPConfigMap(), nil
	}
	if m.err != nil {
		return nil, m.err
	}
	releasable := make([]string, 0, n)
	for id, ipConfig := range m.podIPConfigState {
		if ipConfig.GetState() == types.PendingProgramming {
			releasable = append(releasable, id)
			if len(releasable) == n {
				break
			}
		}
	}
	for id, ipConfig := range m.podIPConfigState {
		if len(releasable) == n {
			break
		}
		if ipConfig.GetState() == types.Available {
			releasable = append(releasable, id)
		}
	}
	if len(releasable) < n {
		return nil, errors.New("unable to release requested number of IPs")
	}
	for _, id := range releasable {
		ipConfig := m.podIPConfigState[id]
		ipConfig.SetState(types.PendingRelease)
		m.podIPConfigState[id] = ipConfig
	}
	return m.pendingReleaseIPConfigMap(), nil
}

func (m *ipStateStoreMock) pendingReleaseIPConfigMap() map[string]cns.IPConfigurationStatus {
	pendingRelease := make(map[string]cns.IPConfigurationStatus)
	for id, ipConfig := range m.podIPConfigState {
		if ipConfig.GetState() == types.PendingRelease {
			pendingRelease[id] = ipConfig
		}
	}
	return pendingRelease
}

func ipConfigStateGenerator(n int, state types.IPState) map[string]cns.IPConfigurationStatus {
	m := make(map[string]cns.IPConfigurationStatus, n)
	ip := netip.MustParseAddr("10.0.0.0")
	for i := 0; i < n; i++ {
		id := uuid.New().String()
		ip = ip.Next()
		status := cns.IPConfigurationStatus{
			ID:        id,
			IPAddress: ip.String(),
		}
		status.SetState(state)
		m[id] = status
	}
	return m
}

// pendingReleaseGenerator generates a variable number of random pendingRelease IPConfigs.
func pendingReleaseGenerator(n int) map[string]cns.IPConfigurationStatus {
	return ipConfigStateGenerator(n, types.PendingRelease)
}

func mergeIPConfigStates(groups ...map[string]cns.IPConfigurationStatus) map[string]cns.IPConfigurationStatus {
	merged := make(map[string]cns.IPConfigurationStatus)
	for _, group := range groups {
		maps.Copy(merged, group)
	}
	return merged
}

func ipStateStoreWithCounts(pendingRelease, available int) ipStateStoreMock {
	return ipStateStoreMock{
		podIPConfigState: mergeIPConfigStates(
			ipConfigStateGenerator(pendingRelease, types.PendingRelease),
			ipConfigStateGenerator(available, types.Available),
		),
	}
}

func TestPendingReleaseIPConfigsGenerator(t *testing.T) {
	t.Parallel()
	n := rand.Intn(100) //nolint:gosec // test
	m := pendingReleaseGenerator(n)
	assert.Len(t, m, n, "pendingReleaseGenerator made the wrong quantity")
	for k, v := range m {
		_, err := uuid.Parse(v.ID)
		require.NoError(t, err, "pendingReleaseGenerator made a bad UUID")
		assert.Equal(t, k, v.ID, "pendingReleaseGenerator stored using the wrong key ")
		_, err = netip.ParseAddr(v.IPAddress)
		require.NoError(t, err, "pendingReleaseGenerator made a bad IP")
		assert.Equal(t, types.PendingRelease, v.GetState(), "pendingReleaseGenerator set the wrong State")
	}
}

func TestBuildNNCSpec(t *testing.T) {
	tests := []struct {
		name                    string
		pendingReleaseIPConfigs map[string]cns.IPConfigurationStatus
		request                 int64
	}{
		{
			name:    "without no pending release",
			request: 16,
		},
		{
			name:                    "with pending release",
			pendingReleaseIPConfigs: pendingReleaseGenerator(16),
			request:                 16,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pm := &Monitor{
				store: &ipStateStoreMock{
					podIPConfigState: tt.pendingReleaseIPConfigs,
				},
			}
			spec := pm.buildNNCSpec(tt.request)
			assert.Equal(t, tt.request, spec.RequestedIPCount)
			assert.Equal(t, len(tt.pendingReleaseIPConfigs), len(spec.IPsNotInUse))
			assert.ElementsMatch(t, maps.Keys(tt.pendingReleaseIPConfigs), spec.IPsNotInUse)
		})
	}
}

type nncClientMock struct {
	req          v1alpha.NodeNetworkConfigSpec
	patchSpecs   []v1alpha.NodeNetworkConfigSpec
	failPatchCnt int
	err          error
}

func (m *nncClientMock) PatchSpec(_ context.Context, spec *v1alpha.NodeNetworkConfigSpec, _ string) (*v1alpha.NodeNetworkConfig, error) {
	m.patchSpecs = append(m.patchSpecs, *spec)
	if m.failPatchCnt > 0 {
		m.failPatchCnt--
		return nil, m.err
	}
	if m.err != nil {
		return nil, m.err
	}
	m.req = *spec
	return nil, nil
}

func TestReconcile(t *testing.T) {
	tests := []struct {
		name                 string
		demand               int64
		request              int64
		scaler               scaler
		nnccli               nncClientMock
		store                ipStateStoreMock
		wantRequest          int64
		wantPendingRelease   int
		wantPatchCalls       int
		wantPatchRequest     int64
		wantMarkReleaseCalls []int
		wantErr              bool
	}{
		{
			name:    "no delta",
			demand:  5,
			request: 16,
			scaler: scaler{
				batch:  16,
				buffer: .5,
				max:    250,
			},
			nnccli: nncClientMock{
				req: v1alpha.NodeNetworkConfigSpec{
					RequestedIPCount: 16,
				},
			},
			store:       ipStateStoreMock{},
			wantRequest: 16,
		},
		{
			name:    "fail to release",
			demand:  6,
			request: 32,
			scaler: scaler{
				batch:  16,
				buffer: .5,
				max:    250,
			},
			nnccli: nncClientMock{
				req: v1alpha.NodeNetworkConfigSpec{
					RequestedIPCount: 32,
				},
			},
			store: ipStateStoreMock{
				podIPConfigState: ipStateStoreWithCounts(0, 32).podIPConfigState,
				err:              errors.Errorf("failed to mark IPs pending release"),
			},
			wantRequest:          32,
			wantMarkReleaseCalls: []int{16},
			wantErr:              true,
		},
		{
			name:    "fail to patch",
			demand:  20,
			request: 16,
			scaler: scaler{
				batch:  16,
				buffer: .5,
				max:    250,
			},
			nnccli: nncClientMock{
				req: v1alpha.NodeNetworkConfigSpec{
					RequestedIPCount: 16,
				},
				err: errors.Errorf("failed to patch NNC Spec"),
			},
			store:            ipStateStoreMock{},
			wantRequest:      16,
			wantPatchCalls:   1,
			wantPatchRequest: 32,
			wantErr:          true,
		},
		{
			name:    "single scale up",
			demand:  15,
			request: 16,
			scaler: scaler{
				batch:  16,
				buffer: .5,
				max:    250,
			},
			nnccli:           nncClientMock{},
			store:            ipStateStoreMock{},
			wantRequest:      32,
			wantPatchCalls:   1,
			wantPatchRequest: 32,
		},
		{
			name:    "big scale up",
			demand:  75,
			request: 16,
			scaler: scaler{
				batch:  16,
				buffer: .5,
				max:    250,
			},
			nnccli:           nncClientMock{},
			store:            ipStateStoreMock{},
			wantRequest:      96,
			wantPatchCalls:   1,
			wantPatchRequest: 96,
		},
		{
			name:    "capped scale up",
			demand:  300,
			request: 16,
			scaler: scaler{
				batch:  16,
				buffer: .5,
				max:    250,
			},
			nnccli:           nncClientMock{},
			store:            ipStateStoreMock{},
			wantRequest:      250,
			wantPatchCalls:   1,
			wantPatchRequest: 250,
		},
		{
			name:    "single scale down",
			demand:  5,
			request: 32,
			scaler: scaler{
				batch:  16,
				buffer: .5,
				max:    250,
			},
			nnccli:               nncClientMock{},
			store:                ipStateStoreWithCounts(0, 32),
			wantRequest:          16,
			wantPendingRelease:   16,
			wantPatchCalls:       1,
			wantPatchRequest:     16,
			wantMarkReleaseCalls: []int{16},
		},
		{
			name:    "big scale down",
			demand:  5,
			request: 128,
			scaler: scaler{
				batch:  16,
				buffer: .5,
				max:    250,
			},
			nnccli:               nncClientMock{},
			store:                ipStateStoreWithCounts(0, 128),
			wantRequest:          16,
			wantPendingRelease:   112,
			wantPatchCalls:       1,
			wantPatchRequest:     16,
			wantMarkReleaseCalls: []int{112},
		},
		{
			name:    "capped scale down",
			demand:  0,
			request: 32,
			scaler: scaler{
				batch:  16,
				buffer: .5,
				max:    250,
			},
			nnccli:               nncClientMock{},
			store:                ipStateStoreWithCounts(0, 32),
			wantRequest:          16,
			wantPendingRelease:   16,
			wantPatchCalls:       1,
			wantPatchRequest:     16,
			wantMarkReleaseCalls: []int{16},
		},
		{
			name:    "scale up unskew",
			demand:  15,
			request: 3,
			scaler: scaler{
				batch:  16,
				buffer: .5,
				max:    250,
			},
			nnccli:           nncClientMock{},
			store:            ipStateStoreMock{},
			wantRequest:      32,
			wantPatchCalls:   1,
			wantPatchRequest: 32,
		},
		{
			name:    "scale down unskew",
			demand:  5,
			request: 37,
			scaler: scaler{
				batch:  16,
				buffer: .5,
				max:    250,
			},
			nnccli:               nncClientMock{},
			store:                ipStateStoreWithCounts(0, 37),
			wantRequest:          16,
			wantPendingRelease:   21,
			wantPatchCalls:       1,
			wantPatchRequest:     16,
			wantMarkReleaseCalls: []int{21},
		},
		{
			name:    "single scale up with pending release",
			demand:  20,
			request: 16,
			scaler: scaler{
				batch:  16,
				buffer: .5,
				max:    250,
			},
			nnccli:             nncClientMock{},
			store:              ipStateStoreWithCounts(16, 16),
			wantRequest:        32,
			wantPendingRelease: 16,
			wantPatchCalls:     1,
			wantPatchRequest:   32,
		},
		{
			name:    "single scale down with pending release",
			demand:  5,
			request: 32,
			scaler: scaler{
				batch:  16,
				buffer: .5,
				max:    250,
			},
			nnccli:               nncClientMock{},
			store:                ipStateStoreWithCounts(16, 32),
			wantRequest:          16,
			wantPendingRelease:   32,
			wantPatchCalls:       1,
			wantPatchRequest:     16,
			wantMarkReleaseCalls: []int{16},
		},
		{
			name:    "scale down with stale request and existing pending release",
			demand:  5,
			request: 48,
			scaler: scaler{
				batch:  16,
				buffer: .5,
				max:    250,
			},
			nnccli:               nncClientMock{},
			store:                ipStateStoreWithCounts(16, 16),
			wantRequest:          16,
			wantPendingRelease:   16,
			wantPatchCalls:       1,
			wantPatchRequest:     16,
			wantMarkReleaseCalls: []int{0},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pm := &Monitor{
				z:       zap.NewNop(),
				demand:  tt.demand,
				request: tt.request,
				scaler:  tt.scaler,
				nnccli:  &tt.nnccli,
				store:   &tt.store,
			}
			err := pm.reconcile(context.Background())
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantRequest, pm.request)
			assert.Equal(t, tt.wantMarkReleaseCalls, tt.store.markCalls)
			assert.Len(t, tt.nnccli.patchSpecs, tt.wantPatchCalls)
			if tt.wantPatchCalls > 0 {
				spec := tt.nnccli.patchSpecs[tt.wantPatchCalls-1]
				assert.Equal(t, tt.wantPatchRequest, spec.RequestedIPCount)
				assert.Len(t, spec.IPsNotInUse, tt.wantPendingRelease)
			}
			if !tt.wantErr && tt.wantPatchCalls > 0 {
				assert.Equal(t, tt.wantPatchRequest, tt.nnccli.req.RequestedIPCount)
				assert.Len(t, tt.nnccli.req.IPsNotInUse, tt.wantPendingRelease)
			}
		})
	}
}

func TestReconcileRetriesPatchFailureWithoutExtraRelease(t *testing.T) {
	t.Parallel()

	store := ipStateStoreWithCounts(0, 32)
	nnccli := &nncClientMock{
		failPatchCnt: 1,
		err:          errors.New("failed to patch NNC Spec"),
	}
	pm := &Monitor{
		z:       zap.NewNop(),
		demand:  5,
		request: 32,
		scaler: scaler{
			batch:  16,
			buffer: .5,
			max:    250,
		},
		nnccli: nnccli,
		store:  &store,
	}

	err := pm.reconcile(context.Background())
	require.Error(t, err)
	assert.Equal(t, int64(32), pm.request)
	assert.Equal(t, []int{16}, store.markCalls)
	assert.Len(t, store.GetPendingReleaseIPConfigs(), 16)

	nnccli.err = nil
	err = pm.reconcile(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(16), pm.request)
	assert.Equal(t, []int{16, 0}, store.markCalls)
	require.Len(t, nnccli.patchSpecs, 2)
	assert.Equal(t, int64(16), nnccli.req.RequestedIPCount)
	assert.Len(t, nnccli.req.IPsNotInUse, 16)
}
