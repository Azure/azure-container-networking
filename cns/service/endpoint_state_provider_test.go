// Copyright 2026 Microsoft. All rights reserved.
// MIT License

package main

import (
	"errors"
	"testing"

	"github.com/Azure/azure-container-networking/cns/configuration"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errUnifiedDatabaseOpened = errors.New("unified database opener called")
	errUnifiedRestore        = errors.New("unified restore failed")
)

func TestEndpointStateProviderSelection(t *testing.T) {
	t.Run("configuration matrix", func(t *testing.T) {
		tests := []struct {
			name            string
			backend         configuration.StateStoreBackend
			mode            configuration.StateStoreMode
			want            endpointStateProvider
			wantRestoreJSON bool
		}{
			{name: "default JSON", backend: configuration.StateStoreBackendJSON, mode: configuration.StateStoreModeNormal, want: endpointStateProviderJSON, wantRestoreJSON: true},
			{name: "cooling JSON", backend: configuration.StateStoreBackendJSON, mode: configuration.StateStoreModeNormal, want: endpointStateProviderJSON, wantRestoreJSON: true},
			{name: "rollback JSON", backend: configuration.StateStoreBackendJSON, mode: configuration.StateStoreModeRollbackToJSON, want: endpointStateProviderJSON, wantRestoreJSON: true},
			{name: "normal Bolt", backend: configuration.StateStoreBackendBolt, mode: configuration.StateStoreModeNormal, want: endpointStateProviderUnified},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				provider := selectEndpointStateProvider(tt.backend, tt.mode)
				assert.Equal(t, tt.want, provider)
				assert.Equal(t, tt.wantRestoreJSON, provider.restoresStateFromJSON())
			})
		}
	})

	t.Run("production JSON does not open unified state", func(t *testing.T) {
		jsonStartup := &persistentStateStartup{}
		jsonCalls := 0
		unifiedCalls := 0
		startup, err := newEndpointStateStartup(
			selectEndpointStateProvider(configuration.StateStoreBackendJSON, configuration.StateStoreModeNormal),
			func() (*persistentStateStartup, error) {
				jsonCalls++
				return jsonStartup, nil
			},
			func() (*persistentStateStartup, error) {
				unifiedCalls++
				return nil, errUnifiedDatabaseOpened
			},
		)
		require.NoError(t, err)
		assert.Same(t, jsonStartup, startup)
		assert.Equal(t, 1, jsonCalls)
		assert.Zero(t, unifiedCalls)
	})

	t.Run("internal unified selection", func(t *testing.T) {
		assert.False(t, endpointStateProviderUnified.restoresStateFromJSON())

		unifiedStartup := &persistentStateStartup{}
		jsonCalls := 0
		startup, err := newEndpointStateStartup(
			endpointStateProviderUnified,
			func() (*persistentStateStartup, error) {
				jsonCalls++
				return &persistentStateStartup{}, nil
			},
			func() (*persistentStateStartup, error) {
				return unifiedStartup, nil
			},
		)
		require.NoError(t, err)
		assert.Same(t, unifiedStartup, startup)
		assert.Zero(t, jsonCalls)
	})

	t.Run("unified failure does not fall back", func(t *testing.T) {
		jsonCalls := 0
		startup, err := newEndpointStateStartup(
			endpointStateProviderUnified,
			func() (*persistentStateStartup, error) {
				jsonCalls++
				return &persistentStateStartup{}, nil
			},
			func() (*persistentStateStartup, error) {
				return nil, errUnifiedRestore
			},
		)
		require.ErrorIs(t, err, errUnifiedRestore)
		assert.Nil(t, startup)
		assert.Zero(t, jsonCalls)
	})

	t.Run("unsupported selection", func(t *testing.T) {
		calls := 0
		factory := func() (*persistentStateStartup, error) {
			calls++
			return &persistentStateStartup{}, nil
		}
		startup, err := newEndpointStateStartup(endpointStateProvider("unsupported"), factory, factory)
		require.Error(t, err)
		assert.Nil(t, startup)
		assert.Zero(t, calls)
	})

	t.Run("selected factory is required", func(t *testing.T) {
		startup, err := newEndpointStateStartup(endpointStateProviderUnified, func() (*persistentStateStartup, error) {
			return &persistentStateStartup{}, nil
		}, nil)
		require.Error(t, err)
		assert.Nil(t, startup)
	})
}
