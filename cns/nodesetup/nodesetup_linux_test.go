// Copyright Microsoft. All rights reserved.
// MIT License

package nodesetup

import (
	"net"
	"testing"

	"github.com/Azure/azure-container-networking/cns/configuration"
	"github.com/Azure/azure-container-networking/cns/iprule"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/sys/unix"
)

func TestIPRulesForDst(t *testing.T) {
	rules, err := ipRulesForDst("168.63.129.16", wireserverRulePriority)
	require.NoError(t, err)
	require.Len(t, rules, 1)

	rule := rules[0]
	assert.Equal(t, "168.63.129.16/32", rule.Dst.String())
	assert.Equal(t, unix.RT_TABLE_MAIN, rule.Table)
	assert.Equal(t, wireserverRulePriority, rule.Priority)
}

var (
	errMockRuleList = errors.New("mock rule list error")
	errMockRuleAdd  = errors.New("mock rule add error")
)

func TestRun(t *testing.T) {
	wireserverCIDR := "168.63.129.16/32"
	_, wireserverNet, _ := net.ParseCIDR(wireserverCIDR)

	tests := []struct {
		name          string
		expectedErr   string
		expectedAdded int
		listFn        func() ([]iprule.IPRule, error)
		addFn         func(iprule.IPRule) error
	}{
		{
			name:          "adds wireserver rule when rule does not exist",
			expectedAdded: 1,
			listFn:        func() ([]iprule.IPRule, error) { return nil, nil },
		},
		{
			name:          "skips wireserver rule when it already exists (idempotency)",
			expectedAdded: 0,
			listFn: func() ([]iprule.IPRule, error) {
				return []iprule.IPRule{
					{Dst: wireserverNet, Table: unix.RT_TABLE_MAIN, Priority: wireserverRulePriority},
				}, nil
			},
		},
		{
			name:        "returns error when list fails",
			expectedErr: "failed to list existing ip rules",
			listFn:      func() ([]iprule.IPRule, error) { return nil, errMockRuleList },
		},
		{
			name:        "returns error when add fails",
			expectedErr: "failed to add ip rule",
			listFn:      func() ([]iprule.IPRule, error) { return nil, nil },
			addFn:       func(_ iprule.IPRule) error { return errMockRuleAdd },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var addedRules []iprule.IPRule

			origList := listIPRules
			origAdd := addIPRule
			defer func() {
				listIPRules = origList
				addIPRule = origAdd
			}()

			listIPRules = tt.listFn
			if tt.addFn != nil {
				addIPRule = tt.addFn
			} else {
				addIPRule = func(rule iprule.IPRule) error {
					addedRules = append(addedRules, rule)
					return nil
				}
			}

			err := New(&configuration.CNSConfig{WireserverIP: "168.63.129.16"}, zap.NewNop()).Run()

			if tt.expectedErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr)
			} else {
				require.NoError(t, err)
			}

			assert.Len(t, addedRules, tt.expectedAdded)
			if tt.expectedAdded > 0 {
				assert.Equal(t, wireserverCIDR, addedRules[0].Dst.String())
				assert.Equal(t, unix.RT_TABLE_MAIN, addedRules[0].Table)
				assert.Equal(t, wireserverRulePriority, addedRules[0].Priority)
			}
		})
	}
}
