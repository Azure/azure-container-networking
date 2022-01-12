package goalstateprocessor

import (
	"context"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/npm/pkg/controlplane"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	dpmocks "github.com/Azure/azure-container-networking/npm/pkg/dataplane/mocks"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
	"github.com/Azure/azure-container-networking/npm/pkg/protos"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

const (
	sleepAfterChanSent = time.Millisecond * 100
)

var (
	testNSSet             = ipsets.NewIPSetMetadata("test-ns-set", ipsets.Namespace)
	testNSCPSet           = controlplane.NewControllerIPSets(testNSSet)
	testKeyPodSet         = ipsets.NewIPSetMetadata("test-keyPod-set", ipsets.KeyLabelOfPod)
	testKeyPodCPSet       = controlplane.NewControllerIPSets(testKeyPodSet)
	testNestedKeyPodSet   = ipsets.NewIPSetMetadata("test-nestedkeyPod-set", ipsets.NestedLabelOfPod)
	testNestedKeyPodCPSet = controlplane.NewControllerIPSets(testNestedKeyPodSet)
	testNetPol            = &policies.NPMNetworkPolicy{
		Name:      "test-netpol",
		NameSpace: "x",
		PolicyKey: "x/test-netpol",
		PodSelectorIPSets: []*ipsets.TranslatedIPSet{
			{
				Metadata: testNSSet,
			},
			{
				Metadata: testKeyPodSet,
			},
		},
		RuleIPSets: []*ipsets.TranslatedIPSet{
			{
				Metadata: testNSSet,
			},
			{
				Metadata: testKeyPodSet,
			},
		},
		ACLs: []*policies.ACLPolicy{
			{
				PolicyID:  "azure-acl-123",
				Target:    policies.Dropped,
				Direction: policies.Ingress,
			},
			{
				PolicyID:  "azure-acl-234",
				Target:    policies.Allowed,
				Direction: policies.Ingress,
				SrcList: []policies.SetInfo{
					{
						IPSet:     testNSSet,
						Included:  true,
						MatchType: policies.SrcMatch,
					},
					{
						IPSet:     testKeyPodSet,
						Included:  true,
						MatchType: policies.SrcMatch,
					},
				},
			},
		},
		PodEndpoints: map[string]string{
			"10.0.0.1": "1234",
		},
	}
)

func TestNewGoalStateProcessor(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dp := dpmocks.NewMockGenericDataplane(ctrl)
	// Verify that the policy was applied
	dp.EXPECT().UpdatePolicy(gomock.Any()).Times(1)
	dp.EXPECT().ApplyDataPlane().Times(1)

	inputChan := make(chan *protos.Events)
	payload, err := controlplane.EncodeNPMNetworkPolicy(testNetPol)
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gsp := NewGoalStateProcessor(ctx, "node1", "pod1", inputChan, dp)

	go func() {
		inputChan <- &protos.Events{
			Payload: map[string]*protos.GoalState{
				controlplane.PolicyApply: {
					Data: [][]byte{payload.Bytes()},
				},
			},
		}
	}()
	time.Sleep(sleepAfterChanSent)

	gsp.processNext()
}

func TestIPSetsApply(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dp := dpmocks.NewMockGenericDataplane(ctrl)
	// Verify that the policy was applied
	dp.EXPECT().GetIPSet(gomock.Any()).Times(3)
	dp.EXPECT().CreateIPSets(gomock.Any()).Times(3)
	dp.EXPECT().ApplyDataPlane().Times(1)

	inputChan := make(chan *protos.Events)

	goalState := getGoalStateForControllerSets(t,
		[]*controlplane.ControllerIPSets{
			testNSCPSet,
			testKeyPodCPSet,
			testNestedKeyPodCPSet,
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gsp := NewGoalStateProcessor(ctx, "node1", "pod1", inputChan, dp)
	go func() {
		inputChan <- &protos.Events{
			Payload: goalState,
		}
	}()
	time.Sleep(sleepAfterChanSent)

	gsp.processNext()
}

func TestIPSetsApplyUpdateMembers(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dp := dpmocks.NewMockGenericDataplane(ctrl)
	// Verify that the policy was applied
	dp.EXPECT().GetIPSet(gomock.Any()).Times(4)
	dp.EXPECT().CreateIPSets(gomock.Any()).Times(1)
	dp.EXPECT().AddToSets(gomock.Any(), gomock.Any()).Times(2)
	dp.EXPECT().AddToLists(gomock.Any(), gomock.Any()).Times(1)
	dp.EXPECT().ApplyDataPlane().Times(2)

	inputChan := make(chan *protos.Events)

	testNSCPSet.IPPodMetadata = map[string]*dataplane.PodMetadata{
		"10.0.0.1": dataplane.NewPodMetadata("test", "10.0.0.1", "1234"),
	}
	testNestedKeyPodCPSet.MemberIPSets = map[string]*ipsets.IPSetMetadata{
		testNSSet.GetPrefixName(): testNSSet,
	}
	goalState := getGoalStateForControllerSets(t,
		[]*controlplane.ControllerIPSets{
			testNSCPSet,
			testKeyPodCPSet,
			testNestedKeyPodCPSet,
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gsp := NewGoalStateProcessor(ctx, "node1", "pod1", inputChan, dp)
	go func() {
		inputChan <- &protos.Events{
			Payload: goalState,
		}
	}()
	time.Sleep(sleepAfterChanSent)

	gsp.processNext()

	// Update one of the ipsets and send another event
	testNSCPSet.IPPodMetadata = map[string]*dataplane.PodMetadata{
		"10.0.0.2": dataplane.NewPodMetadata("test2", "10.0.0.2", "1234"),
	}
	goalState = getGoalStateForControllerSets(t,
		[]*controlplane.ControllerIPSets{
			testNSCPSet,
		},
	)
	go func() {
		inputChan <- &protos.Events{
			Payload: goalState,
		}
	}()
	time.Sleep(sleepAfterChanSent)

	gsp.processNext()
}

func getGoalStateForControllerSets(t *testing.T, sets []*controlplane.ControllerIPSets) map[string]*protos.GoalState {
	goalState := map[string]*protos.GoalState{
		controlplane.IpsetApply: {
			Data: [][]byte{},
		},
	}
	for _, set := range sets {
		payload, err := controlplane.EncodeControllerIPSet(set)
		assert.NoError(t, err)
		goalState[controlplane.IpsetApply].Data = append(goalState[controlplane.IpsetApply].Data, payload.Bytes())
	}
	return goalState
}
