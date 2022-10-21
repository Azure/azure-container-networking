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

// MarkStepRunningInBackground should be called if a step will run in the background.
// MarkStepComplete must be called afterwards.
func (tt *TestCase) MarkStepRunningInBackground(stepID string) {
	tt.lock.Lock()
	defer tt.lock.Unlock()

	wg := new(sync.WaitGroup)
	wg.Add(1)
	tt.waitGroups[stepID] = wg
}

func (tt *TestCase) MarkStepComplete(stepID string) {
	tt.lock.Lock()
	defer tt.lock.Unlock()

	wg, ok := tt.waitGroups[stepID]
	if ok {
		wg.Done()
	}
}

func (tt *TestCase) WaitToRunStep(step string) {
	// don't keep tt locked while waiting on a single wait group

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

func (tt *TestCase) WaitForAllStepsToComplete() {
	tt.lock.Lock()
	defer tt.lock.Unlock()
	for _, wg := range tt.waitGroups {
		wg.Wait()
	}
}

type TestStep struct {
	ID           string
	InBackground bool
	*Action
}

type Action struct {
	HNSAction
	DPAction
}

type HNSAction interface {
	Do(hns *hnswrapper.Hnsv2wrapperFake) error
}

type DPAction interface {
	Do(dp *dataplane.DataPlane) error
}
