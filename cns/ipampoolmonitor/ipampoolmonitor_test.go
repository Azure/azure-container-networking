package ipampoolmonitor

import (
	"context"
	"log"
	"testing"

	"github.com/Azure/azure-container-networking/cns/fakes"
	"github.com/Azure/azure-container-networking/cns/logger"
	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
)

func initFakes(batchSize, initialIPConfigCount, requestThresholdPercent, releaseThresholdPercent int, maxPodIPCount int64) (*fakes.HTTPServiceFake, *fakes.RequestControllerFake, *CNSIPAMPoolMonitor) {
	logger.InitLogger("testlogs", 0, 0, "./")

	scalarUnits := nnc.Scaler{
		BatchSize:               int64(batchSize),
		RequestThresholdPercent: int64(requestThresholdPercent),
		ReleaseThresholdPercent: int64(releaseThresholdPercent),
		MaxIPCount:              int64(maxPodIPCount),
	}
	subnetaddresspace := "10.0.0.0/8"

	fakecns := fakes.NewHTTPServiceFake()
	fakerc := fakes.NewRequestControllerFake(fakecns, scalarUnits, subnetaddresspace, initialIPConfigCount)

	poolmonitor := NewCNSIPAMPoolMonitor(fakecns, fakerc)

	fakecns.PoolMonitor = poolmonitor

	fakerc.Reconcile()

	return fakecns, fakerc, poolmonitor
}

func TestPoolSizeIncrease(t *testing.T) {
	var (
		batchSize               = 10
		initialIPConfigCount    = 10
		requestThresholdPercent = 30
		releaseThresholdPercent = 150
		maxPodIPCount           = int64(30)
	)

	fakecns, fakerc, poolmonitor := initFakes(batchSize, initialIPConfigCount, requestThresholdPercent, releaseThresholdPercent, maxPodIPCount)

	// increase number of allocated IP's in CNS
	err := fakecns.SetNumberOfAllocatedIPs(8)
	if err != nil {
		t.Fatalf("Failed to allocate test ipconfigs with err: %v", err)
	}

	// When poolmonitor reconcile is called, trigger increase and cache goal state
	err = poolmonitor.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Failed to allocate test ipconfigs with err: %v", err)
	}

	// ensure pool monitor has reached quorum with cns
	if poolmonitor.cachedNNC.Spec.RequestedIPCount != int64(initialIPConfigCount+(1*batchSize)) {
		t.Fatalf("Pool monitor target IP count doesn't match CNS pool state after reconcile: %v, actual %v", poolmonitor.cachedNNC.Spec.RequestedIPCount, len(fakecns.GetPodIPConfigState()))
	}

	// request controller reconciles, carves new IP's from the test subnet and adds to CNS state
	err = fakerc.Reconcile()
	if err != nil {
		t.Fatalf("Failed to reconcile fake requestcontroller with err: %v", err)
	}

	// when poolmonitor reconciles again here, the IP count will be within the thresholds
	// so no CRD update and nothing pending
	err = poolmonitor.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Failed to reconcile pool monitor after request controller updates CNS state: %v", err)
	}

	// ensure pool monitor has reached quorum with cns
	if poolmonitor.cachedNNC.Spec.RequestedIPCount != int64(initialIPConfigCount+(1*batchSize)) {
		t.Fatalf("Pool monitor target IP count doesn't match CNS pool state after reconcile: %v, actual %v", poolmonitor.cachedNNC.Spec.RequestedIPCount, len(fakecns.GetPodIPConfigState()))
	}

	// make sure IPConfig state size reflects the new pool size
	if len(fakecns.GetPodIPConfigState()) != initialIPConfigCount+(1*batchSize) {
		t.Fatalf("CNS Pod IPConfig state count doesn't match, expected: %v, actual %v", len(fakecns.GetPodIPConfigState()), initialIPConfigCount+(1*batchSize))
	}

	t.Logf("Pool size %v, Target pool size %v, Allocated IP's %v, ", len(fakecns.GetPodIPConfigState()), poolmonitor.cachedNNC.Spec.RequestedIPCount, len(fakecns.GetAllocatedIPConfigs()))
}

func TestPoolIncreaseDoesntChangeWhenIncreaseIsAlreadyInProgress(t *testing.T) {
	var (
		batchSize               = 10
		initialIPConfigCount    = 10
		requestThresholdPercent = 30
		releaseThresholdPercent = 150
		maxPodIPCount           = int64(30)
	)

	fakecns, fakerc, poolmonitor := initFakes(batchSize, initialIPConfigCount, requestThresholdPercent, releaseThresholdPercent, maxPodIPCount)

	// increase number of allocated IP's in CNS
	err := fakecns.SetNumberOfAllocatedIPs(8)
	if err != nil {
		t.Fatalf("Failed to allocate test ipconfigs with err: %v", err)
	}

	// When poolmonitor reconcile is called, trigger increase and cache goal state
	err = poolmonitor.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Failed to allocate test ipconfigs with err: %v", err)
	}

	// increase number of allocated IP's in CNS, within allocatable size but still inside trigger threshold,
	err = fakecns.SetNumberOfAllocatedIPs(9)
	if err != nil {
		t.Fatalf("Failed to allocate test ipconfigs with err: %v", err)
	}

	// poolmonitor reconciles, but doesn't actually update the CRD, because there is already a pending update
	err = poolmonitor.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Failed to reconcile pool monitor after allocation ip increase with err: %v", err)
	}

	// ensure pool monitor has reached quorum with cns
	if poolmonitor.cachedNNC.Spec.RequestedIPCount != int64(initialIPConfigCount+(1*batchSize)) {
		t.Fatalf("Pool monitor target IP count doesn't match CNS pool state after reconcile: %v, actual %v", poolmonitor.cachedNNC.Spec.RequestedIPCount, len(fakecns.GetPodIPConfigState()))
	}

	// request controller reconciles, carves new IP's from the test subnet and adds to CNS state
	err = fakerc.Reconcile()
	if err != nil {
		t.Fatalf("Failed to reconcile fake requestcontroller with err: %v", err)
	}

	// when poolmonitor reconciles again here, the IP count will be within the thresholds
	// so no CRD update and nothing pending
	err = poolmonitor.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Failed to reconcile pool monitor after request controller updates CNS state: %v", err)
	}

	// make sure IPConfig state size reflects the new pool size
	if len(fakecns.GetPodIPConfigState()) != initialIPConfigCount+(1*batchSize) {
		t.Fatalf("CNS Pod IPConfig state count doesn't match, expected: %v, actual %v", len(fakecns.GetPodIPConfigState()), initialIPConfigCount+(1*batchSize))
	}

	// ensure pool monitor has reached quorum with cns
	if poolmonitor.cachedNNC.Spec.RequestedIPCount != int64(initialIPConfigCount+(1*batchSize)) {
		t.Fatalf("Pool monitor target IP count doesn't match CNS pool state after reconcile: %v, actual %v", poolmonitor.cachedNNC.Spec.RequestedIPCount, len(fakecns.GetPodIPConfigState()))
	}

	t.Logf("Pool size %v, Target pool size %v, Allocated IP's %v, ", len(fakecns.GetPodIPConfigState()), poolmonitor.cachedNNC.Spec.RequestedIPCount, len(fakecns.GetAllocatedIPConfigs()))
}

func TestPoolSizeIncreaseIdempotency(t *testing.T) {
	var (
		batchSize               = 10
		initialIPConfigCount    = 10
		requestThresholdPercent = 30
		releaseThresholdPercent = 150
		maxPodIPCount           = int64(30)
	)

	fakecns, _, poolmonitor := initFakes(batchSize, initialIPConfigCount, requestThresholdPercent, releaseThresholdPercent, maxPodIPCount)

	t.Logf("Minimum free IPs to request: %v", poolmonitor.MinimumFreeIps)
	t.Logf("Maximum free IPs to release: %v", poolmonitor.MaximumFreeIps)

	// increase number of allocated IP's in CNS
	err := fakecns.SetNumberOfAllocatedIPs(8)
	if err != nil {
		t.Fatalf("Failed to allocate test ipconfigs with err: %v", err)
	}

	// When poolmonitor reconcile is called, trigger increase and cache goal state
	err = poolmonitor.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Failed to allocate test ipconfigs with err: %v", err)
	}

	// ensure pool monitor has increased batch size
	if poolmonitor.cachedNNC.Spec.RequestedIPCount != int64(initialIPConfigCount+(1*batchSize)) {
		t.Fatalf("Pool monitor target IP count doesn't match CNS pool state after reconcile: %v, actual %v", poolmonitor.cachedNNC.Spec.RequestedIPCount, len(fakecns.GetPodIPConfigState()))
	}

	// reconcile pool monitor a second time, then verify requested ip count is still the same
	err = poolmonitor.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Failed to allocate test ipconfigs with err: %v", err)
	}

	// ensure pool monitor requested pool size is unchanged as request controller hasn't reconciled yet
	if poolmonitor.cachedNNC.Spec.RequestedIPCount != int64(initialIPConfigCount+(1*batchSize)) {
		t.Fatalf("Pool monitor target IP count doesn't match CNS pool state after reconcile: %v, actual %v", poolmonitor.cachedNNC.Spec.RequestedIPCount, len(fakecns.GetPodIPConfigState()))
	}
}

func TestPoolIncreasePastNodeLimit(t *testing.T) {
	var (
		batchSize               = 16
		initialIPConfigCount    = 16
		requestThresholdPercent = 50
		releaseThresholdPercent = 150
		maxPodIPCount           = int64(30)
	)

	fakecns, _, poolmonitor := initFakes(batchSize, initialIPConfigCount, requestThresholdPercent, releaseThresholdPercent, maxPodIPCount)

	t.Logf("Minimum free IPs to request: %v", poolmonitor.MinimumFreeIps)
	t.Logf("Maximum free IPs to release: %v", poolmonitor.MaximumFreeIps)

	// increase number of allocated IP's in CNS
	err := fakecns.SetNumberOfAllocatedIPs(9)
	if err != nil {
		t.Fatalf("Failed to allocate test ipconfigs with err: %v", err)
	}

	// When poolmonitor reconcile is called, trigger increase and cache goal state
	err = poolmonitor.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Failed to allocate test ipconfigs with err: %v", err)
	}

	// ensure pool monitor has only requested the max pod ip count
	if poolmonitor.cachedNNC.Spec.RequestedIPCount != maxPodIPCount {
		t.Fatalf("Pool monitor target IP count (%v) should be the node limit (%v) when the max has been reached", poolmonitor.cachedNNC.Spec.RequestedIPCount, maxPodIPCount)
	}
}

func TestPoolIncreaseBatchSizeGreaterThanMaxPodIPCount(t *testing.T) {
	var (
		batchSize               = 50
		initialIPConfigCount    = 16
		requestThresholdPercent = 50
		releaseThresholdPercent = 150
		maxPodIPCount           = int64(30)
	)

	fakecns, _, poolmonitor := initFakes(batchSize, initialIPConfigCount, requestThresholdPercent, releaseThresholdPercent, maxPodIPCount)

	t.Logf("Minimum free IPs to request: %v", poolmonitor.MinimumFreeIps)
	t.Logf("Maximum free IPs to release: %v", poolmonitor.MaximumFreeIps)

	// increase number of allocated IP's in CNS
	err := fakecns.SetNumberOfAllocatedIPs(16)
	if err != nil {
		t.Fatalf("Failed to allocate test ipconfigs with err: %v", err)
	}

	// When poolmonitor reconcile is called, trigger increase and cache goal state
	err = poolmonitor.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Failed to allocate test ipconfigs with err: %v", err)
	}

	// ensure pool monitor has only requested the max pod ip count
	if poolmonitor.cachedNNC.Spec.RequestedIPCount != maxPodIPCount {
		t.Fatalf("Pool monitor target IP count (%v) should be the node limit (%v) when the max has been reached", poolmonitor.cachedNNC.Spec.RequestedIPCount, maxPodIPCount)
	}
}

func TestPoolIncreaseMaxIPCountSetToZero(t *testing.T) {
	var (
		batchSize               = 16
		initialIPConfigCount    = 16
		requestThresholdPercent = 50
		releaseThresholdPercent = 150
		initialMaxPodIPCount    = int64(0)
		expectedMaxPodIPCount   = defaultMaxIPCount
	)

	_, _, poolmonitor := initFakes(batchSize, initialIPConfigCount, requestThresholdPercent, releaseThresholdPercent, initialMaxPodIPCount)

	if poolmonitor.getMaxIPCount() != expectedMaxPodIPCount {
		t.Fatalf("Pool monitor target IP count (%v) should be the node limit (%v) when the MaxIPCount field in the CRD is zero", poolmonitor.getMaxIPCount(), expectedMaxPodIPCount)
	}
}

func TestPoolDecrease(t *testing.T) {
	var (
		batchSize               = 10
		initialIPConfigCount    = 20
		requestThresholdPercent = 30
		releaseThresholdPercent = 150
		maxPodIPCount           = int64(30)
	)

	fakecns, fakerc, poolmonitor := initFakes(batchSize, initialIPConfigCount, requestThresholdPercent, releaseThresholdPercent, maxPodIPCount)

	log.Printf("Min free IP's %v", poolmonitor.MinimumFreeIps)
	log.Printf("Max free IP %v", poolmonitor.MaximumFreeIps)

	// initial pool count is 20, set 15 of them to be allocated
	err := fakecns.SetNumberOfAllocatedIPs(15)
	if err != nil {
		t.Fatal(err)
	}

	// Pool monitor does nothing, as the current number of IP's falls in the threshold
	err = poolmonitor.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Decrease the number of allocated IP's down to 5. This should trigger a scale down
	err = fakecns.SetNumberOfAllocatedIPs(4)
	if err != nil {
		t.Fatal(err)
	}

	// Pool monitor will adjust the spec so the pool size will be 1 batch size smaller
	err = poolmonitor.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// ensure that the adjusted spec is smaller than the initial pool size
	if len(poolmonitor.cachedNNC.Spec.IPsNotInUse) != (initialIPConfigCount - batchSize) {
		t.Fatalf("Expected pool size to be one batch size smaller after reconcile, expected %v, actual %v", (initialIPConfigCount - batchSize), len(poolmonitor.cachedNNC.Spec.IPsNotInUse))
	}

	// reconcile the fake request controller
	err = fakerc.Reconcile()
	if err != nil {
		t.Fatal(err)
	}

	// Ensure the size of the requested spec is still the same
	if len(poolmonitor.cachedNNC.Spec.IPsNotInUse) != 0 {
		t.Fatalf("Expected IPsNotInUse to be 0 after request controller reconcile, actual %v", poolmonitor.cachedNNC.Spec.IPsNotInUse)
	}

	return
}

func TestPoolSizeDecreaseWhenDecreaseHasAlreadyBeenRequested(t *testing.T) {
	var (
		batchSize               = 10
		initialIPConfigCount    = 20
		requestThresholdPercent = 30
		releaseThresholdPercent = 100
		maxPodIPCount           = int64(30)
	)

	fakecns, fakerc, poolmonitor := initFakes(batchSize, initialIPConfigCount, requestThresholdPercent, releaseThresholdPercent, maxPodIPCount)

	log.Printf("Min free IP's %v", poolmonitor.MinimumFreeIps)
	log.Printf("Max free IP %v", poolmonitor.MaximumFreeIps)

	// initial pool count is 30, set 25 of them to be allocated
	err := fakecns.SetNumberOfAllocatedIPs(5)
	if err != nil {
		t.Error(err)
	}

	// Pool monitor does nothing, as the current number of IP's falls in the threshold
	err = poolmonitor.Reconcile(context.Background())
	if err != nil {
		t.Errorf("Expected pool monitor to not fail after CNS set number of allocated IP's %v", err)
	}

	// Ensure the size of the requested spec is still the same
	if len(poolmonitor.cachedNNC.Spec.IPsNotInUse) != (initialIPConfigCount - batchSize) {
		t.Fatalf("Expected IP's not in use be one batch size smaller after reconcile, expected %v, actual %v", (initialIPConfigCount - batchSize), len(poolmonitor.cachedNNC.Spec.IPsNotInUse))
	}

	// Ensure the request ipcount is now one batch size smaller than the inital IP count
	if poolmonitor.cachedNNC.Spec.RequestedIPCount != int64(initialIPConfigCount-batchSize) {
		t.Fatalf("Expected pool size to be one batch size smaller after reconcile, expected %v, actual %v", (initialIPConfigCount - batchSize), len(poolmonitor.cachedNNC.Spec.IPsNotInUse))
	}

	// Update pods with IP count, ensure pool monitor stays the same until request controller reconciles
	err = fakecns.SetNumberOfAllocatedIPs(6)
	if err != nil {
		t.Error(err)
	}

	// Ensure the size of the requested spec is still the same
	if len(poolmonitor.cachedNNC.Spec.IPsNotInUse) != (initialIPConfigCount - batchSize) {
		t.Fatalf("Expected IP's not in use to be one batch size smaller after reconcile, and not change after reconcile, expected %v, actual %v", (initialIPConfigCount - batchSize), len(poolmonitor.cachedNNC.Spec.IPsNotInUse))
	}

	// Ensure the request ipcount is now one batch size smaller than the inital IP count
	if poolmonitor.cachedNNC.Spec.RequestedIPCount != int64(initialIPConfigCount-batchSize) {
		t.Fatalf("Expected pool size to be one batch size smaller after reconcile, and not change after existing call, expected %v, actual %v", (initialIPConfigCount - batchSize), len(poolmonitor.cachedNNC.Spec.IPsNotInUse))
	}

	err = fakerc.Reconcile()
	if err != nil {
		t.Error(err)
	}

	err = poolmonitor.Reconcile(context.Background())
	if err != nil {
		t.Errorf("Expected no pool monitor failure after request controller reconcile: %v", err)
	}

	// Ensure the spec doesn't have any IPsNotInUse after request controller has reconciled
	if len(poolmonitor.cachedNNC.Spec.IPsNotInUse) != 0 {
		t.Fatalf("Expected IP's not in use to be 0 after reconcile, expected %v, actual %v", (initialIPConfigCount - batchSize), len(poolmonitor.cachedNNC.Spec.IPsNotInUse))
	}
}

func TestPoolSizeDecreaseToReallyLow(t *testing.T) {
	var (
		batchSize               = 10
		initialIPConfigCount    = 30
		requestThresholdPercent = 30
		releaseThresholdPercent = 100
		maxPodIPCount           = int64(30)
	)

	fakecns, fakerc, poolmonitor := initFakes(batchSize, initialIPConfigCount, requestThresholdPercent, releaseThresholdPercent, maxPodIPCount)

	log.Printf("Min free IP's %v", poolmonitor.MinimumFreeIps)
	log.Printf("Max free IP %v", poolmonitor.MaximumFreeIps)

	// initial pool count is 30, set 23 of them to be allocated
	err := fakecns.SetNumberOfAllocatedIPs(23)
	if err != nil {
		t.Error(err)
	}

	// Pool monitor does nothing, as the current number of IP's falls in the threshold
	err = poolmonitor.Reconcile(context.Background())
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
	err = poolmonitor.Reconcile(context.Background())
	if err != nil {
		t.Errorf("Expected pool monitor to not fail after CNS set number of allocated IP's %v", err)
	}

	// Ensure the size of the requested spec is still the same
	if len(poolmonitor.cachedNNC.Spec.IPsNotInUse) != batchSize {
		t.Fatalf("Expected IP's not in use is not correct, expected %v, actual %v", batchSize, len(poolmonitor.cachedNNC.Spec.IPsNotInUse))
	}

	// Ensure the request ipcount is now one batch size smaller than the inital IP count
	if poolmonitor.cachedNNC.Spec.RequestedIPCount != int64(initialIPConfigCount-batchSize) {
		t.Fatalf("Expected pool size to be one batch size smaller after reconcile, expected %v, actual %v", (initialIPConfigCount - batchSize), len(poolmonitor.cachedNNC.Spec.IPsNotInUse))
	}

	// Reconcile again, it should release the second batch
	t.Logf("Reconcile again - 2, Exepected free count = 20")
	err = poolmonitor.Reconcile(context.Background())
	if err != nil {
		t.Errorf("Expected pool monitor to not fail after CNS set number of allocated IP's %v", err)
	}

	// Ensure the size of the requested spec is still the same
	if len(poolmonitor.cachedNNC.Spec.IPsNotInUse) != batchSize*2 {
		t.Fatalf("Expected IP's not in use is not correct, expected %v, actual %v", batchSize*2, len(poolmonitor.cachedNNC.Spec.IPsNotInUse))
	}

	// Ensure the request ipcount is now one batch size smaller than the inital IP count
	if poolmonitor.cachedNNC.Spec.RequestedIPCount != int64(initialIPConfigCount-(batchSize*2)) {
		t.Fatalf("Expected pool size to be one batch size smaller after reconcile, expected %v, actual %v", (initialIPConfigCount - batchSize), len(poolmonitor.cachedNNC.Spec.IPsNotInUse))
	}

	t.Logf("Update Request Controller")
	err = fakerc.Reconcile()
	if err != nil {
		t.Error(err)
	}

	err = poolmonitor.Reconcile(context.Background())
	if err != nil {
		t.Errorf("Expected no pool monitor failure after request controller reconcile: %v", err)
	}

	// Ensure the spec doesn't have any IPsNotInUse after request controller has reconciled
	if len(poolmonitor.cachedNNC.Spec.IPsNotInUse) != 0 {
		t.Fatalf("Expected IP's not in use to be 0 after reconcile, expected %v, actual %v", (initialIPConfigCount - batchSize), len(poolmonitor.cachedNNC.Spec.IPsNotInUse))
	}
}

func TestDecreaseAfterNodeLimitReached(t *testing.T) {
	var (
		batchSize               = 16
		initialIPConfigCount    = 30
		requestThresholdPercent = 50
		releaseThresholdPercent = 150
		maxPodIPCount           = int64(30)
		expectedRequestedIP     = 16
		expectedDecreaseIP      = int(maxPodIPCount) % batchSize
	)

	fakecns, _, poolmonitor := initFakes(batchSize, initialIPConfigCount, requestThresholdPercent, releaseThresholdPercent, maxPodIPCount)

	t.Logf("Minimum free IPs to request: %v", poolmonitor.MinimumFreeIps)
	t.Logf("Maximum free IPs to release: %v", poolmonitor.MaximumFreeIps)

	err := fakecns.SetNumberOfAllocatedIPs(20)
	if err != nil {
		t.Error(err)
	}

	err = poolmonitor.Reconcile(context.Background())
	if err != nil {
		t.Errorf("Expected pool monitor to not fail after CNS set number of allocated IP's %v", err)
	}

	// Trigger a batch release
	err = fakecns.SetNumberOfAllocatedIPs(5)
	if err != nil {
		t.Error(err)
	}

	err = poolmonitor.Reconcile(context.Background())
	if err != nil {
		t.Errorf("Expected pool monitor to not fail after CNS set number of allocated IP's %v", err)
	}

	// Ensure poolmonitor asked for a multiple of batch size
	if poolmonitor.cachedNNC.Spec.RequestedIPCount != int64(expectedRequestedIP) {
		t.Fatalf("Expected requested ips to be %v when scaling by 1 batch size down from %v (max pod limit) but got %v", expectedRequestedIP, maxPodIPCount, poolmonitor.cachedNNC.Spec.RequestedIPCount)
	}

	// Ensure we minused by the mod result
	if len(poolmonitor.cachedNNC.Spec.IPsNotInUse) != expectedDecreaseIP {
		t.Fatalf("Expected to decrease requested IPs by %v (max pod count mod batchsize) to make the requested ip count a multiple of the batch size in the case of hitting the max before scale down, but got %v", expectedDecreaseIP, len(poolmonitor.cachedNNC.Spec.IPsNotInUse))
	}
}

func TestPoolDecreaseBatchSizeGreaterThanMaxPodIPCount(t *testing.T) {
	var (
		batchSize               = 31
		initialIPConfigCount    = 30
		requestThresholdPercent = 50
		releaseThresholdPercent = 150
		maxPodIPCount           = int64(30)
	)

	fakecns, _, poolmonitor := initFakes(batchSize, initialIPConfigCount, requestThresholdPercent, releaseThresholdPercent, maxPodIPCount)

	t.Logf("Minimum free IPs to request: %v", poolmonitor.MinimumFreeIps)
	t.Logf("Maximum free IPs to release: %v", poolmonitor.MaximumFreeIps)

	// increase number of allocated IP's in CNS
	err := fakecns.SetNumberOfAllocatedIPs(30)
	if err != nil {
		t.Fatalf("Failed to allocate test ipconfigs with err: %v", err)
	}

	// When poolmonitor reconcile is called, trigger increase and cache goal state
	err = poolmonitor.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Failed to allocate test ipconfigs with err: %v", err)
	}

	// Trigger a batch release
	err = fakecns.SetNumberOfAllocatedIPs(1)
	if err != nil {
		t.Error(err)
	}

	err = poolmonitor.Reconcile(context.Background())
	if err != nil {
		t.Errorf("Expected pool monitor to not fail after CNS set number of allocated IP's %v", err)
	}

	// ensure pool monitor has only requested the max pod ip count
	if poolmonitor.cachedNNC.Spec.RequestedIPCount != maxPodIPCount {
		t.Fatalf("Pool monitor target IP count (%v) should be the node limit (%v) when the max has been reached", poolmonitor.cachedNNC.Spec.RequestedIPCount, maxPodIPCount)
	}
}
