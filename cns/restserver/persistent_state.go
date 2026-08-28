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

type statusProvider func(context.Context) (state.Status, error)

type snapshotProvider func(context.Context) (state.Snapshot, error)

type PersistentStateStatusHandler struct {
	status statusProvider
}

func NewPersistentStateStatusHandler(
	status func(context.Context) (state.Status, error),
) (*PersistentStateStatusHandler, error) {
	if status == nil {
		return nil, errors.New("persistent state status provider is nil")
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
		return nil, errors.New("persistent state snapshot provider is nil")
	}
	return &PersistentStateSnapshotHandler{
		snapshot: snapshot,
		enabled:  enabled,
	}, nil
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
	data, err := json.Marshal(state.Snapshot(r))
	if err != nil {
		return nil, fmt.Errorf("encoding persistent state snapshot: %w", err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("sanitizing persistent state snapshot: %w", err)
	}
	removeAuthorizationTokens(document)
	return json.Marshal(document)
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
