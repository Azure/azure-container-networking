package dataplane

import (
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
	"github.com/Azure/azure-container-networking/npm/util"
	testutils "github.com/Azure/azure-container-networking/test/utils"
	"github.com/stretchr/testify/require"
	"k8s.io/klog"
)

var netpolInBackgroundCfg = &Config{
	debug:              true,
	NetPolInBackground: true,
	MaxPendingNetPols:  3,
	NetPolInterval:     time.Duration(15 * time.Millisecond),
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

type linuxPromVals struct {
	restoreCreateCalls    int
	restoreDeleteCalls    int
	restoreCreateFailures int
	restoreDeleteFailures int
	iptablesDeleteCalls   int
}

func (l linuxPromVals) assert(t *testing.T) {
	v, err := metrics.TotalIPTablesRestoreLatencyCalls(metrics.CreateOp)
	require.NoError(t, err)
	require.Equal(t, l.restoreCreateCalls, v, "incorrect iptables restore create calls")

	v, err = metrics.TotalIPTablesRestoreLatencyCalls(metrics.DeleteOp)
	require.NoError(t, err)
	require.Equal(t, l.restoreDeleteCalls, v, "incorrect iptables restore delete calls")

	v, err = metrics.TotalIPTablesRestoreFailures(metrics.CreateOp)
	require.NoError(t, err)
	require.Equal(t, l.restoreCreateFailures, v, "incorrect iptables restore create failures")

	v, err = metrics.TotalIPTablesRestoreFailures(metrics.DeleteOp)
	require.NoError(t, err)
	require.Equal(t, l.restoreDeleteFailures, v, "incorrect iptables restore delete failures")

	v, err = metrics.TotalIPTablesDeleteLatencyCalls()
	require.NoError(t, err)
	require.Equal(t, l.iptablesDeleteCalls, v, "incorrect iptables delete calls")
}

func TestNetPolInBackgroundUpdatePolicy(t *testing.T) {
	metrics.ReinitializeAll()

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
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)

	stopCh := make(chan struct{}, 1)
	dp, err := NewDataPlane("testnode", ioshim, netpolInBackgroundCfg, stopCh)
	require.NoError(t, err)
	defer func() {
		stopCh <- struct{}{}
		time.Sleep(2000 * time.Millisecond)
		klog.Info("defer for TestNetPolInBackgroundUpdatePolicy finished")
	}()

	dp.RunPeriodicTasks()

	err = dp.AddPolicy(&testPolicyobj)
	require.NoError(t, err)

	time.Sleep(2000 * time.Millisecond)

	err = dp.UpdatePolicy(&updatedTestPolicyobj)
	require.NoError(t, err)

	time.Sleep(2000 * time.Millisecond)

	linuxPromVals{2, 1, 0, 0, 1}.assert(t)
}

func TestNetPolInBackgroundSkipAddAfterRemove(t *testing.T) {
	metrics.ReinitializeAll()

	calls := getBootupTestCalls()
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	stopCh := make(chan struct{}, 1)
	dp, err := NewDataPlane("testnode", ioshim, netpolInBackgroundCfg, stopCh)
	require.NoError(t, err)
	defer func() {
		stopCh <- struct{}{}
		time.Sleep(100 * time.Millisecond)
		klog.Info("defer for TestNetPolInBackgroundSkipAddAfterRemove finished")
	}()

	require.NoError(t, dp.AddPolicy(&testPolicyobj))
	require.NoError(t, dp.RemovePolicy(testPolicyobj.PolicyKey))

	dp.RunPeriodicTasks()
	time.Sleep(100 * time.Millisecond)

	// nothing happens
	linuxPromVals{0, 0, 0, 0, 0}.assert(t)
}

func TestNetPolInBackgroundFailureToAddFirstTime(t *testing.T) {
	metrics.ReinitializeAll()

	testPolicy2 := testPolicyobj
	testPolicy2.PolicyKey = "testpolicy2"
	testPolicy3 := testPolicyobj
	testPolicy3.PolicyKey = "testpolicy3"

	calls := getBootupTestCalls()
	calls = append(calls,
		testutils.TestCmd{
			Cmd:      []string{"ipset", "restore"},
			Stdout:   "success",
			ExitCode: 0,
		},
		// restore will try twice per pMgr.AddPolicies() call
		testutils.TestCmd{
			Cmd:      []string{"iptables-nft-restore", "-w", "60", "-T", "filter", "--noflush"},
			ExitCode: 1,
		},
		testutils.TestCmd{
			Cmd:      []string{"iptables-nft-restore", "-w", "60", "-T", "filter", "--noflush"},
			ExitCode: 1,
		},
		// first policy succeeds
		testutils.TestCmd{
			Cmd:      []string{"iptables-nft-restore", "-w", "60", "-T", "filter", "--noflush"},
			ExitCode: 0,
		},
		// second policy succeeds
		testutils.TestCmd{
			Cmd:      []string{"iptables-nft-restore", "-w", "60", "-T", "filter", "--noflush"},
			ExitCode: 0,
		},
		// third policy fails
		// restore will try twice per pMgr.AddPolicies() call
		testutils.TestCmd{
			Cmd:      []string{"iptables-nft-restore", "-w", "60", "-T", "filter", "--noflush"},
			ExitCode: 1,
		},
		testutils.TestCmd{
			Cmd:      []string{"iptables-nft-restore", "-w", "60", "-T", "filter", "--noflush"},
			ExitCode: 1,
		},
	)
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	stopCh := make(chan struct{}, 1)
	dp, err := NewDataPlane("testnode", ioshim, netpolInBackgroundCfg, stopCh)
	require.NoError(t, err)
	defer func() {
		stopCh <- struct{}{}
		time.Sleep(100 * time.Millisecond)
	}()

	require.NoError(t, dp.AddPolicy(&testPolicyobj))
	require.NoError(t, dp.AddPolicy(&testPolicy2))
	require.NoError(t, dp.AddPolicy(&testPolicy3))
	// will reach max pending policies of 3

	linuxPromVals{4, 0, 2, 0, 0}.assert(t)

	require.Equal(t, 1, dp.netPolQueue.len(), "expected one netpol to still be in the queue after it fails when adding one at a time")
}
