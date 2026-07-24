// Copyright 2026 Microsoft. All rights reserved.
// MIT License

package main

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEndpointStateProviderSelection(t *testing.T) {
	t.Run("production JSON does not open unified state", func(t *testing.T) {
		jsonStartup := &persistentStateStartup{}
		jsonCalls := 0
		unifiedCalls := 0
		startup, err := newEndpointStateStartup(
			productionEndpointStateProvider,
			func() (*persistentStateStartup, error) {
				jsonCalls++
				return jsonStartup, nil
			},
			func() (*persistentStateStartup, error) {
				unifiedCalls++
				return nil, errors.New("unified database opener called")
			},
		)
		require.NoError(t, err)
		assert.Same(t, jsonStartup, startup)
		assert.Equal(t, 1, jsonCalls)
		assert.Zero(t, unifiedCalls)
	})

	t.Run("internal unified selection", func(t *testing.T) {
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
		unifiedErr := errors.New("unified restore failed")
		jsonCalls := 0
		startup, err := newEndpointStateStartup(
			endpointStateProviderUnified,
			func() (*persistentStateStartup, error) {
				jsonCalls++
				return &persistentStateStartup{}, nil
			},
			func() (*persistentStateStartup, error) {
				return nil, unifiedErr
			},
		)
		require.ErrorIs(t, err, unifiedErr)
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
