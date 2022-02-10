package dpshim

import (
	"bytes"
	"reflect"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/npm/pkg/controlplane"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
	"github.com/Azure/azure-container-networking/npm/pkg/protos"
	"github.com/Azure/azure-container-networking/npm/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	sleepAfterChanSent = time.Millisecond * 10
	testSetName        = "test-set"
	testListName       = "test-list"
)

var (
	testNSSet             = ipsets.NewIPSetMetadata("test-ns-set", ipsets.Namespace)
	testKeyPodSet         = ipsets.NewIPSetMetadata("test-keyPod-set", ipsets.KeyLabelOfPod)
	testNestedKeyPodSet   = ipsets.NewIPSetMetadata("test-nestedkeyPod-set", ipsets.NestedLabelOfPod)
	testNestedKeyPodCPSet = controlplane.NewControllerIPSets(testNestedKeyPodSet)
	setPodKey1            = &ipsets.TranslatedIPSet{
		Metadata: ipsets.NewIPSetMetadata("setpodkey1", ipsets.KeyLabelOfPod),
	}
	testPolicyobj = &policies.NPMNetworkPolicy{
		Name:      "testpolicy",
		NameSpace: "ns1",
		PolicyKey: "ns1/testpolicy",
		PodSelectorIPSets: []*ipsets.TranslatedIPSet{
			{
				Metadata: ipsets.NewIPSetMetadata("setns1", ipsets.Namespace),
			},
			setPodKey1,
			{
				Metadata: ipsets.NewIPSetMetadata("nestedset1", ipsets.NestedLabelOfPod),
				Members: []string{
					"setpodkey1",
				},
			},
		},
		RuleIPSets: []*ipsets.TranslatedIPSet{
			{
				Metadata: ipsets.NewIPSetMetadata("setns2", ipsets.Namespace),
			},
			{
				Metadata: ipsets.NewIPSetMetadata("setpodkey2", ipsets.KeyLabelOfPod),
			},
			{
				Metadata: ipsets.NewIPSetMetadata("setpodkeyval2", ipsets.KeyValueLabelOfPod),
			},
			{
				Metadata: ipsets.NewIPSetMetadata("testcidr1", ipsets.CIDRBlocks),
				Members: []string{
					"10.0.0.0/8",
				},
			},
		},
		ACLs: []*policies.ACLPolicy{
			{
				PolicyID:  "testpol1",
				Target:    policies.Dropped,
				Direction: policies.Egress,
			},
		},
	}
	podMetadata = &dataplane.PodMetadata{
		PodKey:   "a",
		PodIP:    "10.0.0.0",
		NodeName: "",
	}
)

func TestAddToList(t *testing.T) {
	dp, err := NewDPSim(nil)
	require.NoError(t, err)

	setMetadata := ipsets.NewIPSetMetadata(testSetName, ipsets.Namespace)
	listMetadata := ipsets.NewIPSetMetadata(testListName, ipsets.KeyLabelOfNamespace)
	dp.CreateIPSets([]*ipsets.IPSetMetadata{setMetadata, listMetadata})

	err = dp.AddToLists([]*ipsets.IPSetMetadata{listMetadata}, []*ipsets.IPSetMetadata{setMetadata})
	require.NoError(t, err)

	set := dp.getCachedIPSet(listMetadata.GetPrefixName())
	assert.NotNil(t, set)
	assert.Equal(t, listMetadata.GetPrefixName(), set.GetPrefixName())
	assert.Equal(t, util.GetHashedName(listMetadata.GetPrefixName()), set.GetHashedName())
	assert.Equal(t, 1, len(set.MemberIPSets))
	assert.Equal(t, setMetadata.GetPrefixName(), set.MemberIPSets[setMetadata.GetPrefixName()].GetPrefixName())

	err = dp.ApplyDataPlane()
	require.NoError(t, err)

	payload := getPayload(t, dp.OutChannel, controlplane.IpsetApply)
	sets, err := controlplane.DecodeControllerIPSets(payload)
	require.NoError(t, err)
	assert.Equal(t, 2, len(sets))
}

func TestRemoveFromList(t *testing.T) {
	dp, err := NewDPSim(nil)
	require.NoError(t, err)

	dp.CreateIPSets([]*ipsets.IPSetMetadata{testKeyPodSet, testNestedKeyPodSet})

	err = dp.AddToLists([]*ipsets.IPSetMetadata{testNestedKeyPodSet}, []*ipsets.IPSetMetadata{testKeyPodSet})
	require.NoError(t, err)

	set := dp.getCachedIPSet(testNestedKeyPodCPSet.GetPrefixName())
	assert.NotNil(t, set)
	assert.Equal(t, testNestedKeyPodCPSet.GetPrefixName(), set.GetPrefixName())
	assert.Equal(t, util.GetHashedName(testNestedKeyPodCPSet.GetPrefixName()), set.GetHashedName())
	assert.Equal(t, 1, len(set.MemberIPSets))
	assert.Equal(t, testKeyPodSet.GetPrefixName(), set.MemberIPSets[testKeyPodSet.GetPrefixName()].GetPrefixName())

	err = dp.ApplyDataPlane()
	require.NoError(t, err)

	payload := getPayload(t, dp.OutChannel, controlplane.IpsetApply)
	sets, err := controlplane.DecodeControllerIPSets(payload)
	require.NoError(t, err)
	assert.Equal(t, 2, len(sets))

	err = dp.RemoveFromList(testNestedKeyPodSet, []*ipsets.IPSetMetadata{testKeyPodSet})
	require.NoError(t, err)

	set = dp.getCachedIPSet(testNestedKeyPodCPSet.GetPrefixName())
	assert.NotNil(t, set)
	assert.Equal(t, 0, len(set.MemberIPSets))

	err = dp.ApplyDataPlane()
	require.NoError(t, err)

	payload = getPayload(t, dp.OutChannel, controlplane.IpsetApply)
	sets, err = controlplane.DecodeControllerIPSets(payload)
	require.NoError(t, err)
	assert.Equal(t, 1, len(sets))
	assert.Equal(t, util.GetHashedName(testNestedKeyPodCPSet.GetPrefixName()), sets[0].GetHashedName())
}

func TestAddToSets(t *testing.T) {
	dp, err := NewDPSim(nil)
	require.NoError(t, err)

	err = dp.AddToSets([]*ipsets.IPSetMetadata{
		testKeyPodSet,
		testNSSet,
	},
		podMetadata,
	)
	require.NoError(t, err)

	err = dp.ApplyDataPlane()
	require.NoError(t, err)

	payload := getPayload(t, dp.OutChannel, controlplane.IpsetApply)
	sets, err := controlplane.DecodeControllerIPSets(payload)
	require.NoError(t, err)
	assert.Equal(t, 2, len(sets))
}

func TestRemoveFromSet(t *testing.T) {
	dp, err := NewDPSim(nil)
	require.NoError(t, err)

	setMetadata := ipsets.NewIPSetMetadata(testSetName, ipsets.Namespace)
	err = dp.AddToSets([]*ipsets.IPSetMetadata{setMetadata}, podMetadata)
	require.NoError(t, err)

	err = dp.ApplyDataPlane()
	require.NoError(t, err)

	payload := getPayload(t, dp.OutChannel, controlplane.IpsetApply)
	sets, err := controlplane.DecodeControllerIPSets(payload)
	require.NoError(t, err)
	assert.Equal(t, 1, len(sets))

	err = dp.RemoveFromSets([]*ipsets.IPSetMetadata{setMetadata}, podMetadata)
	require.NoError(t, err)

	err = dp.ApplyDataPlane()
	require.Nil(t, err)

	payload = getPayload(t, dp.OutChannel, controlplane.IpsetApply)
	sets, err = controlplane.DecodeControllerIPSets(payload)
	require.Nil(t, err)
	require.Equal(t, 1, len(sets))
}

func TestPolicyUpdateEvent(t *testing.T) {
	dp, err := NewDPSim(nil)
	require.NoError(t, err)

	err = dp.UpdatePolicy(testPolicyobj)
	require.NoError(t, err)
	assert.True(t, dp.policyExists(testPolicyobj.PolicyKey))

	payload := getPayload(t, dp.OutChannel, controlplane.PolicyApply)
	netpols, err := controlplane.DecodeNPMNetworkPolicies(payload)
	require.NoError(t, err)
	assert.Equal(t, 1, len(netpols))

	assert.True(t, reflect.DeepEqual(netpols[0], testPolicyobj))
}

func getPayload(t *testing.T, outChan chan *protos.Events, key string) *bytes.Buffer {
	time.Sleep(sleepAfterChanSent)
	for {
		select {
		case event := <-outChan:
			gs := event.GetPayload()

			goalState, ok := gs[key]
			assert.True(t, ok)
			return bytes.NewBuffer(goalState.GetData())
		default:
			t.Error("Policy not applied")
		}
	}
}
