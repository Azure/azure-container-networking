package ciliumpool

import (
	"context"
	"errors"
	"testing"
)

func TestAPIConflictRetryLimitAndCancellation(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		a, api, factoryErr := testAllocator()
		if factoryErr != nil {
			t.Fatal(factoryErr)
		}
		t.Cleanup(api.server.Close)
		api.conflicts = 10
		ctx, cancel := context.WithCancel(t.Context())
		wantErr, remaining := errConflictRetries, 2
		if canceled {
			cancel()
			wantErr, remaining = context.Canceled, 10
		}
		_, err := a.Add(ctx, Owner{Network: testNetwork, Sandbox: testSandbox, Interface: testIfName})
		cancel()
		if !errors.Is(err, wantErr) {
			t.Fatalf("Add error = %v; want %v", err, wantErr)
		}
		api.mu.Lock()
		conflicts := api.conflicts
		api.mu.Unlock()
		if conflicts != remaining {
			t.Fatalf("remaining conflicts = %d; want %d", conflicts, remaining)
		}
	}
}
