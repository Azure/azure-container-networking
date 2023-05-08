package policies

import (
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/metrics/promutil"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/util"
	"github.com/stretchr/testify/require"
)

var (
	ipsetConfig = &PolicyManagerCfg{
		NodeIP:               "6.7.8.9",
		PolicyMode:           IPSetPolicyMode,
		PlaceAzureChainFirst: util.PlaceAzureChainFirst,
	}

	// below epList is no-op for linux
	epList = map[string]string{
		"10.0.0.1": "test123",
		"10.0.0.2": "test456",
	}
	epIDs = []string{
		"test123",
		"test456",
	}

	testNSSet     = ipsets.NewIPSetMetadata("test-ns-set", ipsets.Namespace)
	testKeyPodSet = ipsets.NewIPSetMetadata("test-keyPod-set", ipsets.KeyLabelOfPod)
)

type promVals struct {
	numACLs   int
	execCount int
}

// need this as a function so that PodEndpoints is reset everytime
func testNetworkPolicy() *NPMNetworkPolicy {
	return &NPMNetworkPolicy{
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
		ACLs: []*ACLPolicy{
			{
				Target:    Dropped,
				Direction: Ingress,
			},
			{
				Target:    Allowed,
				Direction: Ingress,
				SrcList: []SetInfo{
					{
						IPSet:     testNSSet,
						Included:  true,
						MatchType: SrcMatch,
					},
					{
						IPSet:     testKeyPodSet,
						Included:  true,
						MatchType: SrcMatch,
					},
				},
			},
		},
		PodEndpoints: map[string]string{
			"10.0.0.1": "1234",
		},
	}
}

func (p promVals) testPrometheusMetrics(t *testing.T) {
	numACLs, err := metrics.GetNumACLRules()
	promutil.NotifyIfErrors(t, err)
	require.Equal(t, p.numACLs, numACLs, "Prometheus didn't register correctly for num acls")

	execCount, err := metrics.GetACLRuleExecCount()
	promutil.NotifyIfErrors(t, err)
	require.Equal(t, p.execCount, execCount, "Prometheus didn't register correctly for acl rule exec count")
}

// see chain-management_linux_test.go for testing when an error occurs
func TestBootup(t *testing.T) {
	metrics.ReinitializeAll()
	calls := GetBootupTestCalls(false)
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	pMgr := NewPolicyManager(ioshim, ipsetConfig)

	metrics.IncNumACLRules()
	metrics.IncNumACLRules()

	require.NoError(t, pMgr.Bootup(epIDs))

	expectedNumACLs := 11
	if util.IsWindowsDP() {
		expectedNumACLs = 0
	}
	promVals{expectedNumACLs, 0}.testPrometheusMetrics(t)
}

// see policymanager_linux.go for testing when an error occurs
func TestAddPolicy(t *testing.T) {
	metrics.ReinitializeAll()
	testNetPol := testNetworkPolicy()
	calls := GetAddPolicyTestCalls(testNetPol)
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	pMgr := NewPolicyManager(ioshim, ipsetConfig)

	require.NoError(t, pMgr.AddPolicy(testNetPol, epList))
	_, ok := pMgr.GetPolicy(testNetPol.PolicyKey)
	require.True(t, ok)
	numTestNetPolACLRulesProducedInKernel := 3
	if util.IsWindowsDP() {
		numEndpoints := 2
		numTestNetPolACLRulesProducedInKernel++
		numTestNetPolACLRulesProducedInKernel *= numEndpoints
	}
	promVals{numTestNetPolACLRulesProducedInKernel, 1}.testPrometheusMetrics(t)
}

func TestAddEmptyPolicy(t *testing.T) {
	metrics.ReinitializeAll()
	testNetPol := testNetworkPolicy()
	ioshim := common.NewMockIOShim(nil)
	pMgr := NewPolicyManager(ioshim, ipsetConfig)
	require.NoError(t, pMgr.AddPolicy(&NPMNetworkPolicy{
		Namespace:   "x",
		PolicyKey:   "x/test-netpol",
		ACLPolicyID: "azure-acl-x-test-netpol",
	}, nil))
	_, ok := pMgr.GetPolicy(testNetPol.PolicyKey)
	require.False(t, ok)
	promVals{0, 0}.testPrometheusMetrics(t)
}

func TestGetPolicy(t *testing.T) {
	netpol := &NPMNetworkPolicy{
		Namespace:   "x",
		PolicyKey:   "x/test-netpol",
		ACLPolicyID: "azure-acl-x-test-netpol",
		ACLs: []*ACLPolicy{
			{
				Target:    Dropped,
				Direction: Ingress,
			},
		},
	}

	calls := GetAddPolicyTestCalls(netpol)
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	pMgr := NewPolicyManager(ioshim, ipsetConfig)

	require.NoError(t, pMgr.AddPolicy(netpol, epList))

	require.True(t, pMgr.PolicyExists("x/test-netpol"))

	policy, ok := pMgr.GetPolicy("x/test-netpol")
	require.True(t, ok)
	require.Equal(t, "x/test-netpol", policy.PolicyKey)
}

func TestRemovePolicy(t *testing.T) {
	metrics.ReinitializeAll()
	testNetPol := testNetworkPolicy()
	calls := append(GetAddPolicyTestCalls(testNetPol), GetRemovePolicyTestCalls(testNetPol)...)
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	pMgr := NewPolicyManager(ioshim, ipsetConfig)
	require.NoError(t, pMgr.AddPolicy(testNetPol, epList))
	require.NoError(t, pMgr.RemovePolicy(testNetPol.PolicyKey))
	_, ok := pMgr.GetPolicy(testNetPol.PolicyKey)
	require.False(t, ok)
	promVals{0, 1}.testPrometheusMetrics(t)
}

// see policymanager_linux.go for testing when an error occurs
func TestRemoveNonexistentPolicy(t *testing.T) {
	metrics.ReinitializeAll()
	ioshim := common.NewMockIOShim(nil)
	pMgr := NewPolicyManager(ioshim, ipsetConfig)
	require.NoError(t, pMgr.RemovePolicy("wrong-policy-key"))
	promVals{0, 0}.testPrometheusMetrics(t)
}

func TestNormalizeAndValidatePolicy(t *testing.T) {
	tests := []struct {
		name    string
		acl     *ACLPolicy
		wantErr bool
	}{
		{
			name: "valid policy",
			acl: &ACLPolicy{
				Target:    Dropped,
				Direction: Ingress,
			},
			wantErr: false,
		},
		{
			name: "invalid protocol",
			acl: &ACLPolicy{
				Target:    Dropped,
				Direction: Ingress,
				Protocol:  "invalid",
			},
			wantErr: true,
		},
		// TODO add other invalid cases
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			netPol := &NPMNetworkPolicy{
				Namespace:   "x",
				PolicyKey:   "x/test-netpol",
				ACLPolicyID: "azure-acl-x-test-netpol",
				ACLs:        []*ACLPolicy{tt.acl},
			}
			NormalizePolicy(netPol)
			err := ValidatePolicy(netPol)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestMain(m *testing.M) {
	metrics.InitializeAll()

	exitCode := m.Run()

	os.Exit(exitCode)
}
