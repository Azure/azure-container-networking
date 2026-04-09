package ipampool

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/stretchr/testify/assert"
)

type fakeNodeNetworkConfigUpdater struct {
	nnc *v1alpha.NodeNetworkConfig
}

func (f *fakeNodeNetworkConfigUpdater) PatchSpec(_ context.Context, spec *v1alpha.NodeNetworkConfigSpec, _ string) (*v1alpha.NodeNetworkConfig, error) {
	f.nnc.Spec = *spec
	return f.nnc, nil
}

type fakeNodeNetworkConfigUpdaterFunc func(context.Context, *v1alpha.NodeNetworkConfigSpec, string) (*v1alpha.NodeNetworkConfig, error)

func (f fakeNodeNetworkConfigUpdaterFunc) PatchSpec(ctx context.Context, spec *v1alpha.NodeNetworkConfigSpec, owner string) (*v1alpha.NodeNetworkConfig, error) {
	return f(ctx, spec, owner)
}

// ipConfigStore is a coherent in-memory IP config store for testing.
// It satisfies the ipConfigState interface with a single map as source of truth.
type ipConfigStore struct {
	configs map[string]cns.IPConfigurationStatus
	nextIP  int
}

func newIPConfigStore() *ipConfigStore {
	return &ipConfigStore{configs: make(map[string]cns.IPConfigurationStatus)}
}

func (s *ipConfigStore) GetPodIPConfigState() map[string]cns.IPConfigurationStatus {
	m := make(map[string]cns.IPConfigurationStatus, len(s.configs))
	for k, v := range s.configs {
		m[k] = v
	}
	return m
}

func (s *ipConfigStore) GetPendingReleaseIPConfigs() []cns.IPConfigurationStatus {
	var out []cns.IPConfigurationStatus
	for _, v := range s.configs {
		if v.GetState() == types.PendingRelease {
			out = append(out, v)
		}
	}
	return out
}

func (s *ipConfigStore) MarkIPAsPendingRelease(n int) (map[string]cns.IPConfigurationStatus, error) {
	marked := make(map[string]cns.IPConfigurationStatus)
	for id, ipc := range s.configs {
		if len(marked) >= n {
			break
		}
		if ipc.GetState() == types.Available {
			ipc.SetState(types.PendingRelease)
			s.configs[id] = ipc
			marked[id] = ipc
		}
	}
	if len(marked) < n {
		return nil, fmt.Errorf("not enough available IPs to mark %d as pending release (found %d)", n, len(marked))
	}
	return marked, nil
}

// addAvailableIPs adds n new IPs in Available state.
func (s *ipConfigStore) addAvailableIPs(n int) {
	for i := 0; i < n; i++ {
		s.nextIP++
		id := fmt.Sprintf("ip-%d", s.nextIP)
		ipc := cns.IPConfigurationStatus{
			ID:        id,
			IPAddress: fmt.Sprintf("10.0.0.%d", s.nextIP),
		}
		ipc.SetState(types.Available)
		s.configs[id] = ipc
	}
}

// setAssignedTotal adjusts so that exactly n IPs are in Assigned state.
func (s *ipConfigStore) setAssignedTotal(n int) {
	current := 0
	for _, ipc := range s.configs {
		if ipc.GetState() == types.Assigned {
			current++
		}
	}
	delta := n - current
	if delta > 0 {
		for id, ipc := range s.configs {
			if delta == 0 {
				break
			}
			if ipc.GetState() == types.Available {
				ipc.SetState(types.Assigned)
				s.configs[id] = ipc
				delta--
			}
		}
	} else if delta < 0 {
		for id, ipc := range s.configs {
			if delta == 0 {
				break
			}
			if ipc.GetState() == types.Assigned {
				ipc.SetState(types.Available)
				s.configs[id] = ipc
				delta++
			}
		}
	}
}

// removePendingRelease deletes all PendingRelease IPs (simulates controller cleanup).
func (s *ipConfigStore) removePendingRelease() {
	for id, ipc := range s.configs {
		if ipc.GetState() == types.PendingRelease {
			delete(s.configs, id)
		}
	}
}

type testState struct {
	allocated               int64
	assigned                int
	batch                   int64
	exhausted               bool
	max                     int64
	pendingRelease          int64
	releaseThresholdPercent int64
	requestThresholdPercent int64
	totalIPs                int64
}

// newTestMonitor creates a pool monitor with a coherent ipConfigStore stub.
// It replaces the old initFakes + fakerc.Reconcile(true) setup.
func newTestMonitor(state testState, nnccli nodeNetworkConfigSpecUpdater) (*ipConfigStore, *Monitor) {
	logger.InitLogger("testlogs", 0, 0, "./")

	scaler := v1alpha.Scaler{
		BatchSize:               state.batch,
		RequestThresholdPercent: state.requestThresholdPercent,
		ReleaseThresholdPercent: state.releaseThresholdPercent,
		MaxIPCount:              state.max,
	}

	if state.totalIPs == 0 {
		state.totalIPs = state.allocated
	}

	store := newIPConfigStore()
	store.addAvailableIPs(int(state.totalIPs))
	store.setAssignedTotal(state.assigned)
	if state.pendingRelease > 0 {
		if _, err := store.MarkIPAsPendingRelease(int(state.pendingRelease)); err != nil {
			logger.Printf("%s", err)
		}
	}

	if nnccli == nil {
		nnccli = &fakeNodeNetworkConfigUpdater{nnc: &v1alpha.NodeNetworkConfig{
			Status: v1alpha.NodeNetworkConfigStatus{Scaler: scaler},
		}}
	}

	poolmonitor := NewMonitor(store, nnccli, nil, &Options{RefreshDelay: 100 * time.Second})
	poolmonitor.spec = v1alpha.NodeNetworkConfigSpec{
		RequestedIPCount: state.allocated,
	}
	poolmonitor.metastate = metaState{
		batch:        state.batch,
		max:          state.max,
		exhausted:    state.exhausted,
		minFreeCount: CalculateMinFreeIPs(scaler),
		maxFreeCount: CalculateMaxFreeIPs(scaler),
	}

	return store, poolmonitor
}

func TestPoolSizeIncrease(t *testing.T) {
	tests := []struct {
		name string
		in   testState
		want int64
	}{
		{
			name: "typ",
			in: testState{
				allocated:               10,
				assigned:                8,
				batch:                   10,
				max:                     30,
				releaseThresholdPercent: 150,
				requestThresholdPercent: 50,
			},
			want: 20,
		},
		{
			name: "odd batch",
			in: testState{
				allocated:               10,
				assigned:                10,
				batch:                   3,
				max:                     30,
				releaseThresholdPercent: 150,
				requestThresholdPercent: 50,
			},
			want: 12,
		},
		{
			name: "subnet exhausted",
			in: testState{
				allocated:               10,
				assigned:                8,
				batch:                   10,
				exhausted:               true,
				max:                     30,
				releaseThresholdPercent: 150,
				requestThresholdPercent: 50,
			},
			want: 9,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			store, poolmonitor := newTestMonitor(tt.in, nil)

			// reconcile triggers an increase request
			assert.NoError(t, poolmonitor.reconcile(context.Background()))
			assert.Equal(t, tt.want, poolmonitor.spec.RequestedIPCount)

			// simulate controller convergence: add/remove IPs to match requested count
			currentTotal := len(store.configs)
			desired := int(tt.want)
			if desired > currentTotal {
				store.addAvailableIPs(desired - currentTotal)
			} else if desired < currentTotal {
				store.removePendingRelease()
			}

			// reconcile again: pool is now within thresholds, no further action
			assert.NoError(t, poolmonitor.reconcile(context.Background()))
			assert.Equal(t, tt.want, poolmonitor.spec.RequestedIPCount)

			// verify the store reflects the converged pool size
			assert.Len(t, store.GetPodIPConfigState(), int(tt.want))
		})
	}
}

func TestPoolIncreaseDoesntChangeWhenIncreaseIsAlreadyInProgress(t *testing.T) {
	initState := testState{
		batch:                   10,
		assigned:                8,
		allocated:               10,
		requestThresholdPercent: 30,
		releaseThresholdPercent: 150,
		max:                     30,
	}

	store, poolmonitor := newTestMonitor(initState, nil)

	// reconcile triggers increase
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// increase assigned IPs within trigger threshold (but don't add new IPs from controller yet)
	store.setAssignedTotal(9)

	// poolmonitor reconciles again, but doesn't update because increase is already pending
	assert.NoError(t, poolmonitor.reconcile(context.Background()))
	assert.Equal(t, initState.allocated+(1*initState.batch), poolmonitor.spec.RequestedIPCount)

	// simulate controller convergence
	currentTotal := len(store.configs)
	desired := int(poolmonitor.spec.RequestedIPCount)
	if desired > currentTotal {
		store.addAvailableIPs(desired - currentTotal)
	}

	// reconcile: now within thresholds
	assert.NoError(t, poolmonitor.reconcile(context.Background()))
	assert.Len(t, store.GetPodIPConfigState(), int(initState.allocated+(1*initState.batch)))
	assert.Equal(t, initState.allocated+(1*initState.batch), poolmonitor.spec.RequestedIPCount)
}

func TestPoolSizeIncreaseIdempotency(t *testing.T) {
	initState := testState{
		batch:                   10,
		assigned:                8,
		allocated:               10,
		requestThresholdPercent: 30,
		releaseThresholdPercent: 150,
		max:                     30,
	}

	_, poolmonitor := newTestMonitor(initState, nil)

	// reconcile triggers increase
	assert.NoError(t, poolmonitor.reconcile(context.Background()))
	assert.Equal(t, initState.allocated+(1*initState.batch), poolmonitor.spec.RequestedIPCount)

	// reconcile again without controller convergence: requested count unchanged
	assert.NoError(t, poolmonitor.reconcile(context.Background()))
	assert.Equal(t, initState.allocated+(1*initState.batch), poolmonitor.spec.RequestedIPCount)
}

func TestPoolIncreasePastNodeLimit(t *testing.T) {
	initState := testState{
		batch:                   16,
		assigned:                9,
		allocated:               16,
		requestThresholdPercent: 50,
		releaseThresholdPercent: 150,
		max:                     30,
	}

	_, poolmonitor := newTestMonitor(initState, nil)

	assert.NoError(t, poolmonitor.reconcile(context.Background()))
	assert.Equal(t, initState.max, poolmonitor.spec.RequestedIPCount)
}

func TestPoolIncreaseBatchSizeGreaterThanMaxPodIPCount(t *testing.T) {
	initState := testState{
		batch:                   50,
		assigned:                16,
		allocated:               16,
		requestThresholdPercent: 50,
		releaseThresholdPercent: 150,
		max:                     30,
	}

	_, poolmonitor := newTestMonitor(initState, nil)

	assert.NoError(t, poolmonitor.reconcile(context.Background()))
	assert.Equal(t, initState.max, poolmonitor.spec.RequestedIPCount)
}

func TestIncreaseWithPendingRelease(t *testing.T) {
	initState := testState{
		batch:                   16,
		assigned:                16,
		allocated:               32,
		requestThresholdPercent: 50,
		releaseThresholdPercent: 150,
		max:                     250,
		pendingRelease:          16,
	}
	store, mon := newTestMonitor(initState, nil)

	// first reconcile: discovers pending release IPs and publishes them in IPsNotInUse
	assert.NoError(t, mon.reconcile(context.Background()))
	assert.Equal(t, int64(32), mon.spec.RequestedIPCount)
	assert.Len(t, mon.spec.IPsNotInUse, 16)

	// simulate controller removing pending release IPs
	store.removePendingRelease()

	// second reconcile: cleans up IPsNotInUse since pending release is now empty
	assert.NoError(t, mon.reconcile(context.Background()))
	assert.Equal(t, int64(32), mon.spec.RequestedIPCount)
	assert.Empty(t, mon.spec.IPsNotInUse)
}

func TestPoolDecrease(t *testing.T) {
	tests := []struct {
		name           string
		in             testState
		targetAssigned int
		want           int64
		wantReleased   int
	}{
		{
			name: "typ",
			in: testState{
				allocated:               20,
				assigned:                15,
				batch:                   10,
				max:                     30,
				releaseThresholdPercent: 150,
				requestThresholdPercent: 50,
			},
			targetAssigned: 5,
			want:           10,
			wantReleased:   10,
		},
		{
			name: "odd batch",
			in: testState{
				allocated:               21,
				assigned:                19,
				batch:                   3,
				max:                     30,
				releaseThresholdPercent: 150,
				requestThresholdPercent: 50,
			},
			targetAssigned: 15,
			want:           18,
			wantReleased:   3,
		},
		{
			name: "exhausted",
			in: testState{
				allocated:               20,
				assigned:                15,
				batch:                   10,
				exhausted:               true,
				max:                     30,
				releaseThresholdPercent: 150,
				requestThresholdPercent: 50,
			},
			targetAssigned: 15,
			want:           16,
			wantReleased:   4,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			store, poolmonitor := newTestMonitor(tt.in, nil)

			// decrease assigned IPs to trigger a scale down
			store.setAssignedTotal(tt.targetAssigned)

			assert.Eventually(t, func() bool {
				_ = poolmonitor.reconcile(context.Background())
				return tt.want == poolmonitor.spec.RequestedIPCount
			}, time.Second, 1*time.Millisecond)

			// verify that the adjusted spec is smaller than the initial pool size
			assert.Less(t, poolmonitor.spec.RequestedIPCount, tt.in.allocated)

			// verify that we have released the expected amount
			assert.Len(t, poolmonitor.spec.IPsNotInUse, tt.wantReleased)

			// simulate controller removing pending release IPs
			store.removePendingRelease()

			// verify the store reflects the new pool size
			assert.Len(t, store.GetPodIPConfigState(), int(tt.want))

			// verify no more pending release IPs
			assert.Empty(t, store.GetPendingReleaseIPConfigs())
		})
	}
}

func TestPoolSizeDecreaseWhenDecreaseHasAlreadyBeenRequested(t *testing.T) {
	initState := testState{
		batch:                   10,
		assigned:                5,
		allocated:               20,
		requestThresholdPercent: 30,
		releaseThresholdPercent: 100,
		max:                     30,
	}

	store, poolmonitor := newTestMonitor(initState, nil)

	// reconcile triggers decrease
	assert.NoError(t, poolmonitor.reconcile(context.Background()))
	assert.Len(t, poolmonitor.spec.IPsNotInUse, int(initState.allocated-initState.batch))
	assert.Equal(t, initState.allocated-initState.batch, poolmonitor.spec.RequestedIPCount)

	// update assigned count; spec stays the same until controller reconciles
	store.setAssignedTotal(6)
	assert.Len(t, poolmonitor.spec.IPsNotInUse, int(initState.allocated-initState.batch))
	assert.Equal(t, initState.allocated-initState.batch, poolmonitor.spec.RequestedIPCount)

	// simulate controller removing pending release IPs
	store.removePendingRelease()

	// reconcile cleans up IPsNotInUse
	assert.NoError(t, poolmonitor.reconcile(context.Background()))
	assert.Empty(t, poolmonitor.spec.IPsNotInUse)
}

func TestDecreaseAndIncreaseToSameCount(t *testing.T) {
	initState := testState{
		batch:                   10,
		assigned:                7,
		allocated:               10,
		requestThresholdPercent: 50,
		releaseThresholdPercent: 150,
		max:                     30,
	}

	store, poolmonitor := newTestMonitor(initState, nil)

	// reconcile triggers increase to 20
	assert.NoError(t, poolmonitor.reconcile(context.Background()))
	assert.EqualValues(t, 20, poolmonitor.spec.RequestedIPCount)
	assert.Empty(t, poolmonitor.spec.IPsNotInUse)

	// simulate controller convergence: add IPs to reach 20
	store.addAvailableIPs(10)

	// release all IPs
	store.setAssignedTotal(0)
	assert.NoError(t, poolmonitor.reconcile(context.Background()))
	assert.EqualValues(t, 10, poolmonitor.spec.RequestedIPCount)
	assert.Len(t, poolmonitor.spec.IPsNotInUse, 10)

	// increase it back: assign 7 pods
	store.setAssignedTotal(7)
	assert.NoError(t, poolmonitor.reconcile(context.Background()))
	assert.EqualValues(t, 20, poolmonitor.spec.RequestedIPCount)
	assert.Len(t, poolmonitor.spec.IPsNotInUse, 10)

	// reconcile again without removing pending IPs: stable
	assert.NoError(t, poolmonitor.reconcile(context.Background()))
	assert.EqualValues(t, 20, poolmonitor.spec.RequestedIPCount)
	assert.Len(t, poolmonitor.spec.IPsNotInUse, 10)

	// simulate controller removing pending release IPs
	store.removePendingRelease()

	// reconcile cleans up IPsNotInUse, then stabilizes
	assert.NoError(t, poolmonitor.reconcile(context.Background()))
	assert.NoError(t, poolmonitor.reconcile(context.Background()))
	assert.EqualValues(t, 20, poolmonitor.spec.RequestedIPCount)
	assert.Empty(t, poolmonitor.spec.IPsNotInUse)
}

func TestPoolSizeDecreaseToReallyLow(t *testing.T) {
	initState := testState{
		batch:                   10,
		assigned:                23,
		allocated:               30,
		requestThresholdPercent: 30,
		releaseThresholdPercent: 100,
		max:                     30,
	}

	store, poolmonitor := newTestMonitor(initState, nil)

	// initial reconcile: no action needed (within thresholds)
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// drop assigned count to 3, triggering release in multiple batches
	store.setAssignedTotal(3)

	// first reconcile: releases one batch
	assert.NoError(t, poolmonitor.reconcile(context.Background()))
	assert.Len(t, poolmonitor.spec.IPsNotInUse, int(initState.batch))
	assert.Equal(t, initState.allocated-initState.batch, poolmonitor.spec.RequestedIPCount)

	// second reconcile: releases another batch
	assert.NoError(t, poolmonitor.reconcile(context.Background()))
	assert.Len(t, poolmonitor.spec.IPsNotInUse, int(initState.batch*2))
	assert.Equal(t, initState.allocated-(initState.batch*2), poolmonitor.spec.RequestedIPCount)

	// simulate controller removing pending release IPs
	store.removePendingRelease()

	// reconcile cleans up
	assert.NoError(t, poolmonitor.reconcile(context.Background()))
	assert.Empty(t, poolmonitor.spec.IPsNotInUse)
}

func TestDecreaseAfterNodeLimitReached(t *testing.T) {
	initState := testState{
		batch:                   16,
		assigned:                20,
		allocated:               30,
		requestThresholdPercent: 50,
		releaseThresholdPercent: 150,
		max:                     30,
	}
	store, poolmonitor := newTestMonitor(initState, nil)

	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// trigger a batch release
	store.setAssignedTotal(5)
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// poolmonitor should ask for a multiple of batch size
	assert.EqualValues(t, 16, poolmonitor.spec.RequestedIPCount)
	assert.Len(t, poolmonitor.spec.IPsNotInUse, int(initState.max%initState.batch))
}

func TestDecreaseWithPendingRelease(t *testing.T) {
	initState := testState{
		batch:                   16,
		assigned:                46,
		allocated:               64,
		pendingRelease:          8,
		requestThresholdPercent: 50,
		releaseThresholdPercent: 150,
		totalIPs:                64,
		max:                     250,
	}
	store, poolmonitor := newTestMonitor(initState, nil)
	// override the spec to simulate a previous decrease request
	poolmonitor.spec.RequestedIPCount = 48

	// first reconcile: publishes pending release IPs
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// reallocate some IPs
	store.setAssignedTotal(40)
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	assert.EqualValues(t, 64, poolmonitor.spec.RequestedIPCount)
	assert.Len(t, poolmonitor.spec.IPsNotInUse, int(initState.pendingRelease))

	// trigger a batch release
	store.setAssignedTotal(30)
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	assert.EqualValues(t, 48, poolmonitor.spec.RequestedIPCount)
	assert.Len(t, poolmonitor.spec.IPsNotInUse, int(initState.batch)+int(initState.pendingRelease))
}

func TestDecreaseWithAPIServerFailure(t *testing.T) {
	initState := testState{
		batch:                   16,
		assigned:                46,
		allocated:               64,
		pendingRelease:          0,
		requestThresholdPercent: 50,
		releaseThresholdPercent: 150,
		totalIPs:                64,
		max:                     250,
	}
	var errNNCCLi fakeNodeNetworkConfigUpdaterFunc = func(context.Context, *v1alpha.NodeNetworkConfigSpec, string) (*v1alpha.NodeNetworkConfig, error) {
		return nil, errors.New("fake APIServer failure") //nolint:goerr113 // this is a fake error
	}

	store, poolmonitor := newTestMonitor(initState, errNNCCLi)

	// initial reconcile: no action
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// release some IPs
	store.setAssignedTotal(40)
	// pool monitor panics when it can't publish the updated NNC after retries
	assert.Panics(t, func() {
		_ = poolmonitor.reconcile(context.Background())
	})
}

func TestPoolDecreaseBatchSizeGreaterThanMaxPodIPCount(t *testing.T) {
	initState := testState{
		batch:                   31,
		assigned:                30,
		allocated:               30,
		requestThresholdPercent: 50,
		releaseThresholdPercent: 150,
		max:                     30,
	}

	store, poolmonitor := newTestMonitor(initState, nil)

	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// trigger a batch release
	store.setAssignedTotal(1)
	assert.NoError(t, poolmonitor.reconcile(context.Background()))
	assert.EqualValues(t, initState.max, poolmonitor.spec.RequestedIPCount)
}

func TestCalculateIPs(t *testing.T) {
	tests := []struct {
		name        string
		in          v1alpha.Scaler
		wantMinFree int64
		wantMaxFree int64
	}{
		{
			name: "normal",
			in: v1alpha.Scaler{
				BatchSize:               16,
				RequestThresholdPercent: 50,
				ReleaseThresholdPercent: 150,
				MaxIPCount:              250,
			},
			wantMinFree: 8,
			wantMaxFree: 24,
		},
		{
			name: "200%",
			in: v1alpha.Scaler{
				BatchSize:               16,
				RequestThresholdPercent: 100,
				ReleaseThresholdPercent: 200,
				MaxIPCount:              250,
			},
			wantMinFree: 16,
			wantMaxFree: 32,
		},
		{
			name: "odd batch",
			in: v1alpha.Scaler{
				BatchSize:               3,
				RequestThresholdPercent: 50,
				ReleaseThresholdPercent: 150,
				MaxIPCount:              250,
			},
			wantMinFree: 2,
			wantMaxFree: 5,
		},
		{
			name: "starvation",
			in: v1alpha.Scaler{
				BatchSize:               1,
				RequestThresholdPercent: 50,
				ReleaseThresholdPercent: 150,
				MaxIPCount:              250,
			},
			wantMinFree: 1,
			wantMaxFree: 2,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name+" min free", func(t *testing.T) {
			assert.Equal(t, tt.wantMinFree, CalculateMinFreeIPs(tt.in))
			t.Parallel()
		})
		t.Run(tt.name+" max free", func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.wantMaxFree, CalculateMaxFreeIPs(tt.in))
		})
	}
}
