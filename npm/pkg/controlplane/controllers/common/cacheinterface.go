package common

import (
	"errors"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/pb"
)

// Input struct
type Input struct {
	Content string
	Type    InputType
}

// error type
var (
	ErrSetNotExist      = errors.New("set does not exists")
	ErrInvalidIPAddress = errors.New("invalid ipaddress, no equivalent pod found")
	ErrInvalidInput     = errors.New("invalid input")
	ErrSetType          = errors.New("invalid set type")
)

type TupleAndRule struct {
	Tuple *Tuple
	Rule *pb.RuleResponse
}

// Tuple struct
type Tuple struct {
	RuleType  string `json:"ruleType"`
	Direction string `json:"direction"`
	SrcIP     string `json:"srcIP"`
	SrcPort   string `json:"srcPort"`
	DstIP     string `json:"dstIP"`
	DstPort   string `json:"dstPort"`
	Protocol  string `json:"protocol"`
}

// InputType indicates allowed typle for source and destination input
type InputType int32

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
