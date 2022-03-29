// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package controllers

import (
	"github.com/Azure/azure-container-networking/npm/ipsm"
	"github.com/Azure/azure-container-networking/npm/pkg/controlplane/controllers/common"
)

type Cache struct {
	NodeName string
	NsMap    map[string]*Namespace
	PodMap   map[string]*NpmPod
	ListMap  map[string]*ipsm.Ipset
	SetMap   map[string]*ipsm.Ipset
}

func (c *Cache) GetPod(input common.Input) (common.Pod, error) {
	switch input.Type {
	case common.PODNAME:
		if pod, ok := c.PodMap[input.Content]; ok {
			return pod, nil
		}
		return nil, common.ErrInvalidInput
	case common.IPADDRS:
		if pod, ok := ipPodMap[input.Content]; ok {
			return pod, nil
		}
		return nil, common.ErrInvalidIPAddress
	case common.EXTERNAL:
		return &NpmPod{}, nil
	default:
		return nil, common.ErrInvalidInput
	}
}
