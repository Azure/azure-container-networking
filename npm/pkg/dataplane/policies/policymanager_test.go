package policies

import (
	"testing"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/stretchr/testify/require"
)

var (
	// below epList is no-op for linux
	epList        = map[string]string{"10.0.0.1": "test123", "10.0.0.2": "test456"}
	testNSSet     = ipsets.NewIPSetMetadata("test-ns-set", ipsets.Namespace)
	testKeyPodSet = ipsets.NewIPSetMetadata("test-keyPod-set", ipsets.KeyLabelOfPod)
	testNetPol    = &NPMNetworkPolicy{
		Name: "test/test-netpol",
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
				PolicyID:  "azure-acl-123",
				Target:    Dropped,
				Direction: Ingress,
			},
			{
				PolicyID:  "azure-acl-234",
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
)

func TestAddPolicy(t *testing.T) {
	netpol := &NPMNetworkPolicy{}

	calls := GetAddPolicyTestCalls(netpol)
	pMgr := NewPolicyManager(common.NewMockIOShim(calls))

	require.NoError(t, pMgr.AddPolicy(netpol, epList))

	require.NoError(t, pMgr.AddPolicy(testNetPol, epList))
}

func TestGetPolicy(t *testing.T) {
	netpol := &NPMNetworkPolicy{
		Name: "test",
		ACLs: []*ACLPolicy{
			{
				PolicyID:  "azure-acl-123",
				Target:    Dropped,
				Direction: Ingress,
			},
		},
	}

	calls := GetAddPolicyTestCalls(netpol)
	pMgr := NewPolicyManager(common.NewMockIOShim(calls))

	require.NoError(t, pMgr.AddPolicy(netpol, epList))

	require.True(t, pMgr.PolicyExists("test"))

	policy, ok := pMgr.GetPolicy("test")
	require.True(t, ok)
	require.Equal(t, "test", policy.Name)
}

func TestRemovePolicy(t *testing.T) {
	calls := append(GetAddPolicyTestCalls(testNetPol), GetRemovePolicyTestCalls(testNetPol)...)
	pMgr := NewPolicyManager(common.NewMockIOShim(calls))

	require.NoError(t, pMgr.AddPolicy(testNetPol, epList))

	require.NoError(t, pMgr.RemovePolicy("test", epList))

	require.NoError(t, pMgr.RemovePolicy("test/test-netpol", nil))
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
				PolicyID:  "valid-acl",
				Target:    Dropped,
				Direction: Ingress,
			},
			wantErr: false,
		},
		{
			name: "invalid protocol",
			acl: &ACLPolicy{
				PolicyID:  "bad-protocol-acl",
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
				Name: "test-netpol",
				ACLs: []*ACLPolicy{tt.acl},
			}
			normalizePolicy(netPol)
			err := validatePolicy(netPol)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
