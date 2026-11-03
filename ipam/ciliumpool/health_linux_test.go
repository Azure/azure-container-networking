package ciliumpool

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/ipam"
	"github.com/Azure/azure-container-networking/ipam/ciliumpool/persistence"
	"github.com/Azure/azure-container-networking/store"
	"go.uber.org/zap"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	ktesting "k8s.io/client-go/testing"
)

var errHealthTest = errors.New("observation failed")

func TestHealthLeaseAndObservationFreshness(t *testing.T) {
	now := time.Now()
	m := NewMonitor(8)
	m.now = func() time.Time { return now }
	if got := m.Snapshot().Admission.State; got != HealthUnknown {
		t.Fatalf("startup admission = %s", got)
	}
	lease := now.Add(10 * time.Second)
	state := ipam.CiliumPoolStatus{State: ipam.CiliumPoolState{ReadyUntil: lease, BootID: "boot"}, Free: 4}
	m.setLocal(state, "boot")
	m.Observe("storage", time.Millisecond, nil)
	m.Observe("guest", time.Millisecond, nil)
	m.Observe("usage", time.Millisecond, nil)
	if got := m.Snapshot().Admission.State; got != HealthHealthy {
		t.Fatalf("valid lease = %s", got)
	}
	now = now.Add(time.Second)
	m.Observe("guest", time.Millisecond, errHealthTest)
	if s := m.Snapshot(); s.Admission.State != HealthHealthy || !s.Observations["guest"].LastSuccess.Equal(now.Add(-time.Second)) {
		t.Fatalf("failed guest observation changed lease or success timestamp: %+v", s)
	}
	now = lease.Add(time.Nanosecond)
	response := httptest.NewRecorder()
	m.Handler().ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/readyz", http.NoBody))
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "lease_expired") {
		t.Fatalf("expired readiness = %d %s", response.Code, response.Body)
	}
	now = now.Add(observationTTL)
	if s := m.Snapshot(); s.Admission.State != HealthUnknown || s.Observations["guest"].State != HealthUnknown {
		t.Fatalf("stale observations appear current: %+v", s)
	}
	response = httptest.NewRecorder()
	m.Handler().ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/healthz", http.NoBody))
	if response.Code != http.StatusOK {
		t.Fatalf("dependency failure changed process liveness: %d", response.Code)
	}
}

func TestHealthStorageAndFenceRestoration(t *testing.T) {
	b, cn, node, guest := fixture(t)
	id, err := identity(b.Config, node, cn, guest)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = b.State.Synchronize(t.Context(), id, nil, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = b.State.Fence(t.Context(), "network state missing"); err != nil {
		t.Fatal(err)
	}
	state, boot, err := readLocalStatus(b.Config, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	m := NewMonitor(8)
	m.setLocal(state, boot)
	m.Observe("storage", time.Millisecond, nil)
	if s := m.Snapshot(); s.Admission.Reason != "fenced" || s.Local.State.FenceReason != "network state missing" {
		t.Fatalf("fence was lost on restoration: %+v", s)
	}
	m.Observe("storage", time.Millisecond, errHealthTest)
	if s := m.Snapshot(); s.Admission.State != HealthUnknown || s.Local.State.FenceReason == "" {
		t.Fatalf("storage failure invented empty state: %+v", s)
	}
	cfg := &common.CiliumPoolConfig{StateDir: t.TempDir()}
	if _, _, err = readLocalStatus(cfg, zap.NewNop()); err == nil {
		t.Fatal("missing database reported healthy")
	}
	if _, err = os.Stat(cfg.StatePath("azure-vnet-ipam")); !os.IsNotExist(err) {
		t.Fatalf("observation created a database: %v", err)
	}
}

func TestHealthMetricsSeparateFailuresAndConfirmation(t *testing.T) {
	m := NewMonitor(8)
	m.Observe("usage", time.Millisecond, nil)
	m.Observe("node_api", time.Millisecond, apierrors.NewUnauthorized("expired credential"))
	m.Observe("pool_api", time.Millisecond, apierrors.NewForbidden(schema.GroupResource{Resource: "ciliumnodes"}, "node", errHealthTest))
	response := httptest.NewRecorder()
	m.Handler().ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", http.NoBody))
	body := response.Body.String()
	for _, want := range []string{
		`azure_vnet_pool_errors_total{operation="node_api",reason="unauthorized"} 1`,
		`azure_vnet_pool_errors_total{operation="pool_api",reason="forbidden"} 1`,
		`azure_vnet_pool_last_success_timestamp_seconds{stage="publication"} 0`,
		`azure_vnet_pool_credential_expiry_known 0`,
		`azure_vnet_pool_policy_known 0`,
		`azure_vnet_pool_dataplane_known 0`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing metric %s", want)
		}
	}
	if response.Code != http.StatusOK {
		t.Fatalf("metrics failed: %d %s", response.Code, body)
	}
	if !m.Snapshot().Observations["publication"].LastSuccess.IsZero() {
		t.Fatal("usage confirmation fabricated a publication")
	}
}

func TestHealthDatabaseObservations(t *testing.T) {
	b, cn, node, guest := fixture(t)
	b.Monitor = NewMonitor(8)
	state := b.State.(*persistence.LocalState)
	state.Observe = func(operation string, duration time.Duration, err error) {
		b.Monitor.Observe("database_"+operation, duration, err)
	}
	id, err := identity(b.Config, node, cn, guest)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = b.State.Synchronize(t.Context(), id, nil, true, false); err != nil {
		t.Fatal(err)
	}
	if err = b.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	b.Monitor.Handler().ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", http.NoBody))
	for _, operation := range []string{"database_lock", "database_write"} {
		series := `azure_vnet_pool_operation_duration_seconds_count{operation="` + operation + `",outcome="success"}`
		if body := response.Body.String(); !strings.Contains(body, series) || strings.Contains(body, series+" 0\n") {
			t.Errorf("missing successful %s observations: %s", operation, body)
		}
	}
}

func TestHealthGuestFailureDoesNotHideLocalLease(t *testing.T) {
	b, cn, node, guest := fixture(t)
	b.Monitor = NewMonitor(8)
	id, err := identity(b.Config, node, cn, guest)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = b.State.Synchronize(t.Context(), id, nil, true, true); err != nil {
		t.Fatal(err)
	}
	b.Discover = func(context.Context, *common.CiliumPoolConfig) (Guest, error) {
		return Guest{}, errHealthTest
	}
	if err = b.Reconcile(t.Context()); err == nil {
		t.Fatal("guest failure was ignored")
	}
	ctx, cancel := context.WithCancel(t.Context())
	err = b.Monitor.RunDiagnostics(ctx, b.Config, func() (time.Time, error) {
		cancel()
		return time.Time{}, nil
	}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	s := b.Monitor.Snapshot()
	if s.Admission.State != HealthHealthy || s.Observations["guest"].State != HealthUnhealthy {
		t.Fatalf("independent health observations = %+v", s)
	}
}

func TestHealthAPIFailureKeepsIndependentGuestObservation(t *testing.T) {
	b, _, _, _ := fixture(t)
	b.Monitor = NewMonitor(8)
	client := b.Client.(*dynamicfake.FakeDynamicClient)
	client.PrependReactor("get", "nodes", func(ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewUnauthorized("credential rejected")
	})
	if err := b.Reconcile(t.Context()); !apierrors.IsUnauthorized(err) {
		t.Fatalf("reconcile error = %v", err)
	}
	s := b.Monitor.Snapshot()
	if s.Observations["guest"].State != HealthHealthy ||
		s.Observations["node_api"].State != HealthUnhealthy ||
		!s.Observations["node_api"].LastSuccess.IsZero() ||
		s.Observations["usage"].State != HealthUnknown {
		t.Fatalf("API failure produced misleading observations: %+v", s)
	}
}

func TestHealthServerShutdown(t *testing.T) {
	listenConfig := net.ListenConfig{}
	listener, err := listenConfig.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	m := NewMonitor(8)
	done := make(chan error, 1)
	go func() { done <- m.Serve(ctx, listener) }()
	client := &http.Client{Timeout: time.Second}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+listener.Addr().String()+"/status", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_, readErr := io.Copy(io.Discard, response.Body)
	closeErr := response.Body.Close()
	if err = errors.Join(readErr, closeErr); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(7 * time.Second):
		t.Fatal("health server did not stop")
	}
}

func TestHealthConcurrentScrapesAndObservations(t *testing.T) {
	m := NewMonitor(8)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 50 {
			m.Observe("guest", time.Millisecond, nil)
			m.setPool(8)
		}
	}()
	for range 50 {
		if _, err := m.registry.Gather(); err != nil {
			t.Fatal(err)
		}
		_ = m.Snapshot()
	}
	<-done
}

func TestHealthReadStatusCounts(t *testing.T) {
	b, cn, node, guest := fixture(t)
	id, err := identity(b.Config, node, cn, guest)
	if err != nil {
		t.Fatal(err)
	}
	supply, _, err := eligible(cn, id, guest)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = b.State.Synchronize(t.Context(), id, supply, true, true); err != nil {
		t.Fatal(err)
	}
	reserve(t, b, "pod", supply[0].Address.String())
	kvs, err := lockTestStore(b.Config.StatePath("azure-vnet-ipam"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if unlockErr := kvs.Unlock(); unlockErr != nil {
			t.Errorf("unlocking IPAM state: %v", unlockErr)
		}
	}()
	if _, _, err = ipam.CiliumPoolTransaction(kvs, id, supply[1:], false, false); err != nil {
		t.Fatal(err)
	}
	status, err := ipam.ReadCiliumPoolStatus(kvs)
	if err != nil {
		t.Fatal(err)
	}
	if status.Free != 1 || status.Used != 1 || status.Quarantined != 1 || status.Eligible != 1 {
		t.Fatalf("incorrect address counts: %+v", status)
	}
	if err = kvs.Write("IPAM", "invalid"); err != nil {
		t.Fatal(err)
	}
	if _, err = ipam.ReadCiliumPoolStatus(kvs); err == nil || errors.Is(err, store.ErrKeyNotFound) {
		t.Fatalf("corruption returned empty status: %v", err)
	}
}
