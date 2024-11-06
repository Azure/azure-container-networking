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
	SupportedAPIsF      func(context.Context) ([]string, error)
	GetNCVersionListF   func(context.Context) (nmagent.NCVersionList, error)
	GetHomeAzF          func(context.Context) (nmagent.AzResponse, error)
	GetInterfaceIPInfoF func(ctx context.Context) (nmagent.Interfaces, error)
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

func (n *NMAgentClientFake) GetInterfaceIPInfo(ctx context.Context) (nmagent.Interfaces, error) {
	return n.GetInterfaceIPInfoF(ctx)
}
