package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Azure/azure-container-networking/npm"
	"github.com/Azure/azure-container-networking/npm/http/api"
	"github.com/Azure/azure-container-networking/npm/pkg/controlplane/controllers/common"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetNPMCacheHandler(t *testing.T) {
	assert := assert.New(t)

	nodeName := "nodename"
	npmCacheEncoder := npm.CacheEncoder(nodeName)
	n := &NPMRestServer{}
	handler := n.npmCacheHandler(npmCacheEncoder)

	req, err := http.NewRequest(http.MethodGet, api.NPMMgrPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	byteArray, err := io.ReadAll(rr.Body)
	if err != nil {
		t.Errorf("failed to read response's data : %v", err)
	}

	actual := &common.Cache{}
	err = json.Unmarshal(byteArray, actual)
	if err != nil {
		t.Fatalf("failed to unmarshal %s due to %v", string(byteArray), err)
	}

	expected := &common.Cache{
		NodeName: nodeName,
		NsMap:    make(map[string]*common.Namespace),
		PodMap:   make(map[string]*common.NpmPod),
		ListMap:  make(map[string]string),
		SetMap:   make(map[string]string),
	}

	assert.Exactly(expected, actual)
}

// blockingMarshaler blocks inside MarshalJSON until released, so a test can hold cache
// encodings in flight and observe what happens to further requests.
type blockingMarshaler struct {
	entered chan struct{}
	release chan struct{}
}

func (b *blockingMarshaler) MarshalJSON() ([]byte, error) {
	b.entered <- struct{}{}
	<-b.release
	return []byte("{}"), nil
}

// TestNPMCacheHandlerLimitsConcurrency verifies that the cache handler admits only a bounded
// number of encodings at once. Each encoding holds the cache lock and buffers the whole
// policy cache, so without a ceiling the number of concurrent clients alone decides how much
// memory NPM allocates and how long the cache stays locked.
func TestNPMCacheHandlerLimitsConcurrency(t *testing.T) {
	encoder := &blockingMarshaler{
		entered: make(chan struct{}, maxConcurrentCacheRequests),
		release: make(chan struct{}),
	}
	n := &NPMRestServer{}
	handler := n.npmCacheHandler(encoder)

	// Fill every slot and wait until each request is actually inside MarshalJSON.
	var wg sync.WaitGroup
	for i := 0; i < maxConcurrentCacheRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, api.NPMMgrPath, http.NoBody)
			handler.ServeHTTP(httptest.NewRecorder(), req)
		}()
	}
	for i := 0; i < maxConcurrentCacheRequests; i++ {
		<-encoder.entered
	}

	// With every slot busy, a further request must be shed instead of queueing another
	// full copy of the cache.
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequestWithContext(t.Context(), http.MethodGet, api.NPMMgrPath, http.NoBody))
	require.Equal(t, http.StatusServiceUnavailable, rr.Code,
		"a request beyond the in-flight limit must be shed")

	close(encoder.release)
	wg.Wait()

	// Once the in-flight requests drain, the handler must serve again.
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequestWithContext(t.Context(), http.MethodGet, api.NPMMgrPath, http.NoBody))
	require.Equal(t, http.StatusOK, rr.Code, "the handler must recover once slots free up")
}

// TestServerTimeoutsAreSet guards the deadlines and bounds on the server that is actually
// constructed. The API listens on the host network of a privileged process, so a client that
// never finishes a request must not be able to hold it, and the response buffered for it, open
// indefinitely. Asserting the constructed server rather than the constants means removing an
// assignment in newServer fails this test.
func TestServerTimeoutsAreSet(t *testing.T) {
	srv := newServer("127.0.0.1:0", mux.NewRouter())

	require.Equal(t, readHeaderTimeout, srv.ReadHeaderTimeout)
	require.Equal(t, readTimeout, srv.ReadTimeout)
	require.Equal(t, writeTimeout, srv.WriteTimeout)
	require.Equal(t, idleTimeout, srv.IdleTimeout)
	require.Equal(t, maxHeaderBytes, srv.MaxHeaderBytes)

	require.NotZero(t, srv.ReadHeaderTimeout)
	require.NotZero(t, srv.ReadTimeout)
	require.NotZero(t, srv.WriteTimeout)
	require.NotZero(t, srv.IdleTimeout)
	require.NotZero(t, srv.MaxHeaderBytes)
	require.NotZero(t, maxConcurrentConns)
	require.LessOrEqual(t, maxConcurrentCacheRequests, maxConcurrentConns,
		"cache encodings must be bounded at or below the connection ceiling")
}

// TestLoopbackOnly covers the guard on the debug and pprof routes. NPM runs on the host
// network of a privileged process, so before this guard any pod on the node could reach
// those routes through its own node address; a pod cannot reach the node's loopback, and
// the on-node tooling that consumes them connects over localhost.
func TestLoopbackOnly(t *testing.T) {
	served := false
	handler := loopbackOnly(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		served = true
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name       string
		remoteAddr string
		wantCode   int
		wantServed bool
	}{
		{"IPv4 loopback", "127.0.0.1:54321", http.StatusOK, true},
		{"IPv4 loopback range", "127.9.9.9:54321", http.StatusOK, true},
		{"IPv6 loopback", "[::1]:54321", http.StatusOK, true},
		// the address a pod on the node would come from
		{"pod address", "10.244.1.7:54321", http.StatusForbidden, false},
		// the node's own routable address, which a pod reads from the downward API
		{"node address", "10.240.0.4:54321", http.StatusForbidden, false},
		{"malformed remote address", "not-an-address", http.StatusForbidden, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			served = false
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, api.NPMMgrPath, http.NoBody)
			req.RemoteAddr = tt.remoteAddr

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			require.Equal(t, tt.wantCode, rr.Code)
			require.Equal(t, tt.wantServed, served, "whether the wrapped handler ran")
		})
	}
}

// TestLoopbackOnlyGuardsBeforeHandler makes sure a rejected request never reaches the cache
// encoder. The encoding is the expensive part of the route, so the guard has to run first.
func TestLoopbackOnlyGuardsBeforeHandler(t *testing.T) {
	encoder := &blockingMarshaler{
		entered: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	n := &NPMRestServer{}
	handler := loopbackOnly(n.npmCacheHandler(encoder))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, api.NPMMgrPath, http.NoBody)
	req.RemoteAddr = "10.244.1.7:54321"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusForbidden, rr.Code)
	require.Empty(t, encoder.entered, "the cache must not be encoded for a rejected request")
}

// Remote addresses used by the routing and guard cases below.
const (
	nodeLoopbackAddr = "127.0.0.1:1"
	podAddr          = "10.244.1.7:1"
	// anyRedirect asks for a redirect of any code rather than a specific status.
	anyRedirect = -1
)

// TestPprofRoutesAreMountedAtTheProfilePrefix covers the routing for the profiling handlers:
// every pprof subpath must be served, the routes must stay behind the loopback guard, and
// nothing else on the default mux may be reachable through /debug/.
func TestPprofRoutesAreMountedAtTheProfilePrefix(t *testing.T) {
	http.DefaultServeMux.HandleFunc("/debug/unrelated", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	router := mux.NewRouter()
	router.PathPrefix("/debug/pprof").Handler(loopbackOnly(http.DefaultServeMux))

	tests := []struct {
		name       string
		path       string
		remoteAddr string
		wantCode   int
	}{
		{"pprof index from the node", "/debug/pprof/", nodeLoopbackAddr, http.StatusOK},
		{"pprof cmdline from the node", "/debug/pprof/cmdline", nodeLoopbackAddr, http.StatusOK},
		// a subpath that naming each handler individually did not cover
		{"pprof goroutine from the node", "/debug/pprof/goroutine", nodeLoopbackAddr, http.StatusOK},
		// without the trailing slash the mux redirects to the index rather than 404ing.
		// The exact redirect code is the mux's choice, so only the class is asserted.
		{"pprof index without a trailing slash", "/debug/pprof", nodeLoopbackAddr, anyRedirect},
		{"pprof index from a pod", "/debug/pprof/", podAddr, http.StatusForbidden},
		{"pprof goroutine from a pod", "/debug/pprof/goroutine", podAddr, http.StatusForbidden},
		// anything else on the default mux must not be reachable through this router
		{"unrelated default mux route", "/debug/unrelated", nodeLoopbackAddr, http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, tt.path, http.NoBody)
			req.RemoteAddr = tt.remoteAddr
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)
			if tt.wantCode == anyRedirect {
				require.GreaterOrEqual(t, rr.Code, http.StatusMultipleChoices)
				require.Less(t, rr.Code, http.StatusBadRequest)
				return
			}
			require.Equal(t, tt.wantCode, rr.Code)
		})
	}
}
