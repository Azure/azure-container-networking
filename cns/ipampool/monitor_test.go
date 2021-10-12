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
	d.m.scaler, d.m.spec = nnc.Status.Scaler, nnc.Spec
	d.m.state.minFreeCount, d.m.state.maxFreeCount = CalculateMinFreeIPs(d.m.scaler), CalculateMaxFreeIPs(d.m.scaler)
}

type state struct {
	allocatedIPCount        int
	batchSize               int
	ipConfigCount           int
	maxIPCount              int
	releaseThresholdPercent int
	requestThresholdPercent int
}

func initFakes(initState state) (*fakes.HTTPServiceFake, *fakes.RequestControllerFake, *Monitor) {
	logger.InitLogger("testlogs", 0, 0, "./")

	scalarUnits := v1alpha.Scaler{
		BatchSize:               int64(initState.batchSize),
		RequestThresholdPercent: int64(initState.requestThresholdPercent),
		ReleaseThresholdPercent: int64(initState.releaseThresholdPercent),
		MaxIPCount:              int64(initState.maxIPCount),
	}
	subnetaddresspace := "10.0.0.0/8"

	fakecns := fakes.NewHTTPServiceFake()
	fakerc := fakes.NewRequestControllerFake(fakecns, scalarUnits, subnetaddresspace, initState.ipConfigCount)

	poolmonitor := NewMonitor(fakecns, &fakeNodeNetworkConfigUpdater{fakerc.NNC}, &Options{RefreshDelay: 100 * time.Second})

	fakecns.PoolMonitor = &directUpdatePoolMonitor{m: poolmonitor}
	_ = fakecns.SetNumberOfAllocatedIPs(initState.allocatedIPCount)

	return fakecns, fakerc, poolmonitor
}

func TestPoolSizeIncrease(t *testing.T) {
	initState := state{
		batchSize:               10,
		allocatedIPCount:        8,
		ipConfigCount:           10,
		requestThresholdPercent: 50,
		releaseThresholdPercent: 150,
		maxIPCount:              30,
	}

	fakecns, fakerc, poolmonitor := initFakes(initState)
	assert.NoError(t, fakerc.Reconcile(true))

	// When poolmonitor reconcile is called, trigger increase and cache goal state
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// ensure pool monitor has reached quorum with cns
	assert.Equal(t, int64(initState.ipConfigCount+(1*initState.batchSize)), poolmonitor.spec.RequestedIPCount)

	// request controller reconciles, carves new IP's from the test subnet and adds to CNS state
	assert.NoError(t, fakerc.Reconcile(true))

	// when poolmonitor reconciles again here, the IP count will be within the thresholds
	// so no CRD update and nothing pending
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// ensure pool monitor has reached quorum with cns
	assert.Equal(t, int64(initState.ipConfigCount+(1*initState.batchSize)), poolmonitor.spec.RequestedIPCount)

	// make sure IPConfig state size reflects the new pool size
	assert.Len(t, fakecns.GetPodIPConfigState(), initState.ipConfigCount+(1*initState.batchSize))
}

func TestPoolIncreaseDoesntChangeWhenIncreaseIsAlreadyInProgress(t *testing.T) {
	initState := state{
		batchSize:               10,
		allocatedIPCount:        8,
		ipConfigCount:           10,
		requestThresholdPercent: 30,
		releaseThresholdPercent: 150,
		maxIPCount:              30,
	}

	fakecns, fakerc, poolmonitor := initFakes(initState)
	assert.NoError(t, fakerc.Reconcile(true))

	// When poolmonitor reconcile is called, trigger increase and cache goal state
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// increase number of allocated IP's in CNS, within allocatable size but still inside trigger threshold
	assert.NoError(t, fakecns.SetNumberOfAllocatedIPs(9))

	// poolmonitor reconciles, but doesn't actually update the CRD, because there is already a pending update
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// ensure pool monitor has reached quorum with cns
	assert.Equal(t, int64(initState.ipConfigCount+(1*initState.batchSize)), poolmonitor.spec.RequestedIPCount)

	// request controller reconciles, carves new IP's from the test subnet and adds to CNS state
	assert.NoError(t, fakerc.Reconcile(true))

	// when poolmonitor reconciles again here, the IP count will be within the thresholds
	// so no CRD update and nothing pending
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// make sure IPConfig state size reflects the new pool size
	assert.Len(t, fakecns.GetPodIPConfigState(), initState.ipConfigCount+(1*initState.batchSize))

	// ensure pool monitor has reached quorum with cns
	assert.Equal(t, int64(initState.ipConfigCount+(1*initState.batchSize)), poolmonitor.spec.RequestedIPCount)
}

func TestPoolSizeIncreaseIdempotency(t *testing.T) {
	initState := state{
		batchSize:               10,
		allocatedIPCount:        8,
		ipConfigCount:           10,
		requestThresholdPercent: 30,
		releaseThresholdPercent: 150,
		maxIPCount:              30,
	}

	_, fakerc, poolmonitor := initFakes(initState)
	assert.NoError(t, fakerc.Reconcile(true))

	// When poolmonitor reconcile is called, trigger increase and cache goal state
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// ensure pool monitor has increased batch size
	assert.Equal(t, int64(initState.ipConfigCount+(1*initState.batchSize)), poolmonitor.spec.RequestedIPCount)

	// reconcile pool monitor a second time, then verify requested ip count is still the same
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// ensure pool monitor requested pool size is unchanged as request controller hasn't reconciled yet
	assert.Equal(t, int64(initState.ipConfigCount+(1*initState.batchSize)), poolmonitor.spec.RequestedIPCount)
}

func TestPoolIncreasePastNodeLimit(t *testing.T) {
	initState := state{
		batchSize:               16,
		allocatedIPCount:        9,
		ipConfigCount:           16,
		requestThresholdPercent: 50,
		releaseThresholdPercent: 150,
		maxIPCount:              30,
	}

	_, fakerc, poolmonitor := initFakes(initState)
	assert.NoError(t, fakerc.Reconcile(true))

	// When poolmonitor reconcile is called, trigger increase and cache goal state
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// ensure pool monitor has only requested the max pod ip count
	assert.Equal(t, int64(initState.maxIPCount), poolmonitor.spec.RequestedIPCount)
}

func TestPoolIncreaseBatchSizeGreaterThanMaxPodIPCount(t *testing.T) {
	initState := state{
		batchSize:               50,
		allocatedIPCount:        16,
		ipConfigCount:           16,
		requestThresholdPercent: 50,
		releaseThresholdPercent: 150,
		maxIPCount:              30,
	}

	_, fakerc, poolmonitor := initFakes(initState)
	assert.NoError(t, fakerc.Reconcile(true))

	// When poolmonitor reconcile is called, trigger increase and cache goal state
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// ensure pool monitor has only requested the max pod ip count
	assert.Equal(t, int64(initState.maxIPCount), poolmonitor.spec.RequestedIPCount)
}

func TestPoolDecrease(t *testing.T) {
	initState := state{
		batchSize:               10,
		ipConfigCount:           20,
		allocatedIPCount:        15,
		requestThresholdPercent: 50,
		releaseThresholdPercent: 150,
		maxIPCount:              30,
	}

	fakecns, fakerc, poolmonitor := initFakes(initState)
	assert.NoError(t, fakerc.Reconcile(true))

	// Pool monitor does nothing, as the current number of IP's falls in the threshold
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// Decrease the number of allocated IP's down to 5. This should trigger a scale down
	assert.NoError(t, fakecns.SetNumberOfAllocatedIPs(4))

	// Pool monitor will adjust the spec so the pool size will be 1 batch size smaller
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// ensure that the adjusted spec is smaller than the initial pool size
	assert.Len(t, poolmonitor.spec.IPsNotInUse, initState.ipConfigCount-initState.batchSize)

	// reconcile the fake request controller
	assert.NoError(t, fakerc.Reconcile(true))

	// CNS won't actually clean up the IPsNotInUse until it changes the spec for some other reason (i.e. scale up)
	// so instead we should just verify that the CNS state has no more PendingReleaseIPConfigs,
	// and that they were cleaned up.
	assert.Empty(t, fakecns.GetPendingReleaseIPConfigs())
}

func TestPoolSizeDecreaseWhenDecreaseHasAlreadyBeenRequested(t *testing.T) {
	initState := state{
		batchSize:               10,
		allocatedIPCount:        5,
		ipConfigCount:           20,
		requestThresholdPercent: 30,
		releaseThresholdPercent: 100,
		maxIPCount:              30,
	}

	fakecns, fakerc, poolmonitor := initFakes(initState)
	assert.NoError(t, fakerc.Reconcile(true))

	// Pool monitor does nothing, as the current number of IP's falls in the threshold
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// Ensure the size of the requested spec is still the same
	assert.Len(t, poolmonitor.spec.IPsNotInUse, initState.ipConfigCount-initState.batchSize)

	// Ensure the request ipcount is now one batch size smaller than the initial IP count
	assert.Equal(t, int64(initState.ipConfigCount-initState.batchSize), poolmonitor.spec.RequestedIPCount)

	// Update pods with IP count, ensure pool monitor stays the same until request controller reconciles
	assert.NoError(t, fakecns.SetNumberOfAllocatedIPs(6))

	// Ensure the size of the requested spec is still the same
	assert.Len(t, poolmonitor.spec.IPsNotInUse, initState.ipConfigCount-initState.batchSize)

	// Ensure the request ipcount is now one batch size smaller than the initial IP count
	assert.Equal(t, int64(initState.ipConfigCount-initState.batchSize), poolmonitor.spec.RequestedIPCount)

	assert.NoError(t, fakerc.Reconcile(true))

	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// Ensure the spec doesn't have any IPsNotInUse after request controller has reconciled
	assert.Empty(t, poolmonitor.spec.IPsNotInUse)
}

func TestDecreaseAndIncreaseToSameCount(t *testing.T) {
	initState := state{
		batchSize:               10,
		allocatedIPCount:        7,
		ipConfigCount:           10,
		requestThresholdPercent: 50,
		releaseThresholdPercent: 150,
		maxIPCount:              30,
	}

	fakecns, fakerc, poolmonitor := initFakes(initState)
	assert.NoError(t, fakerc.Reconcile(true))
	assert.NoError(t, poolmonitor.reconcile(context.Background()))
	assert.Equal(t, int64(20), poolmonitor.spec.RequestedIPCount)
	assert.Empty(t, poolmonitor.spec.IPsNotInUse)

	// Update the IPConfig state
	assert.NoError(t, fakerc.Reconcile(true))

	// Release all IPs
	assert.NoError(t, fakecns.SetNumberOfAllocatedIPs(0))
	assert.NoError(t, poolmonitor.reconcile(context.Background()))
	assert.Equal(t, int64(10), poolmonitor.spec.RequestedIPCount)
	assert.Len(t, poolmonitor.spec.IPsNotInUse, 10)

	// Increase it back to 20
	// initial pool count is 10, set 5 of them to be allocated
	assert.NoError(t, fakecns.SetNumberOfAllocatedIPs(7))
	assert.NoError(t, poolmonitor.reconcile(context.Background()))
	assert.Equal(t, int64(20), poolmonitor.spec.RequestedIPCount)
	assert.Len(t, poolmonitor.spec.IPsNotInUse, 10)

	// Update the IPConfig count and dont remove the pending IPs
	assert.NoError(t, fakerc.Reconcile(false))
	assert.NoError(t, poolmonitor.reconcile(context.Background()))
	assert.Equal(t, int64(20), poolmonitor.spec.RequestedIPCount)
	assert.Len(t, poolmonitor.spec.IPsNotInUse, 10)

	assert.NoError(t, fakerc.Reconcile(true))
	assert.NoError(t, poolmonitor.reconcile(context.Background()))
	assert.NoError(t, poolmonitor.reconcile(context.Background()))
	assert.Equal(t, int64(20), poolmonitor.spec.RequestedIPCount)
	assert.Empty(t, poolmonitor.spec.IPsNotInUse)
}

func TestPoolSizeDecreaseToReallyLow(t *testing.T) {
	initState := state{
		batchSize:               10,
		allocatedIPCount:        23,
		ipConfigCount:           30,
		requestThresholdPercent: 30,
		releaseThresholdPercent: 100,
		maxIPCount:              30,
	}

	fakecns, fakerc, poolmonitor := initFakes(initState)
	assert.NoError(t, fakerc.Reconcile(true))

	// Pool monitor does nothing, as the current number of IP's falls in the threshold
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// Now Drop the Allocated count to really low, say 3. This should trigger release in 2 batches
	assert.NoError(t, fakecns.SetNumberOfAllocatedIPs(3))

	// Pool monitor does nothing, as the current number of IP's falls in the threshold
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// Ensure the size of the requested spec is still the same
	assert.Len(t, poolmonitor.spec.IPsNotInUse, initState.batchSize)

	// Ensure the request ipcount is now one batch size smaller than the initial IP count
	assert.Equal(t, int64(initState.ipConfigCount-initState.batchSize), poolmonitor.spec.RequestedIPCount)

	// Reconcile again, it should release the second batch
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// Ensure the size of the requested spec is still the same
	assert.Len(t, poolmonitor.spec.IPsNotInUse, initState.batchSize*2)

	// Ensure the request ipcount is now one batch size smaller than the initial IP count
	assert.Equal(t, int64(initState.ipConfigCount-(initState.batchSize*2)), poolmonitor.spec.RequestedIPCount)

	assert.NoError(t, fakerc.Reconcile(true))
	assert.NoError(t, poolmonitor.reconcile(context.Background()))
	assert.Empty(t, poolmonitor.spec.IPsNotInUse)
}

func TestDecreaseAfterNodeLimitReached(t *testing.T) {
	initState := state{
		batchSize:               16,
		allocatedIPCount:        20,
		ipConfigCount:           30,
		requestThresholdPercent: 50,
		releaseThresholdPercent: 150,
		maxIPCount:              30,
	}
	fakecns, fakerc, poolmonitor := initFakes(initState)
	assert.NoError(t, fakerc.Reconcile(true))

	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// Trigger a batch release
	assert.NoError(t, fakecns.SetNumberOfAllocatedIPs(5))
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// Ensure poolmonitor asked for a multiple of batch size
	assert.Equal(t, int64(16), poolmonitor.spec.RequestedIPCount)
	assert.Len(t, poolmonitor.spec.IPsNotInUse, int(initState.maxIPCount%initState.batchSize))
}

func TestPoolDecreaseBatchSizeGreaterThanMaxPodIPCount(t *testing.T) {
	initState := state{
		batchSize:               31,
		allocatedIPCount:        30,
		ipConfigCount:           30,
		requestThresholdPercent: 50,
		releaseThresholdPercent: 150,
		maxIPCount:              30,
	}

	fakecns, fakerc, poolmonitor := initFakes(initState)
	assert.NoError(t, fakerc.Reconcile(true))

	// When poolmonitor reconcile is called, trigger increase and cache goal state
	assert.NoError(t, poolmonitor.reconcile(context.Background()))

	// Trigger a batch release
	assert.NoError(t, fakecns.SetNumberOfAllocatedIPs(1))
	assert.NoError(t, poolmonitor.reconcile(context.Background()))
	assert.Equal(t, int64(initState.maxIPCount), poolmonitor.spec.RequestedIPCount)
}

func TestCalculateIPs(t *testing.T) {
	tests := []struct {
		name        string
		in          v1alpha.Scaler
		wantMinFree int
		wantMaxFree int
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
