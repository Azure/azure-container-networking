//go:build !ignore_uncovered
// +build !ignore_uncovered

// Copyright 2020 Microsoft. All rights reserved.
// MIT License

package fakes

// NMAgentClientTest can be used to query to VM Host info.
type NMAgentClientTest struct{}

// NewFakeNMAgentClient return a mock implemetation of NMAgentClient
func NewFakeNMAgentClient() *NMAgentClientTest {
	return &NMAgentClientTest{}
}

// GetNcVersionListWithOutToken is mock implementation to return nc version list.
func (nmagentclient *NMAgentClientTest) GetNcVersionListWithOutToken(ncNeedUpdateList []string) map[string]int {
	ncVersionList := make(map[string]int)
	for _, ncID := range ncNeedUpdateList {
		ncVersionList[ncID] = 0
	}
	return ncVersionList
}
