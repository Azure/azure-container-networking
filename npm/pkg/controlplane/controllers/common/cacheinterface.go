package common

import (
	"errors"
	"net"
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
		return PODNAME
	}
}

const (
	// IPADDRS indicates the IP Address input type
	IPADDRS InputType = 0
	// PODNAME indicates the podname input type
	PODNAME InputType = 1
	// EXTERNAL indicates the external input type
	EXTERNAL InputType = 2
)

type Cache interface {
	GetPod(*Input) (*NpmPod, error)
	GetNamespaceLabel(namespace string, key string) string
	GetListMap() map[string]string
	GetSetMap() map[string]string
}
