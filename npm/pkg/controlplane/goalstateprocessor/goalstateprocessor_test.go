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
	"k8s.io/apimachinery/pkg/util/wait"
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
		Namespace:   "x",
		PolicyKey:   "x/test-netpol",
		ACLPolicyID: "azure-acl-x-test-netpol",
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
				Target:    policies.Dropped,
				Direction: policies.Ingress,
			},
			{
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

func TestPolicyApplyEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dp := dpmocks.NewMockGenericDataplane(ctrl)
	// Verify that the policy was applied
	dp.EXPECT().UpdatePolicy(gomock.Any()).Times(1)
	dp.EXPECT().ApplyDataPlane().Times(1)

	inputChan := make(chan *protos.Events)
	payload, err := controlplane.EncodeNPMNetworkPolicies([]*policies.NPMNetworkPolicy{testNetPol})
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gsp, _ := NewGoalStateProcessor(ctx, "node1", "pod1", inputChan, dp)

	go func() {
		inputChan <- &protos.Events{
			EventType: protos.Events_GoalState,
			Payload: map[string]*protos.GoalState{
				controlplane.PolicyApply: {
					Data: payload.Bytes(),
				},
			},
		}
	}()
	time.Sleep(sleepAfterChanSent)

	gsp.processNext(wait.NeverStop)
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
	gsp, _ := NewGoalStateProcessor(ctx, "node1", "pod1", inputChan, dp)
	go func() {
		inputChan <- &protos.Events{
			Payload: goalState,
		}
	}()
	time.Sleep(sleepAfterChanSent)

	gsp.processNext(wait.NeverStop)
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
	gsp, _ := NewGoalStateProcessor(ctx, "node1", "pod1", inputChan, dp)
	go func() {
		inputChan <- &protos.Events{
			EventType: protos.Events_GoalState,
			Payload:   goalState,
		}
	}()
	time.Sleep(sleepAfterChanSent)

	gsp.processNext(wait.NeverStop)

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
			EventType: protos.Events_GoalState,
			Payload:   goalState,
		}
	}()
	time.Sleep(sleepAfterChanSent)

	gsp.processNext(wait.NeverStop)
}

func getGoalStateForControllerSets(t *testing.T, sets []*controlplane.ControllerIPSets) map[string]*protos.GoalState {
	goalState := map[string]*protos.GoalState{
		controlplane.IpsetApply: {
			Data: []byte{},
		},
	}
	payload, err := controlplane.EncodeControllerIPSets(sets)
	assert.NoError(t, err)
	goalState[controlplane.IpsetApply].Data = payload.Bytes()
	return goalState
}
