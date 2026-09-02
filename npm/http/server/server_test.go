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
			req := httptest.NewRequest(http.MethodGet, api.NPMMgrPath, http.NoBody)
			handler.ServeHTTP(httptest.NewRecorder(), req)
		}()
	}
	for i := 0; i < maxConcurrentCacheRequests; i++ {
		<-encoder.entered
	}

	// With every slot busy, a further request must be shed instead of queueing another
	// full copy of the cache.
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, api.NPMMgrPath, http.NoBody))
	require.Equal(t, http.StatusServiceUnavailable, rr.Code,
		"a request beyond the in-flight limit must be shed")

	close(encoder.release)
	wg.Wait()

	// Once the in-flight requests drain, the handler must serve again.
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, api.NPMMgrPath, http.NoBody))
	require.Equal(t, http.StatusOK, rr.Code, "the handler must recover once slots free up")
}

// TestServerTimeoutsAreSet is a guard on the deadlines and header bound. The API listens on
// the host network of a privileged process, so a client that never finishes a request must
// not be able to hold it, and the response buffered for it, open indefinitely.
func TestServerTimeoutsAreSet(t *testing.T) {
	require.NotZero(t, readHeaderTimeout)
	require.NotZero(t, readTimeout)
	require.NotZero(t, writeTimeout)
	require.NotZero(t, idleTimeout)
	require.NotZero(t, maxHeaderBytes)
	require.NotZero(t, maxConcurrentConns)
	require.LessOrEqual(t, maxConcurrentCacheRequests, maxConcurrentConns,
		"cache encodings must be bounded at or below the connection ceiling")
}
