package dpshim

import (
	"errors"
	"fmt"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
	"github.com/Azure/azure-container-networking/npm/pkg/protos"
)

// TODO setting this up to unblock another workitem
type DPShim struct {
	outChannel chan *protos.Events
}

var ErrChannelUnset = errors.New("channel must be set")

func NewDPSim(outChannel chan *protos.Events) (*DPShim, error) {
	if outChannel == nil {
		return nil, fmt.Errorf("out channel must be set: %w", ErrChannelUnset)
	}
	return &DPShim{outChannel: outChannel}, nil
}

func (dp *DPShim) InitializeDataPlane() error {
	return nil
}

func (dp *DPShim) ResetDataPlane() error {
	return nil
}

func (dp *DPShim) RunPeriodicTasks() {}

func (dp *DPShim) GetIPSet(setName string) *ipsets.IPSet {
	return nil
}

func (dp *DPShim) CreateIPSets(setNames []*ipsets.IPSetMetadata) {
}

func (dp *DPShim) DeleteIPSet(setMetadata *ipsets.IPSetMetadata) {
}

func (dp *DPShim) AddToSets(setNames []*ipsets.IPSetMetadata, podMetadata *dataplane.PodMetadata) error {
	return nil
}

func (dp *DPShim) RemoveFromSets(setNames []*ipsets.IPSetMetadata, podMetadata *dataplane.PodMetadata) error {
	return nil
}

func (dp *DPShim) AddToLists(listName, setNames []*ipsets.IPSetMetadata) error {
	return nil
}

func (dp *DPShim) RemoveFromList(listName *ipsets.IPSetMetadata, setNames []*ipsets.IPSetMetadata) error {
	return nil
}

func (dp *DPShim) ApplyDataPlane() error {
	return nil
}

func (dp *DPShim) AddPolicy(networkpolicies *policies.NPMNetworkPolicy) error {
	return nil
}

func (dp *DPShim) RemovePolicy(policyName string) error {
	return nil
}

func (dp *DPShim) UpdatePolicy(networkpolicies *policies.NPMNetworkPolicy) error {
	return nil
}
