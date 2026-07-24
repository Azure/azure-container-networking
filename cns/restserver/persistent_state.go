// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package restserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Azure/azure-container-networking/cns/state"
)

const (
	// PersistentStateStatusPath exposes bounded persistent-state metadata and counts.
	PersistentStateStatusPath = "/debug/persistent-state/status"
	// PersistentStateSnapshotPath exposes the token-redacted logical state when explicitly enabled.
	PersistentStateSnapshotPath = "/debug/persistent-state/snapshot"
)

var (
	errNilPersistentStateStatusProvider   = errors.New("persistent state status provider is nil")
	errNilPersistentStateSnapshotProvider = errors.New("persistent state snapshot provider is nil")
)

type statusProvider func(context.Context) (state.Status, error)

type snapshotProvider func(context.Context) (state.Snapshot, error)

type PersistentStateStatusHandler struct {
	status statusProvider
}

func NewPersistentStateStatusHandler(
	status func(context.Context) (state.Status, error),
) (*PersistentStateStatusHandler, error) {
	if status == nil {
		return nil, errNilPersistentStateStatusProvider
	}
	return &PersistentStateStatusHandler{status: status}, nil
}

func (h *PersistentStateStatusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !allowGET(w, r) {
		return
	}
	status, err := h.status(r.Context())
	if err != nil {
		writePersistentStateError(w, err)
		return
	}
	writeJSON(w, status)
}

type PersistentStateSnapshotHandler struct {
	snapshot snapshotProvider
	enabled  bool
}

func NewPersistentStateSnapshotHandler(
	snapshot func(context.Context) (state.Snapshot, error),
	enabled bool,
) (*PersistentStateSnapshotHandler, error) {
	if snapshot == nil {
		return nil, errNilPersistentStateSnapshotProvider
	}
	return &PersistentStateSnapshotHandler{
		snapshot: snapshot,
		enabled:  enabled,
	}, nil
}

// RegisterPersistentStateRoutes registers the safe status route and optionally the logical snapshot route.
func (service *HTTPRestService) RegisterPersistentStateRoutes(
	status func(context.Context) (state.Status, error),
	snapshot func(context.Context) (state.Snapshot, error),
	enableSnapshot bool,
) error {
	if service == nil || service.Service == nil || service.Listener == nil {
		return errors.New("persistent state listener is nil")
	}
	service.persistentStateRoutesOnce.Do(func() {
		statusHandler, err := NewPersistentStateStatusHandler(status)
		if err != nil {
			service.persistentStateRoutesErr = err
			return
		}
		mux := service.Listener.GetMux()
		mux.Handle(PersistentStateStatusPath, statusHandler)
		if !enableSnapshot {
			return
		}
		snapshotHandler, err := NewPersistentStateSnapshotHandler(snapshot, true)
		if err != nil {
			service.persistentStateRoutesErr = err
			return
		}
		mux.Handle(PersistentStateSnapshotPath, snapshotHandler)
	})
	return service.persistentStateRoutesErr
}

func (h *PersistentStateSnapshotHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !allowGET(w, r) {
		return
	}
	if !h.enabled {
		http.NotFound(w, r)
		return
	}
	snapshot, err := h.snapshot(r.Context())
	if err != nil {
		writePersistentStateError(w, err)
		return
	}
	writeJSON(w, persistentStateSnapshotResponse(snapshot))
}

type persistentStateSnapshotResponse state.Snapshot

func (r persistentStateSnapshotResponse) MarshalJSON() ([]byte, error) {
	data, err := json.Marshal(state.Snapshot(r)) //nolint:musttag // Snapshot preserves its established debug JSON field names.
	if err != nil {
		return nil, fmt.Errorf("encoding persistent state snapshot: %w", err)
	}
	var document map[string]any
	if decodeErr := json.Unmarshal(data, &document); decodeErr != nil {
		return nil, fmt.Errorf("sanitizing persistent state snapshot: %w", decodeErr)
	}
	removeAuthorizationTokens(document)
	sanitized, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("encoding sanitized persistent state snapshot: %w", err)
	}
	return sanitized, nil
}

func removeAuthorizationTokens(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if strings.EqualFold(key, "authorizationToken") {
				delete(typed, key)
				continue
			}
			removeAuthorizationTokens(child)
		}
	case []any:
		for _, child := range typed {
			removeAuthorizationTokens(child)
		}
	}
}

func allowGET(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	if r.Body != nil && r.ContentLength != 0 {
		http.Error(w, "request body is not allowed", http.StatusBadRequest)
		return false
	}
	return true
}

func writePersistentStateError(w http.ResponseWriter, err error) {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		http.Error(w, "request canceled", http.StatusRequestTimeout)
		return
	}
	http.Error(w, "persistent state unavailable", http.StatusServiceUnavailable)
}

func writeJSON(w http.ResponseWriter, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		http.Error(w, "encoding persistent state response", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(append(data, '\n'))
}
