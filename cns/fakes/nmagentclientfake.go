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
type NMAgentClientFake struct{}

// GetNcVersionListWithOutToken is mock implementation to return nc version list.
func (nmagentclient *NMAgentClientFake) GetNCVersionList(_ context.Context) (*nmagent.NetworkContainerListResponse, error) {
	var ncNeedUpdateList []string
	ncVersionList := make(map[string]int)
	for _, ncID := range ncNeedUpdateList {
		ncVersionList[ncID] = 0
	}
	return nil, nil
}
