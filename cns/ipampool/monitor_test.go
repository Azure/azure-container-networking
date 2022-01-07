package ipampool

import (
	"context"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/fakes"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/stretchr/testify/assert"
)

type fakeNodeNetworkConfigUpdater struct {
	nnc *v1alpha.NodeNetworkConfig
}

func (f *fakeNodeNetworkConfigUpdater) UpdateSpec(ctx context.Context, spec *v1alpha.NodeNetworkConfigSpec) (*v1alpha.NodeNetworkConfig, error) {
	f.nnc.Spec = *spec
	return f.nnc, nil
}

type directUpdatePoolMonitor struct {
	m *Monitor
	cns.IPAMPoolMonitor
}

func (d *directUpdatePoolMonitor) Update(nnc *v1alpha.NodeNetworkConfig) {
	scaler := nnc.Status.Scaler
	d.m.spec = nnc.Spec
	d.m.metastate.minFreeCount, d.m.metastate.maxFreeCount = CalculateMinFreeIPs(scaler), CalculateMaxFreeIPs(scaler)
}

type testState struct {
	allocated               int64
	assigned                int
	batch                   int64
	max                     int64
	releaseThresholdPercent int64
	requestThresholdPercent int64
}

func initFakes(state testState) (*fakes.HTTPServiceFake, *fakes.RequestControllerFake, *Monitor) {
	logger.InitLogger("testlogs", 0, 0, "./")

	scalarUnits := v1alpha.Scaler{
		BatchSize:               state.batch,
		RequestThresholdPercent: state.requestThresholdPercent,
		ReleaseThresholdPercent: state.releaseThresholdPercent,
		MaxIPCount:              state.max,
	}
	subnetaddresspace := "10.0.0.0/8"

	fakecns := fakes.NewHTTPServiceFake()
	fakerc := fakes.NewRequestControllerFake(fakecns, scalarUnits, subnetaddresspace, state.allocated)

	poolmonitor := NewMonitor(fakecns, &fakeNodeNetworkConfigUpdater{fakerc.NNC}, &Options{RefreshDelay: 100 * time.Second})
	poolmonitor.metastate = metaState{
		batch: state.batch,
		max:   state.max,
	}
	fakecns.PoolMonitor = &directUpdatePoolMonitor{m: poolmonitor}
	if err := fakecns.SetNumberOfAssignedIPs(state.assigned); err != nil {
		logger.Printf("%s", err)
	}

	return fakecns, fakerc, poolmonitor
}

func TestPoolSizeIncrease(t *testing.T) {
	initState := testState{
		batch:                   10,
		assigned:                8,
		allocated:               10,
		requestThresholdPercent: 50,
		releaseThresholdPercent: 150,
		max:                     30,
	}

	fakecns, fakerc, poolmonitor := initFakes(initState)
	assert.NoError(t, fakerc.Reconcile(true))

	// When poolmonitor reconcile is called, trigger increase and cache goal state
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// ensure pool monitor has reached quorum with cns
	assert.Equal(t, initState.allocated+(1*initState.batch), poolmonitor.spec.RequestedIPCount)

	// request controller reconciles, carves new IPs from the test subnet and adds to CNS state
	assert.NoError(t, fakerc.Reconcile(true))

	// when poolmonitor reconciles again here, the IP count will be within the thresholds
	// so no CRD update and nothing pending
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// ensure pool monitor has reached quorum with cns
	assert.Equal(t, initState.allocated+(1*initState.batch), poolmonitor.spec.RequestedIPCount)

	// make sure IPConfig state size reflects the new pool size
	assert.Len(t, fakecns.GetPodIPConfigState(), int(initState.allocated+(1*initState.batch)))
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

	fakecns, fakerc, poolmonitor := initFakes(initState)
	assert.NoError(t, fakerc.Reconcile(true))

	// When poolmonitor reconcile is called, trigger increase and cache goal state
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// increase number of allocated IPs in CNS, within allocatable size but still inside trigger threshold
	assert.NoError(t, fakecns.SetNumberOfAssignedIPs(9))

	// poolmonitor reconciles, but doesn't actually update the CRD, because there is already a pending update
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// ensure pool monitor has reached quorum with cns
	assert.Equal(t, initState.allocated+(1*initState.batch), poolmonitor.spec.RequestedIPCount)

	// request controller reconciles, carves new IPs from the test subnet and adds to CNS state
	assert.NoError(t, fakerc.Reconcile(true))

	// when poolmonitor reconciles again here, the IP count will be within the thresholds
	// so no CRD update and nothing pending
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// make sure IPConfig state size reflects the new pool size
	assert.Len(t, fakecns.GetPodIPConfigState(), int(initState.allocated+(1*initState.batch)))

	// ensure pool monitor has reached quorum with cns
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

	_, fakerc, poolmonitor := initFakes(initState)
	assert.NoError(t, fakerc.Reconcile(true))

	// When poolmonitor reconcile is called, trigger increase and cache goal state
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// ensure pool monitor has increased batch size
	assert.Equal(t, initState.allocated+(1*initState.batch), poolmonitor.spec.RequestedIPCount)

	// reconcile pool monitor a second time, then verify requested ip count is still the same
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// ensure pool monitor requested pool size is unchanged as request controller hasn't reconciled yet
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

	_, fakerc, poolmonitor := initFakes(initState)
	assert.NoError(t, fakerc.Reconcile(true))

	// When poolmonitor reconcile is called, trigger increase and cache goal state
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// ensure pool monitor has only requested the max pod ip count
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

	_, fakerc, poolmonitor := initFakes(initState)
	assert.NoError(t, fakerc.Reconcile(true))

	// When poolmonitor reconcile is called, trigger increase and cache goal state
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// ensure pool monitor has only requested the max pod ip count
	assert.Equal(t, initState.max, poolmonitor.spec.RequestedIPCount)
}

func TestPoolDecrease(t *testing.T) {
	initState := testState{
		batch:                   10,
		allocated:               20,
		assigned:                15,
		requestThresholdPercent: 50,
		releaseThresholdPercent: 150,
		max:                     30,
	}

	fakecns, fakerc, poolmonitor := initFakes(initState)
	assert.NoError(t, fakerc.Reconcile(true))

	// Pool monitor does nothing, as the current number of IPs falls in the threshold
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// Decrease the number of allocated IPs down to 5. This should trigger a scale down
	assert.NoError(t, fakecns.SetNumberOfAssignedIPs(4))

	// Pool monitor will adjust the spec so the pool size will be 1 batch size smaller
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// ensure that the adjusted spec is smaller than the initial pool size
	assert.Len(t, poolmonitor.spec.IPsNotInUse, int(initState.allocated-initState.batch))

	// reconcile the fake request controller
	assert.NoError(t, fakerc.Reconcile(true))

	// CNS won't actually clean up the IPsNotInUse until it changes the spec for some other reason (i.e. scale up)
	// so instead we should just verify that the CNS state has no more PendingReleaseIPConfigs,
	// and that they were cleaned up.
	assert.Empty(t, fakecns.GetPendingReleaseIPConfigs())
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

	fakecns, fakerc, poolmonitor := initFakes(initState)
	assert.NoError(t, fakerc.Reconcile(true))

	// Pool monitor does nothing, as the current number of IPs falls in the threshold
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// Ensure the size of the requested spec is still the same
	assert.Len(t, poolmonitor.spec.IPsNotInUse, int(initState.allocated-initState.batch))

	// Ensure the request ipcount is now one batch size smaller than the initial IP count
	assert.Equal(t, initState.allocated-initState.batch, poolmonitor.spec.RequestedIPCount)

	// Update pods with IP count, ensure pool monitor stays the same until request controller reconciles
	assert.NoError(t, fakecns.SetNumberOfAssignedIPs(6))

	// Ensure the size of the requested spec is still the same
	assert.Len(t, poolmonitor.spec.IPsNotInUse, int(initState.allocated-initState.batch))

	// Ensure the request ipcount is now one batch size smaller than the initial IP count
	assert.Equal(t, initState.allocated-initState.batch, poolmonitor.spec.RequestedIPCount)

	assert.NoError(t, fakerc.Reconcile(true))

	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// Ensure the spec doesn't have any IPsNotInUse after request controller has reconciled
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

	fakecns, fakerc, poolmonitor := initFakes(initState)
	assert.NoError(t, fakerc.Reconcile(true))
	assert.NoError(t, poolmonitor.reconcile(context.Background()))
	assert.EqualValues(t, 20, poolmonitor.spec.RequestedIPCount)
	assert.Empty(t, poolmonitor.spec.IPsNotInUse)

	// Update the IPConfig state
	assert.NoError(t, fakerc.Reconcile(true))

	// Release all IPs
	assert.NoError(t, fakecns.SetNumberOfAssignedIPs(0))
	assert.NoError(t, poolmonitor.reconcile(context.Background()))
	assert.EqualValues(t, 10, poolmonitor.spec.RequestedIPCount)
	assert.Len(t, poolmonitor.spec.IPsNotInUse, 10)

	// Increase it back to 20
	// initial pool count is 10, set 5 of them to be allocated
	assert.NoError(t, fakecns.SetNumberOfAssignedIPs(7))
	assert.NoError(t, poolmonitor.reconcile(context.Background()))
	assert.EqualValues(t, 20, poolmonitor.spec.RequestedIPCount)
	assert.Len(t, poolmonitor.spec.IPsNotInUse, 10)

	// Update the IPConfig count and dont remove the pending IPs
	assert.NoError(t, fakerc.Reconcile(false))
	assert.NoError(t, poolmonitor.reconcile(context.Background()))
	assert.EqualValues(t, 20, poolmonitor.spec.RequestedIPCount)
	assert.Len(t, poolmonitor.spec.IPsNotInUse, 10)

	assert.NoError(t, fakerc.Reconcile(true))
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

	fakecns, fakerc, poolmonitor := initFakes(initState)
	assert.NoError(t, fakerc.Reconcile(true))

	// Pool monitor does nothing, as the current number of IPs falls in the threshold
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// Now Drop the Assigned count to really low, say 3. This should trigger release in 2 batches
	assert.NoError(t, fakecns.SetNumberOfAssignedIPs(3))

	// Pool monitor does nothing, as the current number of IPs falls in the threshold
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// Ensure the size of the requested spec is still the same
	assert.Len(t, poolmonitor.spec.IPsNotInUse, int(initState.batch))

	// Ensure the request ipcount is now one batch size smaller than the initial IP count
	assert.Equal(t, initState.allocated-initState.batch, poolmonitor.spec.RequestedIPCount)

	// Reconcile again, it should release the second batch
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// Ensure the size of the requested spec is still the same
	assert.Len(t, poolmonitor.spec.IPsNotInUse, int(initState.batch*2))

	// Ensure the request ipcount is now one batch size smaller than the initial IP count
	assert.Equal(t, initState.allocated-(initState.batch*2), poolmonitor.spec.RequestedIPCount)

	assert.NoError(t, fakerc.Reconcile(true))
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
	fakecns, fakerc, poolmonitor := initFakes(initState)
	assert.NoError(t, fakerc.Reconcile(true))

	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// Trigger a batch release
	assert.NoError(t, fakecns.SetNumberOfAssignedIPs(5))
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// Ensure poolmonitor asked for a multiple of batch size
	assert.EqualValues(t, 16, poolmonitor.spec.RequestedIPCount)
	assert.Len(t, poolmonitor.spec.IPsNotInUse, int(initState.max%initState.batch))
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

	fakecns, fakerc, poolmonitor := initFakes(initState)
	assert.NoError(t, fakerc.Reconcile(true))

	// When poolmonitor reconcile is called, trigger increase and cache goal state
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// Trigger a batch release
	assert.NoError(t, fakecns.SetNumberOfAssignedIPs(1))
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
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantMinFree, CalculateMinFreeIPs(tt.in))
			assert.Equal(t, tt.wantMaxFree, CalculateMaxFreeIPs(tt.in))
		})
	}
}
