package ipampoolmonitor

import (
	"log"
	"testing"

	"github.com/Azure/azure-container-networking/cns/fakes"
	"github.com/Azure/azure-container-networking/cns/logger"
	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
)

func initFakes(batchSize, initialIPConfigCount, requestThresholdPercent, releaseThresholdPercent int) (*fakes.HTTPServiceFake, *fakes.RequestControllerFake, *CNSIPAMPoolMonitor) {
	logger.InitLogger("testlogs", 0, 0, "./")

	scalarUnits := nnc.Scaler{
		BatchSize:               int64(batchSize),
		RequestThresholdPercent: int64(requestThresholdPercent),
		ReleaseThresholdPercent: int64(releaseThresholdPercent),
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
	)

	fakecns, fakerc, poolmonitor := initFakes(batchSize, initialIPConfigCount, requestThresholdPercent, releaseThresholdPercent)

	// increase number of allocated IP's in CNS
	err := fakecns.SetNumberOfAllocatedIPs(8)
	if err != nil {
		t.Fatalf("Failed to allocate test ipconfigs with err: %v", err)
	}

	// When poolmonitor reconcile is called, trigger increase and cache goal state
	err = poolmonitor.Reconcile()
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
	err = poolmonitor.Reconcile()
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
	)

	fakecns, fakerc, poolmonitor := initFakes(batchSize, initialIPConfigCount, requestThresholdPercent, releaseThresholdPercent)

	// increase number of allocated IP's in CNS
	err := fakecns.SetNumberOfAllocatedIPs(8)
	if err != nil {
		t.Fatalf("Failed to allocate test ipconfigs with err: %v", err)
	}

	// When poolmonitor reconcile is called, trigger increase and cache goal state
	err = poolmonitor.Reconcile()
	if err != nil {
		t.Fatalf("Failed to allocate test ipconfigs with err: %v", err)
	}

	// increase number of allocated IP's in CNS, within allocatable size but still inside trigger threshold,
	err = fakecns.SetNumberOfAllocatedIPs(9)
	if err != nil {
		t.Fatalf("Failed to allocate test ipconfigs with err: %v", err)
	}

	// poolmonitor reconciles, but doesn't actually update the CRD, because there is already a pending update
	err = poolmonitor.Reconcile()
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
	err = poolmonitor.Reconcile()
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
	)

	fakecns, _, poolmonitor := initFakes(batchSize, initialIPConfigCount, requestThresholdPercent, releaseThresholdPercent)

	t.Logf("Minimum free IPs to request: %v", poolmonitor.MinimumFreeIps)
	t.Logf("Maximum free IPs to release: %v", poolmonitor.MaximumFreeIps)

	// increase number of allocated IP's in CNS
	err := fakecns.SetNumberOfAllocatedIPs(8)
	if err != nil {
		t.Fatalf("Failed to allocate test ipconfigs with err: %v", err)
	}

	// When poolmonitor reconcile is called, trigger increase and cache goal state
	err = poolmonitor.Reconcile()
	if err != nil {
		t.Fatalf("Failed to allocate test ipconfigs with err: %v", err)
	}

	// ensure pool monitor has increased batch size
	if poolmonitor.cachedNNC.Spec.RequestedIPCount != int64(initialIPConfigCount+(1*batchSize)) {
		t.Fatalf("Pool monitor target IP count doesn't match CNS pool state after reconcile: %v, actual %v", poolmonitor.cachedNNC.Spec.RequestedIPCount, len(fakecns.GetPodIPConfigState()))
	}

	// reconcile pool monitor a second time, then verify requested ip count is still the same
	err = poolmonitor.Reconcile()
	if err != nil {
		t.Fatalf("Failed to allocate test ipconfigs with err: %v", err)
	}

	// ensure pool monitor requested pool size is unchanged as request controller hasn't reconciled yet
	if poolmonitor.cachedNNC.Spec.RequestedIPCount != int64(initialIPConfigCount+(1*batchSize)) {
		t.Fatalf("Pool monitor target IP count doesn't match CNS pool state after reconcile: %v, actual %v", poolmonitor.cachedNNC.Spec.RequestedIPCount, len(fakecns.GetPodIPConfigState()))
	}
}

func TestPoolDecrease(t *testing.T) {
	var (
		batchSize               = 10
		initialIPConfigCount    = 20
		requestThresholdPercent = 30
		releaseThresholdPercent = 150
	)

	fakecns, fakerc, poolmonitor := initFakes(batchSize, initialIPConfigCount, requestThresholdPercent, releaseThresholdPercent)

	log.Printf("Min free IP's %v", poolmonitor.MinimumFreeIps)
	log.Printf("Max free IP %v", poolmonitor.MaximumFreeIps)

	// initial pool count is 20, set 15 of them to be allocated
	err := fakecns.SetNumberOfAllocatedIPs(15)
	if err != nil {
		t.Fatal(err)
	}

	// Pool monitor does nothing, as the current number of IP's falls in the threshold
	err = poolmonitor.Reconcile()
	if err != nil {
		t.Fatal(err)
	}

	// Decrease the number of allocated IP's down to 5. This should trigger a scale down
	err = fakecns.SetNumberOfAllocatedIPs(4)
	if err != nil {
		t.Fatal(err)
	}

	// Pool monitor will adjust the spec so the pool size will be 1 batch size smaller
	err = poolmonitor.Reconcile()
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
	)

	fakecns, fakerc, poolmonitor := initFakes(batchSize, initialIPConfigCount, requestThresholdPercent, releaseThresholdPercent)

	log.Printf("Min free IP's %v", poolmonitor.MinimumFreeIps)
	log.Printf("Max free IP %v", poolmonitor.MaximumFreeIps)

	// initial pool count is 30, set 25 of them to be allocated
	err := fakecns.SetNumberOfAllocatedIPs(5)
	if err != nil {
		t.Error(err)
	}

	// Pool monitor does nothing, as the current number of IP's falls in the threshold
	err = poolmonitor.Reconcile()
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

	err = poolmonitor.Reconcile()
	if err != nil {
		t.Errorf("Expected no pool monitor failure after request controller reconcile: %v", err)
	}

	// Ensure the spec doesn't have any IPsNotInUse after request controller has reconciled
	if len(poolmonitor.cachedNNC.Spec.IPsNotInUse) != 0 {
		t.Fatalf("Expected IP's not in use to be 0 after reconcile, expected %v, actual %v", (initialIPConfigCount - batchSize), len(poolmonitor.cachedNNC.Spec.IPsNotInUse))
	}
}
