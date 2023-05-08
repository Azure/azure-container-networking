package dataplane

import (
	"fmt"
	"testing"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
	"github.com/Azure/azure-container-networking/npm/util"
	testutils "github.com/Azure/azure-container-networking/test/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	nodeName = "testnode"

	dpCfg = &Config{
		IPSetManagerCfg: &ipsets.IPSetManagerCfg{
			IPSetMode:   ipsets.ApplyAllIPSets,
			NetworkName: "azure",
		},
		PolicyManagerCfg: &policies.PolicyManagerCfg{
			NodeIP:               "6.7.8.9",
			PolicyMode:           policies.IPSetPolicyMode,
			PlaceAzureChainFirst: util.PlaceAzureChainFirst,
		},
	}

	setPodKey1 = &ipsets.TranslatedIPSet{
		Metadata: ipsets.NewIPSetMetadata("setpodkey1", ipsets.KeyLabelOfPod),
	}
	testPolicyobj = policies.NPMNetworkPolicy{
		Namespace:   "ns1",
		PolicyKey:   "ns1/testpolicy",
		ACLPolicyID: "azure-acl-ns1-testpolicy",
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
				Target:    policies.Dropped,
				Direction: policies.Egress,
			},
		},
	}
)

func TestNewDataPlane(t *testing.T) {
	metrics.InitializeAll()

	calls := getBootupTestCalls()
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	dp, err := NewDataPlane("testnode", ioshim, dpCfg, nil)
	require.NoError(t, err)
	assert.NotNil(t, dp)
}

func TestCreateAndDeleteIpSets(t *testing.T) {
	metrics.InitializeAll()

	calls := getBootupTestCalls()
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	dp, err := NewDataPlane("testnode", ioshim, dpCfg, nil)
	require.NoError(t, err)
	assert.NotNil(t, dp)
	setsTocreate := []*ipsets.IPSetMetadata{
		{
			Name: "test",
			Type: ipsets.Namespace,
		},
		{
			Name: "test1",
			Type: ipsets.Namespace,
		},
	}

	dp.CreateIPSets(setsTocreate)

	// Creating again to see if duplicates get created
	dp.CreateIPSets(setsTocreate)

	for _, v := range setsTocreate {
		prefixedName := v.GetPrefixName()
		set := dp.ipsetMgr.GetIPSet(prefixedName)
		assert.NotNil(t, set)
	}

	for _, v := range setsTocreate {
		dp.DeleteIPSet(v, util.SoftDelete)
	}

	for _, v := range setsTocreate {
		prefixedName := v.GetPrefixName()
		set := dp.ipsetMgr.GetIPSet(prefixedName)
		assert.Nil(t, set)
	}
}

func TestAddToSet(t *testing.T) {
	metrics.InitializeAll()

	calls := getBootupTestCalls()
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	dp, err := NewDataPlane("testnode", ioshim, dpCfg, nil)
	require.NoError(t, err)

	setsTocreate := []*ipsets.IPSetMetadata{
		{
			Name: "test",
			Type: ipsets.Namespace,
		},
		{
			Name: "test1",
			Type: ipsets.Namespace,
		},
	}

	dp.CreateIPSets(setsTocreate)

	for _, v := range setsTocreate {
		prefixedName := v.GetPrefixName()
		set := dp.ipsetMgr.GetIPSet(prefixedName)
		assert.NotNil(t, set)
	}

	podMetadata := NewPodMetadata("testns/a", "10.0.0.1", nodeName)
	err = dp.AddToSets(setsTocreate, podMetadata)
	require.NoError(t, err)

	v6PodMetadata := NewPodMetadata("testns/a", "2001:db8:0:0:0:0:2:1", nodeName)
	// Test IPV6 addess it should error out
	err = dp.AddToSets(setsTocreate, v6PodMetadata)
	require.Error(t, err)

	for _, v := range setsTocreate {
		dp.DeleteIPSet(v, util.SoftDelete)
	}

	for _, v := range setsTocreate {
		prefixedName := v.GetPrefixName()
		set := dp.ipsetMgr.GetIPSet(prefixedName)
		assert.NotNil(t, set)
	}

	err = dp.RemoveFromSets(setsTocreate, podMetadata)
	require.NoError(t, err)

	err = dp.RemoveFromSets(setsTocreate, v6PodMetadata)
	require.Error(t, err)

	for _, v := range setsTocreate {
		dp.DeleteIPSet(v, util.SoftDelete)
	}

	for _, v := range setsTocreate {
		prefixedName := v.GetPrefixName()
		set := dp.ipsetMgr.GetIPSet(prefixedName)
		assert.Nil(t, set)
	}
}

func TestApplyPolicy(t *testing.T) {
	metrics.InitializeAll()

	calls := append(getBootupTestCalls(), getAddPolicyTestCallsForDP(&testPolicyobj)...)
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	dp, err := NewDataPlane("testnode", ioshim, dpCfg, nil)
	require.NoError(t, err)

	err = dp.AddPolicy(&testPolicyobj)
	require.NoError(t, err)
}

func TestRemovePolicy(t *testing.T) {
	metrics.InitializeAll()

	calls := append(getBootupTestCalls(), getAddPolicyTestCallsForDP(&testPolicyobj)...)
	calls = append(calls, getRemovePolicyTestCallsForDP(&testPolicyobj)...)
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	dp, err := NewDataPlane("testnode", ioshim, dpCfg, nil)
	require.NoError(t, err)

	err = dp.AddPolicy(&testPolicyobj)
	require.NoError(t, err)

	err = dp.RemovePolicy(testPolicyobj.PolicyKey)
	require.NoError(t, err)
}

func TestUpdatePolicy(t *testing.T) {
	metrics.InitializeAll()

	updatedTestPolicyobj := testPolicyobj
	updatedTestPolicyobj.ACLs = []*policies.ACLPolicy{
		{
			Target:    policies.Dropped,
			Direction: policies.Ingress,
		},
	}

	calls := append(getBootupTestCalls(), getAddPolicyTestCallsForDP(&testPolicyobj)...)
	calls = append(calls, getRemovePolicyTestCallsForDP(&testPolicyobj)...)
	calls = append(calls, getAddPolicyTestCallsForDP(&updatedTestPolicyobj)...)
	for _, call := range calls {
		fmt.Println(call)
	}
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	dp, err := NewDataPlane("testnode", ioshim, dpCfg, nil)
	require.NoError(t, err)

	err = dp.AddPolicy(&testPolicyobj)
	require.NoError(t, err)

	err = dp.UpdatePolicy(&updatedTestPolicyobj)
	require.NoError(t, err)
}

func TestUpdatePodCache(t *testing.T) {
	m1 := NewPodMetadata("x/a", "10.0.0.1", nodeName)
	m2 := NewPodMetadata("x/b", "10.0.0.2", nodeName)
	m3 := NewPodMetadata("x/c", "10.0.0.3", nodeName)

	c := newUpdatePodCache(3)
	require.True(t, c.isEmpty())

	p1 := c.enqueue(m1)
	require.False(t, c.isEmpty())
	require.Equal(t, *newUpdateNPMPod(m1), *p1)
	require.Equal(t, c.queue, []string{m1.PodKey})
	require.Equal(t, c.cache, map[string]*updateNPMPod{m1.PodKey: p1})

	p2 := c.enqueue(m2)
	require.False(t, c.isEmpty())
	require.Equal(t, *newUpdateNPMPod(m2), *p2)
	require.Equal(t, c.queue, []string{m1.PodKey, m2.PodKey})
	require.Equal(t, c.cache, map[string]*updateNPMPod{m1.PodKey: p1, m2.PodKey: p2})

	p3 := c.enqueue(m3)
	require.False(t, c.isEmpty())
	require.Equal(t, *newUpdateNPMPod(m3), *p3)
	require.Equal(t, c.queue, []string{m1.PodKey, m2.PodKey, m3.PodKey})
	require.Equal(t, c.cache, map[string]*updateNPMPod{m1.PodKey: p1, m2.PodKey: p2, m3.PodKey: p3})

	// Test that enqueueing an existing pod does not change the queue or cache.
	pairs := []struct {
		m *PodMetadata
		p *updateNPMPod
	}{
		{m1, p1},
		{m2, p2},
		{m3, p3},
	}
	for _, pair := range pairs {
		p := c.enqueue(pair.m)
		require.False(t, c.isEmpty())
		require.Equal(t, pair.p, p)
		require.Equal(t, c.queue, []string{m1.PodKey, m2.PodKey, m3.PodKey})
		require.Equal(t, c.cache, map[string]*updateNPMPod{m1.PodKey: p1, m2.PodKey: p2, m3.PodKey: p3})
	}

	// test dequeue
	p := c.dequeue()
	require.False(t, c.isEmpty())
	require.Equal(t, p1, p)
	require.Equal(t, c.queue, []string{m2.PodKey, m3.PodKey})
	require.Equal(t, c.cache, map[string]*updateNPMPod{m2.PodKey: p2, m3.PodKey: p3})

	p = c.dequeue()
	require.False(t, c.isEmpty())
	require.Equal(t, p2, p)
	require.Equal(t, c.queue, []string{m3.PodKey})
	require.Equal(t, c.cache, map[string]*updateNPMPod{m3.PodKey: p3})

	// test requeuing
	c.requeue(p)
	require.False(t, c.isEmpty())
	require.Equal(t, c.queue, []string{m3.PodKey, m2.PodKey})
	require.Equal(t, c.cache, map[string]*updateNPMPod{m3.PodKey: p3, m2.PodKey: p2})

	p = c.dequeue()
	require.False(t, c.isEmpty())
	require.Equal(t, p3, p)
	require.Equal(t, c.queue, []string{m2.PodKey})
	require.Equal(t, c.cache, map[string]*updateNPMPod{m2.PodKey: p2})

	// test enqueuing again
	p = c.enqueue(m1)
	require.Equal(t, *p1, *p)
	require.False(t, c.isEmpty())
	require.Equal(t, c.queue, []string{m2.PodKey, m1.PodKey})
	require.Equal(t, c.cache, map[string]*updateNPMPod{m2.PodKey: p2, m1.PodKey: p1})

	p = c.dequeue()
	require.False(t, c.isEmpty())
	require.Equal(t, p2, p)
	require.Equal(t, c.queue, []string{m1.PodKey})
	require.Equal(t, c.cache, map[string]*updateNPMPod{m1.PodKey: p1})

	p = c.dequeue()
	require.True(t, c.isEmpty())
	require.Equal(t, p1, p)
	require.Equal(t, c.queue, []string{})
	require.Equal(t, c.cache, map[string]*updateNPMPod{})

	// test requeue on empty queue
	c.requeue(p)
	require.False(t, c.isEmpty())
	require.Equal(t, c.queue, []string{m1.PodKey})
	require.Equal(t, c.cache, map[string]*updateNPMPod{m1.PodKey: p1})

	p = c.dequeue()
	require.True(t, c.isEmpty())
	require.Equal(t, p1, p)
	require.Equal(t, c.queue, []string{})
	require.Equal(t, c.cache, map[string]*updateNPMPod{})

	// test nil result on empty queue
	p = c.dequeue()
	require.True(t, c.isEmpty())
	require.Nil(t, p)

	// test enqueue on empty queue
	p = c.enqueue(m3)
	require.False(t, c.isEmpty())
	require.Equal(t, *p3, *p)
	require.Equal(t, c.queue, []string{m3.PodKey})
	require.Equal(t, c.cache, map[string]*updateNPMPod{m3.PodKey: p})

	// test enqueue with different node on only item in queue
	m3Node2 := *m3
	m3Node2.NodeName = "node2"
	p3Node2 := *newUpdateNPMPod(&m3Node2)
	p = c.enqueue(&m3Node2)
	require.False(t, c.isEmpty())
	require.Equal(t, p3Node2, *p)
	require.Equal(t, c.queue, []string{m3Node2.PodKey})
	require.Equal(t, c.cache, map[string]*updateNPMPod{m3Node2.PodKey: p})

	// test enqueue with different node on first item in queue
	p = c.enqueue(m1)
	require.False(t, c.isEmpty())
	require.Equal(t, *p1, *p)
	require.Equal(t, c.queue, []string{m3Node2.PodKey, m1.PodKey})
	require.Equal(t, c.cache, map[string]*updateNPMPod{m3Node2.PodKey: &p3Node2, m1.PodKey: p1})

	p = c.enqueue(m3)
	require.False(t, c.isEmpty())
	require.Equal(t, *p3, *p)
	require.Equal(t, c.queue, []string{m1.PodKey, m3.PodKey})
	require.Equal(t, c.cache, map[string]*updateNPMPod{m1.PodKey: p1, m3.PodKey: p})

	// test enqueue with different node on last item in queue
	p = c.enqueue(&m3Node2)
	require.False(t, c.isEmpty())
	require.Equal(t, p3Node2, *p)
	require.Equal(t, c.queue, []string{m1.PodKey, m3Node2.PodKey})
	require.Equal(t, c.cache, map[string]*updateNPMPod{m1.PodKey: p1, m3Node2.PodKey: p})
}

func getBootupTestCalls() []testutils.TestCmd {
	return append(policies.GetBootupTestCalls(true), ipsets.GetResetTestCalls()...)
}

func getAddPolicyTestCallsForDP(networkPolicy *policies.NPMNetworkPolicy) []testutils.TestCmd {
	toAddOrUpdateSets := getAffectedIPSets(networkPolicy)
	calls := ipsets.GetApplyIPSetsTestCalls(toAddOrUpdateSets, nil)
	calls = append(calls, policies.GetAddPolicyTestCalls(networkPolicy)...)
	return calls
}

func getRemovePolicyTestCallsForDP(networkPolicy *policies.NPMNetworkPolicy) []testutils.TestCmd {
	// NOTE toDeleteSets is only correct if these ipsets are referenced by no other policy in iMgr
	toDeleteSets := getAffectedIPSets(networkPolicy)
	calls := policies.GetRemovePolicyTestCalls(networkPolicy)
	calls = append(calls, ipsets.GetApplyIPSetsTestCalls(nil, toDeleteSets)...)
	return calls
}

func getAffectedIPSets(networkPolicy *policies.NPMNetworkPolicy) []*ipsets.IPSetMetadata {
	sets := make([]*ipsets.IPSetMetadata, 0)
	for _, translatedIPSet := range networkPolicy.PodSelectorIPSets {
		sets = append(sets, translatedIPSet.Metadata)
	}
	for _, translatedIPSet := range networkPolicy.RuleIPSets {
		sets = append(sets, translatedIPSet.Metadata)
	}
	return sets
}
