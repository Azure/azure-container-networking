//go:build !ignore_uncovered
// +build !ignore_uncovered

// Copyright 2020 Microsoft. All rights reserved.
// MIT License

package fakes

import (
	"context"

	"github.com/Azure/azure-container-networking/nmagent"
)

// NMAgentClientFake can be used to query to VM Host info.
type NMAgentClientFake struct {
	PutNetworkContainerF    func(context.Context, *nmagent.PutNetworkContainerRequest) error
	DeleteNetworkContainerF func(context.Context, nmagent.DeleteContainerRequest) error
	JoinNetworkF            func(context.Context, nmagent.JoinNetworkRequest) error
	SupportedAPIsF          func(context.Context) ([]string, error)
	GetNCVersionF           func(context.Context, nmagent.NCVersionRequest) (nmagent.NCVersion, error)
	GetNCVersionListF       func(context.Context) (nmagent.NCVersionList, error)
	GetHomeAzInfoF          func(context.Context) (nmagent.HomeAzInfo, error)
}

func (c *NMAgentClientFake) PutNetworkContainer(ctx context.Context, req *nmagent.PutNetworkContainerRequest) error {
	return c.PutNetworkContainerF(ctx, req)
}

func (c *NMAgentClientFake) DeleteNetworkContainer(ctx context.Context, req nmagent.DeleteContainerRequest) error {
	return c.DeleteNetworkContainerF(ctx, req)
}

func (c *NMAgentClientFake) JoinNetwork(ctx context.Context, req nmagent.JoinNetworkRequest) error {
	return c.JoinNetworkF(ctx, req)
}

func (c *NMAgentClientFake) SupportedAPIs(ctx context.Context) ([]string, error) {
	return c.SupportedAPIsF(ctx)
}

func (c *NMAgentClientFake) GetNCVersion(ctx context.Context, req nmagent.NCVersionRequest) (nmagent.NCVersion, error) {
	return c.GetNCVersionF(ctx, req)
}

func (c *NMAgentClientFake) GetNCVersionList(ctx context.Context) (nmagent.NCVersionList, error) {
	return c.GetNCVersionListF(ctx)
}

func (c *NMAgentClientFake) GetHomeAzInfo(ctx context.Context) (nmagent.HomeAzInfo, error) {
	return c.GetHomeAzInfoF(ctx)
}
