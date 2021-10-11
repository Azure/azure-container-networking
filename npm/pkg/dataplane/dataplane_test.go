package dataplane

import (
	"testing"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
	testutils "github.com/Azure/azure-container-networking/test/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	ioShim = common.NewMockIOShim([]testutils.TestCmd{})
	ns1Set = &ipsets.TranslatedIPSet{
		Metadata: ipsets.NewIPSetMetadata("setns1", ipsets.NameSpace),
	}
	testPolicyobj = &policies.NPMNetworkPolicy{
		Name: "ns1/testpolicy",
		PodSelectorIPSets: map[string]*ipsets.TranslatedIPSet{
			"setns1": ns1Set,
			"setpodkey1": {
				Metadata: ipsets.NewIPSetMetadata("setpodkey1", ipsets.KeyLabelOfPod),
			},
			"setpodkeyval1": {
				Metadata: ipsets.NewIPSetMetadata("setpodkeyval1", ipsets.KeyValueLabelOfPod),
			},
			"nestedset1": {
				Metadata: ipsets.NewIPSetMetadata("nestedset1", ipsets.NestedLabelOfPod),
				MemberIPSets: map[string]*ipsets.TranslatedIPSet{
					"setns1": ns1Set,
				},
			},
		},
		RuleIPSets: map[string]*ipsets.TranslatedIPSet{
			"setns2": {
				Metadata: ipsets.NewIPSetMetadata("setns2", ipsets.NameSpace),
			},
			"setpodkey2": {
				Metadata: ipsets.NewIPSetMetadata("setpodkey2", ipsets.KeyLabelOfPod),
			},
			"setpodkeyval2": {
				Metadata: ipsets.NewIPSetMetadata("setpodkeyval2", ipsets.KeyValueLabelOfPod),
			},
			"testcidr1": {
				Metadata: ipsets.NewIPSetMetadata("testcidr1", ipsets.CIDRBlocks),
				IPPodKey: map[string]string{
					"10.0.0.0": "val1",
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
)

func TestNewDataPlane(t *testing.T) {
	metrics.InitializeAll()
	dp := NewDataPlane("testnode", ioShim)

	if dp == nil {
		t.Error("NewDataPlane() returned nil")
	}

	setMetadata := ipsets.NewIPSetMetadata("test", ipsets.NameSpace)
	dp.CreateIPSet(setMetadata)
}

func TestInitializeDataPlane(t *testing.T) {
	metrics.InitializeAll()
	dp := NewDataPlane("testnode", ioShim)

	assert.NotNil(t, dp)
	err := dp.InitializeDataPlane()
	require.NoError(t, err)
}

func TestResetDataPlane(t *testing.T) {
	metrics.InitializeAll()
	dp := NewDataPlane("testnode", ioShim)

	assert.NotNil(t, dp)
	err := dp.InitializeDataPlane()
	require.NoError(t, err)
	err = dp.ResetDataPlane()
	require.NoError(t, err)
}

func TestCreateAndDeleteIpSets(t *testing.T) {
	metrics.InitializeAll()
	dp := NewDataPlane("testnode", ioShim)
	assert.NotNil(t, dp)
	setsTocreate := []*ipsets.IPSetMetadata{
		{
			Name: "test",
			Type: ipsets.NameSpace,
		},
		{
			Name: "test1",
			Type: ipsets.NameSpace,
		},
	}

	for _, v := range setsTocreate {
		dp.CreateIPSet(v)
	}

	// Creating again to see if duplicates get created
	for _, v := range setsTocreate {
		dp.CreateIPSet(v)
	}

	for _, v := range setsTocreate {
		prefixedName := v.GetPrefixName()
		set := dp.ipsetMgr.GetIPSet(prefixedName)
		assert.NotNil(t, set)
	}

	for _, v := range setsTocreate {
		dp.DeleteIPSet(v)
	}

	for _, v := range setsTocreate {
		prefixedName := v.GetPrefixName()
		set := dp.ipsetMgr.GetIPSet(prefixedName)
		assert.Nil(t, set)
	}
}

func TestAddToSet(t *testing.T) {
	metrics.InitializeAll()
	dp := NewDataPlane("testnode", ioShim)

	setsTocreate := []*ipsets.IPSetMetadata{
		{
			Name: "test",
			Type: ipsets.NameSpace,
		},
		{
			Name: "test1",
			Type: ipsets.NameSpace,
		},
	}

	for _, v := range setsTocreate {
		dp.CreateIPSet(v)
	}

	for _, v := range setsTocreate {
		prefixedName := v.GetPrefixName()
		set := dp.ipsetMgr.GetIPSet(prefixedName)
		assert.NotNil(t, set)
	}

	err := dp.AddToSet(setsTocreate, "10.0.0.1", "testns/a")
	require.NoError(t, err)

	// Test IPV6 addess it should error out
	err = dp.AddToSet(setsTocreate, "2001:db8:0:0:0:0:2:1", "testns/a")
	require.Error(t, err)

	for _, v := range setsTocreate {
		dp.DeleteIPSet(v)
	}

	for _, v := range setsTocreate {
		prefixedName := v.GetPrefixName()
		set := dp.ipsetMgr.GetIPSet(prefixedName)
		assert.NotNil(t, set)
	}

	err = dp.RemoveFromSet(setsTocreate, "10.0.0.1", "testns/a")
	require.NoError(t, err)

	for _, v := range setsTocreate {
		dp.DeleteIPSet(v)
	}

	for _, v := range setsTocreate {
		prefixedName := v.GetPrefixName()
		set := dp.ipsetMgr.GetIPSet(prefixedName)
		assert.Nil(t, set)
	}
}

func TestApplyPolicy(t *testing.T) {
	metrics.InitializeAll()
	dp := NewDataPlane("testnode", ioShim)

	err := dp.AddPolicy(testPolicyobj)
	require.NoError(t, err)
}

func TestRemovePolicy(t *testing.T) {
	metrics.InitializeAll()
	dp := NewDataPlane("testnode", ioShim)

	err := dp.AddPolicy(testPolicyobj)
	require.NoError(t, err)

	err = dp.RemovePolicy(testPolicyobj.Name)
	require.NoError(t, err)
}

func TestUpdatePolicy(t *testing.T) {
	metrics.InitializeAll()
	dp := NewDataPlane("testnode", ioShim)

	err := dp.AddPolicy(testPolicyobj)
	require.NoError(t, err)

	testPolicyobj.ACLs = []*policies.ACLPolicy{
		{
			PolicyID:  "testpol1",
			Target:    policies.Dropped,
			Direction: policies.Ingress,
		},
	}

	err = dp.UpdatePolicy(testPolicyobj)
	require.NoError(t, err)
}
