package nodesubnet_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns/nodesubnet"
	"github.com/Azure/azure-container-networking/nmagent"
)

// Mock client that simply tracks if refresh has been called
type TestClient struct {
	fetchCalled bool
}

// Mock refresh
func (c *TestClient) GetInterfaceIPInfo(_ context.Context) (nmagent.Interfaces, error) {
	c.fetchCalled = true
	return nmagent.Interfaces{}, nil
}

func TestRefreshSecondaryIPsIfNeeded(t *testing.T) {
	getTests := []struct {
		name       string
		shouldCall bool
		interval   time.Duration
	}{
		{
			"fetch called",
			true,
			-1 * time.Second, // Negative timeout to force refresh
		},
		{
			"no refresh needed",
			false,
			10 * time.Hour, // High timeout to avoid refresh
		},
	}

	clientPtr := &TestClient{}
	fetcher := nodesubnet.NewIPFetcher(clientPtr, 0)

	for _, test := range getTests {
		test := test
		t.Run(test.name, func(t *testing.T) { // Do not parallelize, as we are using a shared client
			fetcher.SetSecondaryIPQueryInterval(test.interval)
			ctx, cancel := testContext(t)
			defer cancel()
			clientPtr.fetchCalled = false
			_, err := fetcher.RefreshSecondaryIPsIfNeeded(ctx)

			if test.shouldCall {
				if err != nil && errors.Is(err, nodesubnet.ErrRefreshSkipped) {
					t.Error("refresh expected, but didn't happen")
				}

				checkErr(t, err, false)
			} else if err == nil || !errors.Is(err, nodesubnet.ErrRefreshSkipped) {
				t.Error("refresh not expected, but happened")
			}
		})
	}
}

// testContext creates a context from the provided testing.T that will be
// canceled if the test suite is terminated.
func testContext(t *testing.T) (context.Context, context.CancelFunc) {
	if deadline, ok := t.Deadline(); ok {
		return context.WithDeadline(context.Background(), deadline)
	}
	return context.WithCancel(context.Background())
}

// checkErr is an assertion of the presence or absence of an error
func checkErr(t *testing.T, err error, shouldErr bool) {
	t.Helper()
	if err != nil && !shouldErr {
		t.Fatal("unexpected error: err:", err)
	}

	if err == nil && shouldErr {
		t.Fatal("expected error but received none")
	}
}
