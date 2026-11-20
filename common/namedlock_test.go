package common

import (
	"fmt"
	"sync"
	"testing"
)

// Run with -race: acquiring one name while another is released must not
// access the lock map outside its mutex.
func TestNamedLockConcurrentNames(t *testing.T) {
	namedLock := InitNamedLock()
	counts := make([]int, 4)
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				name := fmt.Sprintf("nc-%d", i%len(counts))
				namedLock.LockAcquire(name)
				counts[i%len(counts)]++
				namedLock.LockRelease(name)
			}
		}(i)
	}
	wg.Wait()

	for i, count := range counts {
		if count != 1600 {
			t.Errorf("nc-%d: got %d increments, want 1600", i, count)
		}
	}
	if len(namedLock.lockMap) != 0 {
		t.Errorf("lock map has %d entries after all releases, want 0", len(namedLock.lockMap))
	}
}
