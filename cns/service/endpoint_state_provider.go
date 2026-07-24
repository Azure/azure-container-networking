// Copyright 2026 Microsoft. All rights reserved.
// MIT License

package main

import (
	"errors"
	"fmt"
)

type endpointStateProvider string

const (
	endpointStateProviderJSON    endpointStateProvider = "json"
	endpointStateProviderUnified endpointStateProvider = "unified"
)

const productionEndpointStateProvider = endpointStateProviderJSON

type endpointStateStartupFactory func() (*persistentStateStartup, error)

func newEndpointStateStartup(
	provider endpointStateProvider,
	jsonFactory endpointStateStartupFactory,
	unifiedFactory endpointStateStartupFactory,
) (*persistentStateStartup, error) {
	var factory endpointStateStartupFactory
	switch provider {
	case endpointStateProviderJSON:
		factory = jsonFactory
	case endpointStateProviderUnified:
		factory = unifiedFactory
	default:
		return nil, fmt.Errorf("unsupported endpoint state provider %q", provider)
	}
	if factory == nil {
		return nil, errors.New("endpoint state startup factory is nil")
	}
	return factory()
}
