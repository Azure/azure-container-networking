// Copyright 2026 Microsoft. All rights reserved.
// MIT License

package main

import (
	"errors"
	"fmt"
)

var (
	errUnsupportedEndpointStateProvider = errors.New("unsupported endpoint state provider")
	errNilEndpointStateStartupFactory   = errors.New("endpoint state startup factory is nil")
)

type endpointStateProvider string

const (
	endpointStateProviderJSON    endpointStateProvider = "json"
	endpointStateProviderUnified endpointStateProvider = "unified"
)

const productionEndpointStateProvider = endpointStateProviderJSON

func (provider endpointStateProvider) restoresStateFromJSON() bool {
	return provider == endpointStateProviderJSON
}

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
		return nil, fmt.Errorf("%w: %q", errUnsupportedEndpointStateProvider, provider)
	}
	if factory == nil {
		return nil, errNilEndpointStateStartupFactory
	}
	return factory()
}
