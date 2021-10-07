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
	d.m.nnc = nnc
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

// func TestReconcile(t *testing.T) {
// 	tests := []struct {
// 		name string
// 		init state
// 		want state
// 	}{
// 		{
// 			name: "increase",
// 			init: state{
// 				initState.batchSize:               10,
// 				allocatedIPCount:        8,
// 				ipConfigCount:           10,
// 				requestThresholdPercent: 50,
// 				releaseThresholdPercent: 150,
// 				maxIPCount:              30,
// 			},
// 			want: state{
// 				allocatedIPCount: 8,
// 				ipConfigCount:    20,
// 			},
// 		},
// 		{
// 			name: "increase idempotency",
// 			init: state{
// 				initState.batchSize:               10,
// 				allocatedIPCount:        8,
// 				ipConfigCount:           10,
// 				requestThresholdPercent: 50,
// 				releaseThresholdPercent: 150,
// 				maxIPCount:              30,
// 			},
// 			want: state{
// 				allocatedIPCount: 9,
// 				ipConfigCount:    20,
// 			},
// 		},
// 		{
// 			name: "increase cap at node limit",
// 			init: state{
// 				initState.batchSize:               16,
// 				allocatedIPCount:        9,
// 				ipConfigCount:           16,
// 				requestThresholdPercent: 50,
// 				releaseThresholdPercent: 150,
// 				maxIPCount:              30,
// 			},
// 			want: state{
// 				allocatedIPCount: 9,
// 				ipConfigCount:    30,
// 			},
// 		},
// 		{
// 			name: "increase cap batch at node limit",
// 			init: state{
// 				initState.batchSize:               50,
// 				allocatedIPCount:        16,
// 				ipConfigCount:           16,
// 				requestThresholdPercent: 50,
// 				releaseThresholdPercent: 150,
// 				maxIPCount:              30,
// 			},
// 			want: state{
// 				allocatedIPCount: 16,
// 				ipConfigCount:    30,
// 			},
// 		},
// 		{
// 			name: "decrease",
// 			init: state{
// 				initState.batchSize:               10,
// 				allocatedIPCount:        15,
// 				ipConfigCount:           20,
// 				requestThresholdPercent: 50,
// 				releaseThresholdPercent: 150,
// 				maxIPCount:              30,
// 			},
// 			want: state{
// 				allocatedIPCount: 4,
// 				ipConfigCount:    10,
// 			},
// 		},
// 		{
// 			name: "decrease in-progress idempotency",
// 			init: state{
// 				initState.batchSize:               10,
// 				allocatedIPCount:        5,
// 				ipConfigCount:           20,
// 				requestThresholdPercent: 50,
// 				releaseThresholdPercent: 150,
// 				maxIPCount:              30,
// 			},
// 			want: state{
// 				allocatedIPCount: 6,
// 				ipConfigCount:    10,
// 			},
// 		},
// 		{
// 			name: "decrease multiple batches",
// 			init: state{
// 				initState.batchSize:               10,
// 				allocatedIPCount:        23,
// 				ipConfigCount:           30,
// 				requestThresholdPercent: 50,
// 				releaseThresholdPercent: 150,
// 				maxIPCount:              30,
// 			},
// 			want: state{
// 				allocatedIPCount: 3,
// 				ipConfigCount:    10,
// 			},
// 		},

// 		{
// 			name: "decrease to zero",
// 			init: state{
// 				initState.batchSize:               10,
// 				allocatedIPCount:        23,
// 				ipConfigCount:           30,
// 				requestThresholdPercent: 50,
// 				releaseThresholdPercent: 150,
// 				maxIPCount:              30,
// 			},
// 			want: state{
// 				allocatedIPCount: 0,
// 				ipConfigCount:    10,
// 			},
// 		},
// 	}
// 	for _, tt := range tests {
// 		tt := tt
// 		t.Run(tt.name, func(t *testing.T) {
// 			fakecns, fakerc, poolmonitor := initFakes(tt.init)
// 			require.NoError(t, fakerc.Reconcile(true))
// 			require.NoError(t, poolmonitor.reconcile(context.Background()))
// 			require.NoError(t, fakecns.SetNumberOfAllocatedIPs(tt.want.allocatedIPCount))

// 			// reconcile until CNS and pool monitor have converged
// 			for tt.want.ipConfigCount != len(fakecns.GetPodIPConfigState()) || tt.want.ipConfigCount != int(poolmonitor.nnc.Spec.RequestedIPCount) {
// 				// request controller reconciles, carves new IPs from the test subnet and adds to CNS state
// 				require.NoError(t, fakerc.Reconcile(true))
// 				// pool monitor reconciles, makes new pool size requests
// 				require.NoError(t, poolmonitor.reconcile(context.Background()))
// 			}

// 			require.Zero(t, len(fakecns.GetPendingReleaseIPConfigs()))
// 		})
// 	}
// }

// func TestPoolSizeDecreaseToReallyLow(t *testing.T) {
// 	var (
// 		initState.batchSize               = 10
// 		initialAllocated        = 23
// 		initState.ipConfigCount    = 30
// 		requestThresholdPercent = 30
// 		releaseThresholdPercent = 100
// 		maxIPCount           = 30
// 	)

// 	fakecns, fakerc, poolmonitor := initFakes(t,
// 		initState.batchSize,
// 		initState.ipConfigCount,
// 		requestThresholdPercent,
// 		releaseThresholdPercent,
// 		maxIPCount,
// 		initialAllocated,
// 	)

// 	// Pool monitor does nothing, as the current number of IP's falls in the threshold
// 	assert.NoError(t, poolmonitor.reconcile(context.Background()))

// 	// Now Drop the Allocated count to really low, say 3. This should trigger release in 2 batches
// 	assert.NoError(t, fakecns.SetNumberOfAllocatedIPs(3))

// 	// Pool monitor does nothing, as the current number of IP's falls in the threshold
// 	t.Logf("Reconcile after Allocated count from 33 -> 3, Exepected free count = 10")
// 	assert.NoError(t, poolmonitor.reconcile(context.Background()))

// 	// Ensure the size of the requested spec is still the same
// 	if len(poolmonitor.nnc.Spec.IPsNotInUse) != initState.batchSize {
// 		t.Fatalf("Expected IP's not in use is not correct, expected %v, actual %v",
// 			initState.batchSize, len(poolmonitor.nnc.Spec.IPsNotInUse))
// 	}

// 	// Ensure the request ipcount is now one batch size smaller than the initial IP count
// 	if poolmonitor.nnc.Spec.RequestedIPCount != int64(initState.ipConfigCount-initState.batchSize) {
// 		t.Fatalf("Expected pool size to be one batch size smaller after reconcile, expected %v, "+
// 			"actual %v", (initState.ipConfigCount - initState.batchSize), len(poolmonitor.nnc.Spec.IPsNotInUse))
// 	}

// 	// Reconcile again, it should release the second batch
// 	t.Logf("Reconcile again - 2, Exepected free count = 20")
// 	assert.NoError(t, poolmonitor.reconcile(context.Background()))

// 	// Ensure the size of the requested spec is still the same
// 	if len(poolmonitor.nnc.Spec.IPsNotInUse) != initState.batchSize*2 {
// 		t.Fatalf("Expected IP's not in use is not correct, expected %v, actual %v", initState.batchSize*2,
// 			len(poolmonitor.nnc.Spec.IPsNotInUse))
// 	}

// 	// Ensure the request ipcount is now one batch size smaller than the inital IP count
// 	if poolmonitor.nnc.Spec.RequestedIPCount != int64(initState.ipConfigCount-(initState.batchSize*2)) {
// 		t.Fatalf("Expected pool size to be one batch size smaller after reconcile, expected %v, "+
// 			"actual %v", (initState.ipConfigCount - initState.batchSize), len(poolmonitor.nnc.Spec.IPsNotInUse))
// 	}

// 	t.Logf("Update Request Controller")
// 	assert.NoError(t, fakerc.Reconcile(true))

// 	assert.NoError(t, poolmonitor.reconcile(context.Background()))

// 	// Ensure the spec doesn't have any IPsNotInUse after request controller has reconciled
// 	if len(poolmonitor.nnc.Spec.IPsNotInUse) != 0 {
// 		t.Fatalf("Expected IP's not in use to be 0 after reconcile, expected %v, actual %v",
// 			(initState.ipConfigCount - initState.batchSize), len(poolmonitor.nnc.Spec.IPsNotInUse))
// 	}
// }

// func TestDecreaseAfterNodeLimitReached(t *testing.T) {
// 	var (
// 		initState.batchSize               = 16
// 		initialAllocated        = 20
// 		initState.ipConfigCount    = 30
// 		requestThresholdPercent = 50
// 		releaseThresholdPercent = 150
// 		maxIPCount           = 30
// 		expectedRequestedIP     = 16
// 		expectedDecreaseIP      = int(maxIPCount) % initState.batchSize
// 	)

// 	fakecns, _, poolmonitor := initFakes(t,
// 		initState.batchSize,
// 		initState.ipConfigCount,
// 		requestThresholdPercent,
// 		releaseThresholdPercent,
// 		maxIPCount,
// 		initialAllocated,
// 	)

// 	assert.NoError(t, poolmonitor.reconcile(context.Background()))

// 	// Trigger a batch release
// 	assert.NoError(t, fakecns.SetNumberOfAllocatedIPs(5))

// 	assert.NoError(t, poolmonitor.reconcile(context.Background()))

// 	// Ensure poolmonitor asked for a multiple of batch size
// 	if poolmonitor.nnc.Spec.RequestedIPCount != int64(expectedRequestedIP) {
// 		t.Fatalf("Expected requested ips to be %v when scaling by 1 batch size down from %v "+
// 			"(max pod limit) but got %v", expectedRequestedIP, maxIPCount,
// 			poolmonitor.nnc.Spec.RequestedIPCount)
// 	}

// 	// Ensure we minused by the mod result
// 	if len(poolmonitor.nnc.Spec.IPsNotInUse) != expectedDecreaseIP {
// 		t.Fatalf("Expected to decrease requested IPs by %v (max pod count mod batchsize) to "+
// 			"make the requested ip count a multiple of the batch size in the case of hitting "+
// 			"the max before scale down, but got %v", expectedDecreaseIP, len(poolmonitor.nnc.Spec.IPsNotInUse))
// 	}
// }

// func TestPoolDecreaseBatchSizeGreaterThanMaxPodIPCount(t *testing.T) {
// 	var (
// 		initState.batchSize               = 31
// 		initialAllocated        = 30
// 		initState.ipConfigCount    = 30
// 		requestThresholdPercent = 50
// 		releaseThresholdPercent = 150
// 		maxIPCount           = 30
// 	)

// 	fakecns, _, poolmonitor := initFakes(t,
// 		initState.batchSize,
// 		initState.ipConfigCount,
// 		requestThresholdPercent,
// 		releaseThresholdPercent,
// 		maxIPCount,
// 		initialAllocated,
// 	)

// 	// When poolmonitor reconcile is called, trigger increase and cache goal state
// 	assert.NoError(t, poolmonitor.reconcile(context.Background()))

// 	// Trigger a batch release
// 	assert.NoError(t, fakecns.SetNumberOfAllocatedIPs(1))

// 	assert.NoError(t, poolmonitor.reconcile(context.Background()))

// 	// ensure pool monitor has only requested the max pod ip count
// 	if poolmonitor.nnc.Spec.RequestedIPCount != int64(maxIPCount) {
// 		t.Fatalf("Pool monitor target IP count (%v) should be the node limit (%v) when the max "+
// 			"has been reached", poolmonitor.nnc.Spec.RequestedIPCount, maxIPCount)
// 	}
// }

// func ReconcileAndValidate(ctx context.Context, t *testing.T, poolmonitor *Monitor, expectedRequestCount, expectedIpsNotInUse int) {
// 	assert.NoError(t, poolmonitor.reconcile(context.Background()))

// 	// Increased the new count to be 20
// 	if poolmonitor.nnc.Spec.RequestedIPCount != int64(expectedRequestCount) {
// 		t.Fatalf("RequestIPCount not same, expected %v, actual %v",
// 			expectedRequestCount,
// 			poolmonitor.nnc.Spec.RequestedIPCount)
// 	}

// 	// Ensure there is no pending release ips
// 	if len(poolmonitor.nnc.Spec.IPsNotInUse) != expectedIpsNotInUse {
// 		t.Fatalf("Expected IP's not in use, expected %v, actual %v",
// 			expectedIpsNotInUse,
// 			len(poolmonitor.nnc.Spec.IPsNotInUse))
// 	}
// }

// func TestDecreaseAndIncreaseToSameCount(t *testing.T) {
// 	var (
// 		initState.batchSize               = 10
// 		initialAllocated        = 7
// 		initState.ipConfigCount    = 10
// 		requestThresholdPercent = 50
// 		releaseThresholdPercent = 150
// 		maxIPCount           = 30
// 	)

// 	fakecns, fakerc, poolmonitor := initFakes(t,
// 		initState.batchSize,
// 		initState.ipConfigCount,
// 		requestThresholdPercent,
// 		releaseThresholdPercent,
// 		maxIPCount,
// 		initialAllocated,
// 	)

// 	// Pool monitor will increase the count to 20
// 	t.Logf("Scaleup: Increase pool size to 20")
// 	ReconcileAndValidate(context.Background(), t, poolmonitor, 20, 0)

// 	// Update the IPConfig state
// 	t.Logf("Reconcile with PodIPState")
// 	err := fakerc.Reconcile(true)
// 	if err != nil {
// 		t.Error(err)
// 	}

// 	// Release all IPs
// 	assert.NoError(t, fakecns.SetNumberOfAllocatedIPs(0))

// 	t.Logf("Scaledown: Decrease pool size to 10")
// 	ReconcileAndValidate(context.Background(), t, poolmonitor, 10, 10)

// 	// Increase it back to 20
// 	// initial pool count is 10, set 5 of them to be allocated
// 	t.Logf("Scaleup:  pool size back to 20 without updating the PodIpState for previous scale down")
// 	assert.NoError(t, fakecns.SetNumberOfAllocatedIPs(7))
// 	ReconcileAndValidate(context.Background(), t, poolmonitor, 20, 10)

// 	// Update the IPConfig count and dont remove the pending IPs
// 	t.Logf("Reconcile with PodIPState")
// 	assert.NoError(t, fakerc.Reconcile(false))

// 	// reconcile again
// 	t.Logf("Reconcole with pool monitor again, it should not cleanup ipsnotinuse")
// 	ReconcileAndValidate(context.Background(), t, poolmonitor, 20, 10)

// 	t.Logf("Now update podipconfig state")
// 	assert.NoError(t, fakerc.Reconcile(true))

// 	assert.NoError(t, poolmonitor.reconcile(context.Background()))
// 	ReconcileAndValidate(context.Background(), t, poolmonitor, 20, 0)
// }

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
	err := poolmonitor.reconcile(context.Background())
	if err != nil {
		t.Fatalf("Failed to allocate test ipconfigs with err: %v", err)
	}

	// ensure pool monitor has reached quorum with cns
	if poolmonitor.nnc.Spec.RequestedIPCount != int64(initState.ipConfigCount+(1*initState.batchSize)) {
		t.Fatalf("Pool monitor target IP count doesn't match CNS pool state "+
			"after reconcile: %v, "+
			"actual %v", poolmonitor.nnc.Spec.RequestedIPCount, len(fakecns.GetPodIPConfigState()))
	}

	// request controller reconciles, carves new IP's from the test subnet and adds to CNS state
	err = fakerc.Reconcile(true)
	if err != nil {
		t.Fatalf("Failed to reconcile fake requestcontroller with err: %v", err)
	}

	// when poolmonitor reconciles again here, the IP count will be within the thresholds
	// so no CRD update and nothing pending
	err = poolmonitor.reconcile(context.Background())
	if err != nil {
		t.Fatalf("Failed to reconcile pool monitor after request controller updates CNS state: %v", err)
	}

	// ensure pool monitor has reached quorum with cns
	if poolmonitor.nnc.Spec.RequestedIPCount != int64(initState.ipConfigCount+(1*initState.batchSize)) {
		t.Fatalf("Pool monitor target IP count doesn't "+
			"match CNS pool state after reconcile: %v, "+
			"actual %v", poolmonitor.nnc.Spec.RequestedIPCount, len(fakecns.GetPodIPConfigState()))
	}

	// make sure IPConfig state size reflects the new pool size
	if len(fakecns.GetPodIPConfigState()) != initState.ipConfigCount+(1*initState.batchSize) {
		t.Fatalf("CNS Pod IPConfig state count doesn't "+
			"match, expected: %v, actual %v", len(fakecns.GetPodIPConfigState()), initState.ipConfigCount+(1*initState.batchSize))
	}

	t.Logf("Pool size %v, Target pool size %v, "+
		"Allocated IP's %v, ", len(fakecns.GetPodIPConfigState()),
		poolmonitor.nnc.Spec.RequestedIPCount, len(fakecns.GetAllocatedIPConfigs()))
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
	err := poolmonitor.reconcile(context.Background())
	if err != nil {
		t.Fatalf("Failed to allocate test ipconfigs with err: %v", err)
	}

	// increase number of allocated IP's in CNS, within allocatable size but still inside trigger threshold,
	err = fakecns.SetNumberOfAllocatedIPs(9)
	if err != nil {
		t.Fatalf("Failed to allocate test ipconfigs with err: %v", err)
	}

	// poolmonitor reconciles, but doesn't actually update the CRD, because there is already a pending update
	err = poolmonitor.reconcile(context.Background())
	if err != nil {
		t.Fatalf("Failed to reconcile pool monitor after allocation ip increase with err: %v", err)
	}

	// ensure pool monitor has reached quorum with cns
	if poolmonitor.nnc.Spec.RequestedIPCount != int64(initState.ipConfigCount+(1*initState.batchSize)) {
		t.Fatalf("Pool monitor target IP count doesn't match CNS pool state after reconcile: %v,"+
			" actual %v", poolmonitor.nnc.Spec.RequestedIPCount, len(fakecns.GetPodIPConfigState()))
	}

	// request controller reconciles, carves new IP's from the test subnet and adds to CNS state
	err = fakerc.Reconcile(true)
	if err != nil {
		t.Fatalf("Failed to reconcile fake requestcontroller with err: %v", err)
	}

	// when poolmonitor reconciles again here, the IP count will be within the thresholds
	// so no CRD update and nothing pending
	err = poolmonitor.reconcile(context.Background())
	if err != nil {
		t.Fatalf("Failed to reconcile pool monitor after request controller updates CNS state: %v", err)
	}

	// make sure IPConfig state size reflects the new pool size
	if len(fakecns.GetPodIPConfigState()) != initState.ipConfigCount+(1*initState.batchSize) {
		t.Fatalf("CNS Pod IPConfig state count doesn't match, expected: %v, actual %v",
			len(fakecns.GetPodIPConfigState()), initState.ipConfigCount+(1*initState.batchSize))
	}

	// ensure pool monitor has reached quorum with cns
	if poolmonitor.nnc.Spec.RequestedIPCount != int64(initState.ipConfigCount+(1*initState.batchSize)) {
		t.Fatalf("Pool monitor target IP count doesn't match CNS pool state after reconcile: %v, "+
			"actual %v", poolmonitor.nnc.Spec.RequestedIPCount, len(fakecns.GetPodIPConfigState()))
	}

	t.Logf("Pool size %v, Target pool size %v, Allocated IP's %v, ", len(fakecns.GetPodIPConfigState()),
		poolmonitor.nnc.Spec.RequestedIPCount, len(fakecns.GetAllocatedIPConfigs()))
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

	fakecns, fakerc, poolmonitor := initFakes(initState)
	assert.NoError(t, fakerc.Reconcile(true))

	// When poolmonitor reconcile is called, trigger increase and cache goal state
	err := poolmonitor.reconcile(context.Background())
	if err != nil {
		t.Fatalf("Failed to allocate test ipconfigs with err: %v", err)
	}

	// ensure pool monitor has increased batch size
	if poolmonitor.nnc.Spec.RequestedIPCount != int64(initState.ipConfigCount+(1*initState.batchSize)) {
		t.Fatalf("Pool monitor target IP count doesn't match CNS pool state after reconcile: %v,"+
			" actual %v", poolmonitor.nnc.Spec.RequestedIPCount, len(fakecns.GetPodIPConfigState()))
	}

	// reconcile pool monitor a second time, then verify requested ip count is still the same
	err = poolmonitor.reconcile(context.Background())
	if err != nil {
		t.Fatalf("Failed to allocate test ipconfigs with err: %v", err)
	}

	// ensure pool monitor requested pool size is unchanged as request controller hasn't reconciled yet
	if poolmonitor.nnc.Spec.RequestedIPCount != int64(initState.ipConfigCount+(1*initState.batchSize)) {
		t.Fatalf("Pool monitor target IP count doesn't match CNS pool state after reconcile: %v,"+
			" actual %v", poolmonitor.nnc.Spec.RequestedIPCount, len(fakecns.GetPodIPConfigState()))
	}
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
	err := poolmonitor.reconcile(context.Background())
	if err != nil {
		t.Fatalf("Failed to allocate test ipconfigs with err: %v", err)
	}

	// ensure pool monitor has only requested the max pod ip count
	if poolmonitor.nnc.Spec.RequestedIPCount != int64(initState.maxIPCount) {
		t.Fatalf("Pool monitor target IP count (%v) should be the node limit (%v) when the max "+
			"has been reached", poolmonitor.nnc.Spec.RequestedIPCount, initState.maxIPCount)
	}
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
	err := poolmonitor.reconcile(context.Background())
	if err != nil {
		t.Fatalf("Failed to allocate test ipconfigs with err: %v", err)
	}

	// ensure pool monitor has only requested the max pod ip count
	if poolmonitor.nnc.Spec.RequestedIPCount != int64(initState.maxIPCount) {
		t.Fatalf("Pool monitor target IP count (%v) should be the node limit (%v) "+
			"when the max has been reached", poolmonitor.nnc.Spec.RequestedIPCount, initState.maxIPCount)
	}
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
	assert.Equal(t, (initState.ipConfigCount - initState.batchSize), len(poolmonitor.nnc.Spec.IPsNotInUse), "expected pool size to be smaller after reconcile")

	// reconcile the fake request controller
	assert.NoError(t, fakerc.Reconcile(true))

	// CNS won't actually clean up the IPsNotInUse until it changes the spec for some other reason (i.e. scale up)
	// so instead we should just verify that the CNS state has no more PendingReleaseIPConfigs,
	// and that they were cleaned up.
	assert.Zero(t, len(fakecns.GetPendingReleaseIPConfigs()), "expected 0 PendingReleaseIPConfigs")
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
	err := poolmonitor.reconcile(context.Background())
	if err != nil {
		t.Errorf("Expected pool monitor to not fail after CNS set number of allocated IP's %v", err)
	}

	// Ensure the size of the requested spec is still the same
	if len(poolmonitor.nnc.Spec.IPsNotInUse) != (initState.ipConfigCount - initState.batchSize) {
		t.Fatalf("Expected IP's not in use be one batch size smaller after reconcile, expected %v,"+
			" actual %v", (initState.ipConfigCount - initState.batchSize), len(poolmonitor.nnc.Spec.IPsNotInUse))
	}

	// Ensure the request ipcount is now one batch size smaller than the inital IP count
	if poolmonitor.nnc.Spec.RequestedIPCount != int64(initState.ipConfigCount-initState.batchSize) {
		t.Fatalf("Expected pool size to be one batch size smaller after reconcile, expected %v,"+
			" actual %v", (initState.ipConfigCount - initState.batchSize), len(poolmonitor.nnc.Spec.IPsNotInUse))
	}

	// Update pods with IP count, ensure pool monitor stays the same until request controller reconciles
	err = fakecns.SetNumberOfAllocatedIPs(6)
	if err != nil {
		t.Error(err)
	}

	// Ensure the size of the requested spec is still the same
	if len(poolmonitor.nnc.Spec.IPsNotInUse) != (initState.ipConfigCount - initState.batchSize) {
		t.Fatalf("Expected IP's not in use to be one batch size smaller after reconcile, and not change"+
			" after reconcile, expected %v, actual %v",
			(initState.ipConfigCount - initState.batchSize), len(poolmonitor.nnc.Spec.IPsNotInUse))
	}

	// Ensure the request ipcount is now one batch size smaller than the inital IP count
	if poolmonitor.nnc.Spec.RequestedIPCount != int64(initState.ipConfigCount-initState.batchSize) {
		t.Fatalf("Expected pool size to be one batch size smaller after reconcile, and not change after"+
			" existing call, expected %v, actual %v", (initState.ipConfigCount - initState.batchSize),
			len(poolmonitor.nnc.Spec.IPsNotInUse))
	}

	err = fakerc.Reconcile(true)
	if err != nil {
		t.Error(err)
	}

	err = poolmonitor.reconcile(context.Background())
	if err != nil {
		t.Errorf("Expected no pool monitor failure after request controller reconcile: %v", err)
	}

	// Ensure the spec doesn't have any IPsNotInUse after request controller has reconciled
	if len(poolmonitor.nnc.Spec.IPsNotInUse) != 0 {
		t.Fatalf("Expected IP's not in use to be 0 after reconcile, expected %v, actual %v",
			(initState.ipConfigCount - initState.batchSize), len(poolmonitor.nnc.Spec.IPsNotInUse))
	}
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

	// Pool monitor will increase the count to 20
	t.Logf("Scaleup: Increase pool size to 20")
	ReconcileAndValidate(context.Background(), t, poolmonitor, 20, 0)

	// Update the IPConfig state
	t.Logf("Reconcile with PodIPState")
	err := fakerc.Reconcile(true)
	if err != nil {
		t.Error(err)
	}

	// Release all IPs
	err = fakecns.SetNumberOfAllocatedIPs(0)
	if err != nil {
		t.Error(err)
	}

	t.Logf("Scaledown: Decrease pool size to 10")
	ReconcileAndValidate(context.Background(), t, poolmonitor, 10, 10)

	// Increase it back to 20
	// initial pool count is 10, set 5 of them to be allocated
	t.Logf("Scaleup:  pool size back to 20 without updating the PodIpState for previous scale down")
	err = fakecns.SetNumberOfAllocatedIPs(7)
	if err != nil {
		t.Error(err)
	}
	ReconcileAndValidate(context.Background(), t, poolmonitor, 20, 10)

	// Update the IPConfig count and dont remove the pending IPs
	t.Logf("Reconcile with PodIPState")
	err = fakerc.Reconcile(false)
	if err != nil {
		t.Error(err)
	}

	// reconcile again
	t.Logf("Reconcole with pool monitor again, it should not cleanup ipsnotinuse")
	ReconcileAndValidate(context.Background(), t, poolmonitor, 20, 10)

	t.Logf("Now update podipconfig state")
	err = fakerc.Reconcile(true)
	if err != nil {
		t.Error(err)
	}

	err = poolmonitor.reconcile(context.Background())
	if err != nil {
		t.Errorf("Expected no pool monitor failure after request controller reconcile: %v", err)
	}
	ReconcileAndValidate(context.Background(), t, poolmonitor, 20, 0)
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
	err := poolmonitor.reconcile(context.Background())
	if err != nil {
		t.Errorf("Expected pool monitor to not fail after CNS set number of allocated IP's %v", err)
	}

	// Now Drop the Allocated count to really low, say 3. This should trigger release in 2 batches
	err = fakecns.SetNumberOfAllocatedIPs(3)
	if err != nil {
		t.Error(err)
	}

	// Pool monitor does nothing, as the current number of IP's falls in the threshold
	t.Logf("Reconcile after Allocated count from 33 -> 3, Exepected free count = 10")
	err = poolmonitor.reconcile(context.Background())
	if err != nil {
		t.Errorf("Expected pool monitor to not fail after CNS set number of allocated IP's %v", err)
	}

	// Ensure the size of the requested spec is still the same
	if len(poolmonitor.nnc.Spec.IPsNotInUse) != initState.batchSize {
		t.Fatalf("Expected IP's not in use is not correct, expected %v, actual %v",
			initState.batchSize, len(poolmonitor.nnc.Spec.IPsNotInUse))
	}

	// Ensure the request ipcount is now one batch size smaller than the initial IP count
	if poolmonitor.nnc.Spec.RequestedIPCount != int64(initState.ipConfigCount-initState.batchSize) {
		t.Fatalf("Expected pool size to be one batch size smaller after reconcile, expected %v, "+
			"actual %v", (initState.ipConfigCount - initState.batchSize), len(poolmonitor.nnc.Spec.IPsNotInUse))
	}

	// Reconcile again, it should release the second batch
	t.Logf("Reconcile again - 2, Exepected free count = 20")
	err = poolmonitor.reconcile(context.Background())
	if err != nil {
		t.Errorf("Expected pool monitor to not fail after CNS set number of allocated IP's %v", err)
	}

	// Ensure the size of the requested spec is still the same
	if len(poolmonitor.nnc.Spec.IPsNotInUse) != initState.batchSize*2 {
		t.Fatalf("Expected IP's not in use is not correct, expected %v, actual %v", initState.batchSize*2,
			len(poolmonitor.nnc.Spec.IPsNotInUse))
	}

	// Ensure the request ipcount is now one batch size smaller than the inital IP count
	if poolmonitor.nnc.Spec.RequestedIPCount != int64(initState.ipConfigCount-(initState.batchSize*2)) {
		t.Fatalf("Expected pool size to be one batch size smaller after reconcile, expected %v, "+
			"actual %v", (initState.ipConfigCount - initState.batchSize), len(poolmonitor.nnc.Spec.IPsNotInUse))
	}

	t.Logf("Update Request Controller")
	err = fakerc.Reconcile(true)
	if err != nil {
		t.Error(err)
	}

	err = poolmonitor.reconcile(context.Background())
	if err != nil {
		t.Errorf("Expected no pool monitor failure after request controller reconcile: %v", err)
	}

	// Ensure the spec doesn't have any IPsNotInUse after request controller has reconciled
	if len(poolmonitor.nnc.Spec.IPsNotInUse) != 0 {
		t.Fatalf("Expected IP's not in use to be 0 after reconcile, expected %v, actual %v",
			(initState.ipConfigCount - initState.batchSize), len(poolmonitor.nnc.Spec.IPsNotInUse))
	}
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
	expectedRequestedIP := 16
	expectedDecreaseIP := int(initState.maxIPCount) % initState.batchSize
	fakecns, fakerc, poolmonitor := initFakes(initState)
	assert.NoError(t, fakerc.Reconcile(true))

	err := poolmonitor.reconcile(context.Background())
	if err != nil {
		t.Errorf("Expected pool monitor to not fail after CNS set number of allocated IP's %v", err)
	}

	// Trigger a batch release
	err = fakecns.SetNumberOfAllocatedIPs(5)
	if err != nil {
		t.Error(err)
	}

	err = poolmonitor.reconcile(context.Background())
	if err != nil {
		t.Errorf("Expected pool monitor to not fail after CNS set number of allocated IP's %v", err)
	}

	// Ensure poolmonitor asked for a multiple of batch size
	if poolmonitor.nnc.Spec.RequestedIPCount != int64(expectedRequestedIP) {
		t.Fatalf("Expected requested ips to be %v when scaling by 1 batch size down from %v "+
			"(max pod limit) but got %v", expectedRequestedIP, initState.maxIPCount,
			poolmonitor.nnc.Spec.RequestedIPCount)
	}

	// Ensure we minused by the mod result
	if len(poolmonitor.nnc.Spec.IPsNotInUse) != expectedDecreaseIP {
		t.Fatalf("Expected to decrease requested IPs by %v (max pod count mod batchsize) to "+
			"make the requested ip count a multiple of the batch size in the case of hitting "+
			"the max before scale down, but got %v", expectedDecreaseIP, len(poolmonitor.nnc.Spec.IPsNotInUse))
	}
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
	err := poolmonitor.reconcile(context.Background())
	if err != nil {
		t.Fatalf("Failed to allocate test ipconfigs with err: %v", err)
	}

	// Trigger a batch release
	err = fakecns.SetNumberOfAllocatedIPs(1)
	if err != nil {
		t.Error(err)
	}

	err = poolmonitor.reconcile(context.Background())
	if err != nil {
		t.Errorf("Expected pool monitor to not fail after CNS set number of allocated IP's %v", err)
	}

	// ensure pool monitor has only requested the max pod ip count
	if poolmonitor.nnc.Spec.RequestedIPCount != int64(initState.maxIPCount) {
		t.Fatalf("Pool monitor target IP count (%v) should be the node limit (%v) when the max "+
			"has been reached", poolmonitor.nnc.Spec.RequestedIPCount, initState.maxIPCount)
	}
}

func ReconcileAndValidate(ctx context.Context,
	t *testing.T,
	poolmonitor *Monitor,
	expectedRequestCount,
	expectedIpsNotInUse int) {
	err := poolmonitor.reconcile(context.Background())
	if err != nil {
		t.Errorf("Expected pool monitor to not fail after CNS set number of allocated IP's %v", err)
	}

	// Increased the new count to be 20
	if poolmonitor.nnc.Spec.RequestedIPCount != int64(expectedRequestCount) {
		t.Fatalf("RequestIPCount not same, expected %v, actual %v",
			expectedRequestCount,
			poolmonitor.nnc.Spec.RequestedIPCount)
	}

	// Ensure there is no pending release ips
	if len(poolmonitor.nnc.Spec.IPsNotInUse) != expectedIpsNotInUse {
		t.Fatalf("Expected IP's not in use, expected %v, actual %v",
			expectedIpsNotInUse,
			len(poolmonitor.nnc.Spec.IPsNotInUse))
	}
}
