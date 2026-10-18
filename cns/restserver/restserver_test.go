package restserver

import (
	"io"
	"sync"
	"testing"

	"github.com/Azure/azure-container-networking/cns/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type conflistGeneratorFuncs struct {
	generate func() error
	close    func() error
}

func (g conflistGeneratorFuncs) Generate() error { return g.generate() }
func (g conflistGeneratorFuncs) Close() error    { return g.close() }

func TestCNIConflistGenerated(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		generateErr error
		closeErr    error
		wantPanic   string
	}{
		{name: "success"},
		{
			name:        "generation fails",
			generateErr: io.ErrShortWrite,
			wantPanic:   "unable to generate cni conflist with error: " + io.ErrShortWrite.Error(),
		},
		{
			name:      "publication fails",
			closeErr:  io.ErrClosedPipe,
			wantPanic: "unable to close the cni conflist output stream: " + io.ErrClosedPipe.Error(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			service := &HTTPRestService{}
			var generateCalls, closeCalls int
			service.cniConflistGenerator = conflistGeneratorFuncs{
				generate: func() error {
					generateCalls++
					assert.False(t, service.CNIConflistGenerated(), "generation is incomplete")
					return tt.generateErr
				},
				close: func() error {
					closeCalls++
					assert.False(t, service.CNIConflistGenerated(), "publication is incomplete")
					return tt.closeErr
				},
			}

			require.False(t, service.CNIConflistGenerated())
			if tt.wantPanic != "" {
				require.PanicsWithValue(t, tt.wantPanic, service.MustGenerateCNIConflistOnce)
			} else {
				require.NotPanics(t, service.MustGenerateCNIConflistOnce)
			}
			require.Equal(t, tt.wantPanic == "", service.CNIConflistGenerated())

			service.MustGenerateCNIConflistOnce()
			assert.Equal(t, tt.wantPanic == "", service.CNIConflistGenerated())
			assert.Equal(t, 1, generateCalls)
			if tt.generateErr != nil {
				assert.Zero(t, closeCalls)
			} else {
				assert.Equal(t, 1, closeCalls)
			}
		})
	}
}

func TestCNIConflistGeneratedConcurrent(t *testing.T) {
	t.Parallel()
	var generateCalls, closeCalls int
	service := &HTTPRestService{
		cniConflistGenerator: conflistGeneratorFuncs{
			generate: func() error {
				generateCalls++
				return nil
			},
			close: func() error {
				closeCalls++
				return nil
			},
		},
	}
	var callers sync.WaitGroup
	for range 16 {
		callers.Go(func() {
			_ = service.CNIConflistGenerated()
			service.MustGenerateCNIConflistOnce()
			assert.True(t, service.CNIConflistGenerated())
		})
	}
	callers.Wait()
	assert.Equal(t, 1, generateCalls)
	assert.Equal(t, 1, closeCalls)
}

func setMockNMAgent(h *HTTPRestService, m *fakes.NMAgentClientFake) func() {
	// this is a hack that exists because the tests are too DRY, so the setup
	// logic has ossified in TestMain

	// save the previous value of the NMAgent so that it can be restored by the
	// cleanup function
	prev := h.nma

	// set the NMAgent to what was requested
	h.nma = m

	// return a cleanup function that will restore NMAgent back to what it was
	return func() {
		h.nma = prev
	}
}

func setWireserverProxy(h *HTTPRestService, w *fakes.WireserverProxyFake) func() {
	prev := h.wsproxy
	h.wsproxy = w
	return func() {
		h.wsproxy = prev
	}
}
