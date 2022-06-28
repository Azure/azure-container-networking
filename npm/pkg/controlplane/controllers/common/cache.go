package common

import (
	"errors"
	"net"

	"github.com/Azure/azure-container-networking/npm/util"
)

// error type
var (
	ErrSetNotExist      = errors.New("set does not exists")
	ErrInvalidIPAddress = errors.New("invalid ipaddress, no equivalent pod found")
	ErrInvalidInput     = errors.New("invalid input")
	ErrSetType          = errors.New("invalid set type")
)

type Input struct {
	Content string
	Type    InputType
}

// InputType indicates allowed typle for source and destination input
type InputType int32

// GetInputType returns the type of the input for GetNetworkTuple.
func GetInputType(input string) InputType {
	if input == "External" {
		return EXTERNAL
	} else if ip := net.ParseIP(input); ip != nil {
		return IPADDRS
	} else {
		return NSPODNAME
	}
}

const (
	// IPADDRS indicates the IP Address input type, example: 10.0.0.1
	IPADDRS InputType = 0
	// NSPODNAME indicates the podname input type, example: x/a
	NSPODNAME InputType = 1
	// EXTERNAL indicates the external input type
	EXTERNAL InputType = 2
)

type GenericCache interface {
	GetPod(*Input) (*NpmPod, error)
	GetNamespaceLabel(namespace string, key string) string
	GetListMap() map[string]string
	GetSetMap() map[string]string
}

type Cache struct {
	NodeName string
	NsMap    map[string]*Namespace
	PodMap   map[string]*NpmPod
	SetMap   map[string]string
	ListMap  map[string]string // not used in NPMV2
}

func (c *Cache) GetPod(input *Input) (*NpmPod, error) {
	switch input.Type {
	case NSPODNAME:
		if pod, ok := c.PodMap[input.Content]; ok {
			return pod, nil
		}
		return nil, ErrInvalidInput
	case IPADDRS:
		for _, pod := range c.PodMap {
			if pod.PodIP == input.Content {
				return pod, nil
			}
		}
		return nil, ErrInvalidIPAddress
	case EXTERNAL:
		return &NpmPod{}, nil
	default:
		return nil, ErrInvalidInput
	}
}

func (c *Cache) GetNamespaceLabel(namespace, labelkey string) string {
	if _, ok := c.NsMap[namespace]; ok {
		return c.NsMap[namespace].LabelsMap[labelkey]
	}
	return ""
}

func (c *Cache) GetSetMap() map[string]string {
	return c.SetMap
}

func (c *Cache) GetListMap() map[string]string {
	listMap := make(map[string]string)
	for k := range c.ListMap {
		hashedName := util.GetHashedName(k)
		listMap[hashedName] = k
	}
	return listMap
}
