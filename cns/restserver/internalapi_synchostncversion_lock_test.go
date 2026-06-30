package restserver

// Regression tests for the SyncHostNCVersion locking discipline.
//
// SyncHostNCVersion must not hold the HTTPRestService lock across its NMAgent and
// IMDS calls. Every IPAM request handler takes the same lock, so holding it across
// that network I/O lets a slow or unresponsive NMAgent/wireserver block all IPAM
// request handling (CNI Add) until the call returns.

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/fakes"
	nma "github.com/Azure/azure-container-networking/nmagent"
)

// TestSyncHostNCVersionDoesNotHoldLockDuringNMAgentCall verifies that the service
// lock is available to other callers (such as IPAM request handlers) while
// SyncHostNCVersion is waiting on the NMAgent call.
func TestSyncHostNCVersionDoesNotHoldLockDuringNMAgentCall(t *testing.T) {
	// Sets up the test service with one NC whose programmed (host) version is behind
	// the DNC version, so syncHostNCVersion treats it as outdated and calls NMAgent.
	req := createNCReqeustForSyncHostNCVersion(t)

	entered := make(chan struct{}) // closed once we are inside the NMAgent call
	release := make(chan struct{}) // closing this lets the NMAgent call return

	mnma := &fakes.NMAgentClientFake{
		GetNCVersionListF: func(_ context.Context) (nma.NCVersionList, error) {
			close(entered)
			<-release // block, simulating a slow/unresponsive NMAgent
			return nma.NCVersionList{
				Containers: []nma.NCVersion{{NetworkContainerID: req.NetworkContainerid, Version: "0"}},
			}, nil
		},
	}
	cleanup := setMockNMAgent(svc, mnma)
	defer cleanup()
	defer close(release)

	syncReturned := make(chan struct{})
	go func() {
		svc.SyncHostNCVersion(context.Background(), cns.CRD)
		close(syncReturned)
	}()

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("SyncHostNCVersion never reached the NMAgent call; test setup is wrong")
	}

	// An IPAM request handler would take the service lock; a bare RLock stands in for it.
	ipamProceeded := make(chan struct{})
	go func() {
		svc.RLock()
		svc.RUnlock()
		close(ipamProceeded)
	}()

	select {
	case <-ipamProceeded:
		// expected: the lock is not held across the NMAgent call
	case <-time.After(5 * time.Second):
		t.Fatal("service lock unavailable while SyncHostNCVersion is in the NMAgent call: lock is held across NMAgent/IMDS I/O")
	}

	// The sync goroutine is still parked in the (blocked) NMAgent call, proving the
	// IPAM lock above was acquired concurrently with an in-flight NMAgent request.
	select {
	case <-syncReturned:
		t.Fatal("SyncHostNCVersion returned before release; the NMAgent mock did not block as intended")
	default:
	}
}

// TestNMAgentClientHonorsContext verifies that the nmagent client (which sets no
// client-level HTTP timeout) aborts a request when the supplied context deadline
// fires, against a server that accepts the connection but never responds.
func TestNMAgentClientHonorsContext(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			defer c.Close() //nolint:revive // hold the conn open, never respond
		}
	}()

	host, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)
	client, err := nma.NewClient(nma.Config{Host: host, Port: uint16(port)})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	done := make(chan time.Duration, 1)
	go func() {
		start := time.Now()
		_, _ = client.GetNCVersionList(ctx)
		done <- time.Since(start)
	}()

	select {
	case elapsed := <-done:
		if elapsed > 5*time.Second {
			t.Errorf("context not honored: call ran %v past a 1s deadline", elapsed)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("context not honored: GetNCVersionList did not return after its deadline")
	}
}
