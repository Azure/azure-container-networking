// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package restserver

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPersistentStateHandlerConstructors(t *testing.T) {
	statusHandler, err := NewPersistentStateStatusHandler(nil)
	require.Error(t, err)
	assert.Nil(t, statusHandler)

	snapshotHandler, err := NewPersistentStateSnapshotHandler(nil, false)
	require.Error(t, err)
	assert.Nil(t, snapshotHandler)
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
	var gotContext context.Context
	handler, err := NewPersistentStateStatusHandler(func(ctx context.Context) (state.Status, error) {
		gotContext = ctx
		return safeStatus, nil
	})
	require.NoError(t, err)

	t.Run("safe JSON", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/persistent-state", nil)
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
		assert.Equal(t, request.Context(), gotContext)
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
		invalidHandler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/persistent-state", nil))
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
		{name: "canceled", err: context.Canceled, status: http.StatusRequestTimeout, body: "request canceled\n"},
		{name: "deadline", err: context.DeadlineExceeded, status: http.StatusRequestTimeout, body: "request canceled\n"},
		{name: "provider", err: errors.New("secret path /var/lib/state"), status: http.StatusServiceUnavailable, body: "persistent state unavailable\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, err := NewPersistentStateStatusHandler(func(context.Context) (state.Status, error) {
				return state.Status{}, tt.err
			})
			require.NoError(t, err)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/persistent-state", nil))
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
	request := httptest.NewRequest(http.MethodGet, "/persistent-state", nil).WithContext(ctx)
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
	disabled.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/persistent-state/snapshot", nil))
	assert.Equal(t, http.StatusNotFound, response.Code)
	assert.Zero(t, calls)

	enabled, err := NewPersistentStateSnapshotHandler(provider, true)
	require.NoError(t, err)
	response = httptest.NewRecorder()
	enabled.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/persistent-state/snapshot", nil))
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
			return state.Snapshot{}, errors.New("provider failure")
		}, true)
		require.NoError(t, err)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/persistent-state/snapshot", nil))
		assert.Equal(t, http.StatusServiceUnavailable, response.Code)
	})

	t.Run("encoding", func(t *testing.T) {
		snapshot := state.NewSnapshot()
		snapshot.Networks["network"] = state.NetworkRecord{
			NetworkName: "network",
			Options:     map[string]any{"unsupported": make(chan struct{})},
		}
		handler, err := NewPersistentStateSnapshotHandler(func(context.Context) (state.Snapshot, error) {
			return snapshot, nil
		}, true)
		require.NoError(t, err)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/persistent-state/snapshot", nil))
		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.Equal(t, "encoding persistent state response\n", response.Body.String())
	})

	t.Run("recursive sanitizer", func(t *testing.T) {
		document := map[string]any{
			"AuthorizationToken": "secret",
			"nested": []any{
				map[string]any{"authorizationtoken": "another"},
			},
		}
		removeAuthorizationTokens(document)
		assert.NotContains(t, document, "AuthorizationToken")
		assert.NotContains(t, document["nested"].([]any)[0].(map[string]any), "authorizationtoken")
	})
}

func assertCommonGETContract(t *testing.T, handler http.Handler) {
	t.Helper()
	t.Run("method mismatch", func(t *testing.T) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/persistent-state", nil))
		assert.Equal(t, http.StatusMethodNotAllowed, response.Code)
		assert.Equal(t, http.MethodGet, response.Header().Get("Allow"))
	})
	t.Run("GET body rejected", func(t *testing.T) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/persistent-state", strings.NewReader("{}")))
		assert.Equal(t, http.StatusBadRequest, response.Code)
		assert.Equal(t, "request body is not allowed\n", response.Body.String())
	})
}
