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
	GetNCVersionListF       func(context.Context) (nmagent.NCVersionList, error)
	GetHomeAzF              func(context.Context) (nmagent.AzResponse, error)
}

func (n *NMAgentClientFake) PutNetworkContainer(ctx context.Context, req *nmagent.PutNetworkContainerRequest) error {
	return n.PutNetworkContainerF(ctx, req)
}

func (n *NMAgentClientFake) DeleteNetworkContainer(ctx context.Context, req nmagent.DeleteContainerRequest) error {
	return n.DeleteNetworkContainerF(ctx, req)
}

func (n *NMAgentClientFake) JoinNetwork(ctx context.Context, req nmagent.JoinNetworkRequest) error {
	return n.JoinNetworkF(ctx, req)
}

func (n *NMAgentClientFake) SupportedAPIs(ctx context.Context) ([]string, error) {
	return n.SupportedAPIsF(ctx)
}

func (n *NMAgentClientFake) GetNCVersionList(ctx context.Context) (nmagent.NCVersionList, error) {
	return n.GetNCVersionListF(ctx)
}

func (n *NMAgentClientFake) GetHomeAz(ctx context.Context) (nmagent.AzResponse, error) {
	return n.GetHomeAzF(ctx)
}
