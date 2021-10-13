//go:build !ignore_uncovered
// +build !ignore_uncovered

// Copyright 2020 Microsoft. All rights reserved.
// MIT License

package fakes

import "context"

// NMAgentClientFake can be used to query to VM Host info.
type NMAgentClientFake struct{}

// GetNcVersionListWithOutToken is mock implementation to return nc version list.
func (nmagentclient *NMAgentClientFake) GetNcVersionListWithOutToken(_ context.Context, ncNeedUpdateList []string) map[string]int {
	ncVersionList := make(map[string]int)
	for _, ncID := range ncNeedUpdateList {
		ncVersionList[ncID] = 0
	}
	return ncVersionList
}
