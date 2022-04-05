package controllers

import (
	"log"

	"github.com/Azure/azure-container-networking/npm/pkg/controlplane/controllers/common"
)

type Cache struct {
	NodeName string
	NsMap    map[string]*Namespace
	PodMap   map[string]*common.NpmPod
	SetMap   map[string]string
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
	return c.NsMap[namespace].LabelsMap[labelkey]
}

func (c *Cache) GetListMap() map[string]string {
	listMap := make(map[string]string, 0)
	// get all lists
	log.Printf("info: NPMV2 doesn't make use of the listmap")
	return listMap
}

func (c *Cache) GetSetMap() map[string]string {
	return c.SetMap
}
