// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package restserver

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/state"
	acn "github.com/Azure/azure-container-networking/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errPersistentStateProvider = errors.New("secret path /var/lib/state")
	errSnapshotProvider        = errors.New("provider failure")
)

const (
	persistentStateTestNetwork   = "network"
	persistentStateTestNestedKey = "nested"
)

func TestPersistentStateHandlerConstructors(t *testing.T) {
	statusHandler, err := NewPersistentStateStatusHandler(nil)
	require.Error(t, err)
	assert.Nil(t, statusHandler)

	snapshotHandler, err := NewPersistentStateSnapshotHandler(nil, false)
	require.Error(t, err)
	assert.Nil(t, snapshotHandler)
}

func TestRegisterPersistentStateRoutes(t *testing.T) {
	newService := func(t *testing.T) *HTTPRestService {
		t.Helper()
		listener, err := acn.NewListener(&url.URL{Scheme: "tcp", Host: "127.0.0.1:0"})
		require.NoError(t, err)
		return &HTTPRestService{
			Service: &cns.Service{Listener: listener},
		}
	}
	status := func(context.Context) (state.Status, error) {
		return state.Status{Backend: state.BackendBolt, InvariantStatus: state.InvariantHealthy}, nil
	}
	snapshot := func(context.Context) (state.Snapshot, error) {
		return state.NewSnapshot(), nil
	}

	t.Run("safe only", func(t *testing.T) {
		service := newService(t)
		require.NoError(t, service.RegisterPersistentStateRoutes(status, snapshot, false))
		require.NoError(t, service.RegisterPersistentStateRoutes(status, snapshot, false))

		response := httptest.NewRecorder()
		service.Listener.GetMux().ServeHTTP(
			response,
			httptest.NewRequest(http.MethodGet, PersistentStateStatusPath, nil),
		)
		assert.Equal(t, http.StatusOK, response.Code)

		response = httptest.NewRecorder()
		service.Listener.GetMux().ServeHTTP(
			response,
			httptest.NewRequest(http.MethodGet, PersistentStateSnapshotPath, nil),
		)
		assert.Equal(t, http.StatusNotFound, response.Code)
	})

	t.Run("debug snapshot", func(t *testing.T) {
		service := newService(t)
		require.NoError(t, service.RegisterPersistentStateRoutes(status, snapshot, true))
		response := httptest.NewRecorder()
		service.Listener.GetMux().ServeHTTP(
			response,
			httptest.NewRequest(http.MethodGet, PersistentStateSnapshotPath, nil),
		)
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("listener required", func(t *testing.T) {
		service := &HTTPRestService{}
		require.Error(t, service.RegisterPersistentStateRoutes(status, snapshot, false))
	})
}

func TestPersistentStateStatusHandlerContract(t *testing.T) {
	safeStatus := state.Status{
		Backend:         state.BackendBolt,
		Authority:       state.AuthorityBolt,
		SchemaVersion:   state.SchemaVersion,
		Generation:      7,
		BootPresent:     true,
		StoragePresent:  true,
		DatabaseBytes:   4096,
		InvariantStatus: state.InvariantHealthy,
	}
	contexts := make(chan context.Context, 1)
	handler, err := NewPersistentStateStatusHandler(func(ctx context.Context) (state.Status, error) {
		contexts <- ctx
		return safeStatus, nil
	})
	require.NoError(t, err)

	t.Run("safe JSON", func(t *testing.T) {
		request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/persistent-state", http.NoBody)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)

		assert.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
		assert.JSONEq(t, `{
			"backend":"bbolt",
			"authority":"bolt",
			"schemaVersion":1,
			"generation":7,
			"bootPresent":true,
			"storagePresent":true,
			"databaseBytes":4096,
			"records":{
				"networkContainers":0,
				"ips":0,
				"networks":0,
				"endpoints":0,
				"assignments":0,
				"owners":0,
				"deleteIntents":0
			},
			"invariantStatus":"healthy",
			"legacyImported":false,
			"rollbackExported":false
		}`, response.Body.String())
		assert.Equal(t, request.Context(), <-contexts)
		assert.NotContains(t, response.Body.String(), "bootID")
		assert.NotContains(t, response.Body.String(), "node")
		assert.NotContains(t, response.Body.String(), "path")
	})

	t.Run("invalid invariant is observable", func(t *testing.T) {
		invalidHandler, handlerErr := NewPersistentStateStatusHandler(func(context.Context) (state.Status, error) {
			status := safeStatus
			status.InvariantStatus = state.InvariantFailed
			status.FailedInvariant = state.InvariantStructural
			return status, nil
		})
		require.NoError(t, handlerErr)
		response := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/persistent-state", http.NoBody)
		invalidHandler.ServeHTTP(response, request)
		assert.Equal(t, http.StatusOK, response.Code)
		assert.Contains(t, response.Body.String(), `"failedInvariant":"structural"`)
	})

	assertCommonGETContract(t, handler)
}

func TestPersistentStateHandlersProviderErrors(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		body   string
	}{
		{name: delTestCanceled, err: context.Canceled, status: http.StatusRequestTimeout, body: "request canceled\n"},
		{name: "deadline", err: context.DeadlineExceeded, status: http.StatusRequestTimeout, body: "request canceled\n"},
		{name: "provider", err: errPersistentStateProvider, status: http.StatusServiceUnavailable, body: "persistent state unavailable\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, err := NewPersistentStateStatusHandler(func(context.Context) (state.Status, error) {
				return state.Status{}, tt.err
			})
			require.NoError(t, err)
			response := httptest.NewRecorder()
			request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/persistent-state", http.NoBody)
			handler.ServeHTTP(response, request)
			assert.Equal(t, tt.status, response.Code)
			assert.Equal(t, tt.body, response.Body.String())
			assert.NotContains(t, response.Body.String(), "/var/lib")
		})
	}

	handler, err := NewPersistentStateStatusHandler(func(ctx context.Context) (state.Status, error) {
		return state.Status{}, ctx.Err()
	})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := httptest.NewRequestWithContext(ctx, http.MethodGet, "/persistent-state", http.NoBody)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	assert.Equal(t, http.StatusRequestTimeout, response.Code)
}

func TestPersistentStateSnapshotHandlerGateAndSanitization(t *testing.T) {
	calls := 0
	snapshot := state.NewSnapshot()
	snapshot.NetworkContainers["nc"] = state.NetworkContainerRecord{
		ID: "nc",
		Request: cns.CreateNetworkContainerRequest{
			NetworkContainerid: "nc",
			AuthorizationToken: "super-secret",
		},
	}
	provider := func(context.Context) (state.Snapshot, error) {
		calls++
		return snapshot, nil
	}

	disabled, err := NewPersistentStateSnapshotHandler(provider, false)
	require.NoError(t, err)
	response := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/persistent-state/snapshot", http.NoBody)
	disabled.ServeHTTP(response, request)
	assert.Equal(t, http.StatusNotFound, response.Code)
	assert.Zero(t, calls)

	enabled, err := NewPersistentStateSnapshotHandler(provider, true)
	require.NoError(t, err)
	response = httptest.NewRecorder()
	request = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/persistent-state/snapshot", http.NoBody)
	enabled.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
	assert.Equal(t, 1, calls)
	assert.Contains(t, response.Body.String(), `"NetworkContainers"`)
	assert.NotContains(t, strings.ToLower(response.Body.String()), "authorizationtoken")
	assert.NotContains(t, response.Body.String(), "super-secret")

	assertCommonGETContract(t, enabled)
}

func TestPersistentStateSnapshotHandlerErrors(t *testing.T) {
	t.Run("provider", func(t *testing.T) {
		handler, err := NewPersistentStateSnapshotHandler(func(context.Context) (state.Snapshot, error) {
			return state.Snapshot{}, errSnapshotProvider
		}, true)
		require.NoError(t, err)
		response := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/persistent-state/snapshot", http.NoBody)
		handler.ServeHTTP(response, request)
		assert.Equal(t, http.StatusServiceUnavailable, response.Code)
	})

	t.Run("encoding", func(t *testing.T) {
		snapshot := state.NewSnapshot()
		snapshot.Networks[persistentStateTestNetwork] = state.NetworkRecord{
			NetworkName: persistentStateTestNetwork,
			Options:     map[string]any{"unsupported": make(chan struct{})},
		}
		handler, err := NewPersistentStateSnapshotHandler(func(context.Context) (state.Snapshot, error) {
			return snapshot, nil
		}, true)
		require.NoError(t, err)
		response := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/persistent-state/snapshot", http.NoBody)
		handler.ServeHTTP(response, request)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.Equal(t, "encoding persistent state response\n", response.Body.String())
	})

	t.Run("recursive sanitizer", func(t *testing.T) {
		document := map[string]any{
			"AuthorizationToken": "secret",
			persistentStateTestNestedKey: []any{
				map[string]any{"authorizationtoken": "another"},
			},
		}
		removeAuthorizationTokens(document)
		assert.NotContains(t, document, "AuthorizationToken")
		assert.NotContains(
			t,
			document[persistentStateTestNestedKey].([]any)[0].(map[string]any),
			"authorizationtoken",
		)
	})
}

func assertCommonGETContract(t *testing.T, handler http.Handler) {
	t.Helper()
	t.Run("method mismatch", func(t *testing.T) {
		response := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/persistent-state", http.NoBody)
		handler.ServeHTTP(response, request)
		assert.Equal(t, http.StatusMethodNotAllowed, response.Code)
		assert.Equal(t, http.MethodGet, response.Header().Get("Allow"))
	})
	t.Run("GET body rejected", func(t *testing.T) {
		response := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/persistent-state",
			strings.NewReader("{}"),
		)
		handler.ServeHTTP(response, request)
		assert.Equal(t, http.StatusBadRequest, response.Code)
		assert.Equal(t, "request body is not allowed\n", response.Body.String())
	})
}
