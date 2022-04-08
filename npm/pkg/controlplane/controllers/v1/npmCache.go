// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package controllers

import (
	"github.com/Azure/azure-container-networking/npm/ipsm"
	"github.com/Azure/azure-container-networking/npm/pkg/controlplane/controllers/common"
	"github.com/Azure/azure-container-networking/npm/util"
)

type Cache struct {
	NodeName string
	NsMap    map[string]*Namespace
	PodMap   map[string]*common.NpmPod
	ListMap  map[string]*ipsm.Ipset
	SetMap   map[string]*ipsm.Ipset
}

func (c *Cache) GetPod(input *common.Input) (*common.NpmPod, error) {
	switch input.Type {
	case common.PODNAME:
		if pod, ok := c.PodMap[input.Content]; ok {
			return pod, nil
		}
		return nil, common.ErrInvalidInput
	case common.IPADDRS:
		for _, pod := range c.PodMap {
			if pod.PodIP == input.Content {
				return pod, nil
			}
		}
		return nil, common.ErrInvalidIPAddress
	case common.EXTERNAL:
		return &common.NpmPod{}, nil
	default:
		return nil, common.ErrInvalidInput
	}
}

func (c *Cache) GetNamespaceLabel(namespace, labelkey string) string {
	if _, ok := c.NsMap[namespace]; ok {
		return c.NsMap[namespace].LabelsMap[labelkey]
	}
	return ""
}

func (c *Cache) GetListMap() map[string]string {
	listMap := make(map[string]string)
	for k := range c.ListMap {
		hashedName := util.GetHashedName(k)
		listMap[hashedName] = k
	}
	return listMap
}

func (c *Cache) GetSetMap() map[string]string {
	setMap := make(map[string]string)

	for k := range c.SetMap {
		hashedName := util.GetHashedName(k)
		setMap[hashedName] = k
	}
	return setMap
}
