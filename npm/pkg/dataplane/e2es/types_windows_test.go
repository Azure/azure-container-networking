package e2e

import (
	"sync"

	"github.com/Azure/azure-container-networking/network/hnswrapper"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane"
	"github.com/Microsoft/hcsshim/hcn"
)

type Tag string

type TestCaseMetadata struct {
	Description      string
	Tags             []Tag
	InitialEndpoints []*hcn.HostComputeEndpoint
	// DpCfg needs to be set for NPM tests only
	DpCfg *dataplane.Config
	*TestCase
	ExpectedSetPolicies  []*hcn.SetPolicySetting
	ExpectedEnpdointACLs map[string][]*hnswrapper.FakeEndpointPolicy
}

type TestCase struct {
	Steps []*TestStep
	// waitToComplete maps a Step to the Steps it should wait to complete before it runs.
	waitToComplete map[string][]string
	// waitGroups maps a Step to its WaitGroup so that we can see when a Step has completed.
	waitGroups map[string]*sync.WaitGroup
	// lock is held while accessing the above maps
	lock sync.Mutex
}

func NewTestCase(steps []*TestStep, waitToComplete map[string][]string) *TestCase {
	if waitToComplete == nil {
		waitToComplete = make(map[string][]string)
	}

	return &TestCase{
		Steps:          steps,
		waitToComplete: waitToComplete,
		waitGroups:     make(map[string]*sync.WaitGroup),
	}
}

// AddStepWaitGroup tracks the wait group for the step.
// The caller is expected to call wg.Done().
func (tt *TestCase) AddStepWaitGroup(step string, wg *sync.WaitGroup) {
	tt.lock.Lock()
	defer tt.lock.Unlock()
	tt.waitGroups[step] = wg
}

func (tt *TestCase) WaitToRun(step string) {
	tt.lock.Lock()
	stepsToWaitOn, ok := tt.waitToComplete[step]
	tt.lock.Unlock()
	if !ok {
		return
	}

	for _, s := range stepsToWaitOn {
		tt.lock.Lock()
		wg, ok := tt.waitGroups[s]
		tt.lock.Unlock()
		if !ok {
			continue
		}
		wg.Wait()
	}
}

func (tt *TestCase) WaitForAll() {
	tt.lock.Lock()
	defer tt.lock.Unlock()
	for _, wg := range tt.waitGroups {
		wg.Wait()
	}
}

type TestStep struct {
	ID           string
	InBackground bool
	Action
}

type Action interface {
	Do() error
	SetHNS(hns *hnswrapper.Hnsv2wrapperFake)
	// SetDP needs to be implemented for NPM tests only
	SetDP(dp *dataplane.DataPlane)
}

// HNSAction is meant to be embedded in other Actions so that they don't have to implement SetHNS and SetDP.
// HNSAction does not implement Do().
type HNSAction struct {
	hns *hnswrapper.Hnsv2wrapperFake
}

func (h *HNSAction) SetHNS(hns *hnswrapper.Hnsv2wrapperFake) {
	h.hns = hns
}

func (h *HNSAction) SetDP(_ *dataplane.DataPlane) {
	// purposely not implemented
}

// DPAction is meant to be embedded in other Actions so that they don't have to implement SetHNS and SetDP.
// DPAction does not implement Do().
type DPAction struct {
	dp *dataplane.DataPlane
}

func (d *DPAction) SetHNS(_ *hnswrapper.Hnsv2wrapperFake) {
	// purposely not implemented
}

func (d *DPAction) SetDP(dp *dataplane.DataPlane) {
	d.dp = dp
}
