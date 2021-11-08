//go:build !ignore_uncovered
// +build !ignore_uncovered

// Copyright 2020 Microsoft. All rights reserved.
// MIT License

package fakes

import (
	"context"

	"github.com/Azure/azure-container-networking/cns/nmagent"
)

// NMAgentClientFake can be used to query to VM Host info.
type NMAgentClientFake struct {
	GetNCVersionListFunc func(context.Context) (*nmagent.NetworkContainerListResponse, error)
}

// GetNcVersionListWithOutToken is mock implementation to return nc version list.
func (c *NMAgentClientFake) GetNCVersionList(ctx context.Context) (*nmagent.NetworkContainerListResponse, error) {
	return c.GetNCVersionListFunc(ctx)
}
