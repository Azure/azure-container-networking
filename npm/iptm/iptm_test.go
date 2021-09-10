package iptm

import (
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/metrics/promutil"
	"github.com/Azure/azure-container-networking/npm/util"
	testutils "github.com/Azure/azure-container-networking/test/utils"
	"github.com/stretchr/testify/require"
)

var (
	initCalls = []testutils.TestCmd{
		{Cmd: []string{"iptables", "-w", "60", "-N", "AZURE-NPM"}},
		{Cmd: []string{"iptables", "-w", "60", "-N", "AZURE-NPM-ACCEPT"}},
		{Cmd: []string{"iptables", "-w", "60", "-N", "AZURE-NPM-INGRESS"}},
		{Cmd: []string{"iptables", "-w", "60", "-N", "AZURE-NPM-EGRESS"}},
		{Cmd: []string{"iptables", "-w", "60", "-N", "AZURE-NPM-INGRESS-PORT"}},
		{Cmd: []string{"iptables", "-w", "60", "-N", "AZURE-NPM-INGRESS-FROM"}},
		{Cmd: []string{"iptables", "-w", "60", "-N", "AZURE-NPM-EGRESS-PORT"}},
		{Cmd: []string{"iptables", "-w", "60", "-N", "AZURE-NPM-EGRESS-TO"}},
		{Cmd: []string{"iptables", "-w", "60", "-N", "AZURE-NPM-INGRESS-DROPS"}},
		{Cmd: []string{"iptables", "-w", "60", "-N", "AZURE-NPM-EGRESS-DROPS"}},
		{Cmd: []string{"iptables", "-w", "60", "-N", "AZURE-NPM"}},

		{Cmd: []string{"iptables", "-t", "filter", "-n", "--list", "FORWARD", "--line-numbers"}, Stdout: "3  "}, // THIS IS THE GREP CALL
		{Cmd: []string{"grep", "KUBE-SERVICES"}, Stdout: "4  "},

		{Cmd: []string{"iptables", "-w", "60", "-C", "FORWARD", "-j", "AZURE-NPM"}},
		{Cmd: []string{"iptables", "-t", "filter", "-n", "--list", "FORWARD", "--line-numbers"}, Stdout: "3  "}, // THIS IS THE GREP CALL
		{Cmd: []string{"grep", "AZURE-NPM"}, Stdout: "4  "},
		{Cmd: []string{"iptables", "-w", "60", "-D", "FORWARD", "-j", "AZURE-NPM"}},
		{Cmd: []string{"iptables", "-w", "60", "-I", "FORWARD", "3", "-j", "AZURE-NPM"}},

		{Cmd: []string{"iptables", "-w", "60", "-C", "AZURE-NPM", "-j", "AZURE-NPM-INGRESS"}}, // broken here
		{Cmd: []string{"iptables", "-w", "60", "-C", "AZURE-NPM", "-j", "AZURE-NPM-EGRESS"}},
		{Cmd: []string{"iptables", "-w", "60", "-C", "AZURE-NPM", "-j", "AZURE-NPM-ACCEPT", "-m", "mark", "--mark", "0x3000", "-m", "comment", "--comment", "ACCEPT-on-INGRESS-and-EGRESS-mark-0x3000"}},
		{Cmd: []string{"iptables", "-w", "60", "-C", "AZURE-NPM", "-j", "AZURE-NPM-ACCEPT", "-m", "mark", "--mark", "0x2000", "-m", "comment", "--comment", "ACCEPT-on-INGRESS-mark-0x2000"}},
		{Cmd: []string{"iptables", "-w", "60", "-C", "AZURE-NPM", "-j", "AZURE-NPM-ACCEPT", "-m", "mark", "--mark", "0x1000", "-m", "comment", "--comment", "ACCEPT-on-EGRESS-mark-0x1000"}},
		{Cmd: []string{"iptables", "-w", "60", "-C", "AZURE-NPM", "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT", "-m", "comment", "--comment", "ACCEPT-on-connection-state"}},
		{Cmd: []string{"iptables", "-w", "60", "-C", "AZURE-NPM-ACCEPT", "-j", "MARK", "--set-mark", "0x0", "-m", "comment", "--comment", "Clear-AZURE-NPM-MARKS"}},
		{Cmd: []string{"iptables", "-w", "60", "-C", "AZURE-NPM-ACCEPT", "-j", "ACCEPT", "-m", "comment", "--comment", "ACCEPT-All-packets"}},
		{Cmd: []string{"iptables", "-w", "60", "-C", "AZURE-NPM-INGRESS", "-j", "AZURE-NPM-INGRESS-PORT"}},
		{Cmd: []string{"iptables", "-w", "60", "-C", "AZURE-NPM-INGRESS", "-j", "RETURN", "-m", "mark", "--mark", "0x2000", "-m", "comment", "--comment", "RETURN-on-INGRESS-mark-0x2000"}},
		{Cmd: []string{"iptables", "-w", "60", "-C", "AZURE-NPM-INGRESS", "-j", "AZURE-NPM-INGRESS-DROPS"}},
		{Cmd: []string{"iptables", "-w", "60", "-C", "AZURE-NPM-INGRESS-PORT", "-j", "RETURN", "-m", "mark", "--mark", "0x2000", "-m", "comment", "--comment", "RETURN-on-INGRESS-mark-0x2000"}},
		///////////
		{Cmd: []string{"iptables", "-w", "60", "-C", "AZURE-NPM-INGRESS-PORT", "-j", "AZURE-NPM-INGRESS-FROM", "-m", "comment", "--comment", "ALL-JUMP-TO-AZURE-NPM-INGRESS-FROM"}},
		{Cmd: []string{"iptables", "-w", "60", "-C", "AZURE-NPM-EGRESS", "-j", "AZURE-NPM-EGRESS-PORT"}},
		{Cmd: []string{"iptables", "-w", "60", "-C", "AZURE-NPM-EGRESS", "-j", "RETURN", "-m", "mark", "--mark", "0x3000", "-m", "comment", "--comment", "RETURN-on-EGRESS-and-INGRESS-mark-0x3000"}},
		{Cmd: []string{"iptables", "-w", "60", "-C", "AZURE-NPM-EGRESS", "-j", "RETURN", "-m", "mark", "--mark", "0x1000", "-m", "comment", "--comment", "RETURN-on-EGRESS-mark-0x1000"}},
		{Cmd: []string{"iptables", "-w", "60", "-C", "AZURE-NPM-EGRESS", "-j", "AZURE-NPM-EGRESS-DROPS"}},
		{Cmd: []string{"iptables", "-w", "60", "-C", "AZURE-NPM-EGRESS-PORT", "-j", "RETURN", "-m", "mark", "--mark", "0x3000", "-m", "comment", "--comment", "RETURN-on-EGRESS-and-INGRESS-mark-0x3000"}},
		{Cmd: []string{"iptables", "-w", "60", "-C", "AZURE-NPM-EGRESS-PORT", "-j", "RETURN", "-m", "mark", "--mark", "0x1000", "-m", "comment", "--comment", "RETURN-on-EGRESS-mark-0x1000"}},
		{Cmd: []string{"iptables", "-w", "60", "-C", "AZURE-NPM-EGRESS-PORT", "-j", "AZURE-NPM-EGRESS-TO", "-m", "comment", "--comment", "ALL-JUMP-TO-AZURE-NPM-EGRESS-TO"}},
		{Cmd: []string{"iptables", "-w", "60", "-C", "AZURE-NPM-INGRESS-DROPS", "-j", "RETURN", "-m", "mark", "--mark", "0x2000", "-m", "comment", "--comment", "RETURN-on-INGRESS-mark-0x2000"}},
		{Cmd: []string{"iptables", "-w", "60", "-C", "AZURE-NPM-EGRESS-DROPS", "-j", "RETURN", "-m", "mark", "--mark", "0x3000", "-m", "comment", "--comment", "RETURN-on-EGRESS-and-INGRESS-mark-0x3000"}},
		{Cmd: []string{"iptables", "-w", "60", "-C", "AZURE-NPM-EGRESS-DROPS", "-j", "RETURN", "-m", "mark", "--mark", "0x1000", "-m", "comment", "--comment", "RETURN-on-EGRESS-mark-0x1000"}},
	}

	unInitCalls = []testutils.TestCmd{
		{Cmd: []string{"iptables", "-w", "60", "-D", "FORWARD", "-j", "AZURE-NPM"}},
		{Cmd: []string{"iptables", "-w", "60", "-F", "AZURE-NPM"}},
		{Cmd: []string{"iptables", "-w", "60", "-F", "AZURE-NPM-ACCEPT"}},
		{Cmd: []string{"iptables", "-w", "60", "-F", "AZURE-NPM-INGRESS"}},
		{Cmd: []string{"iptables", "-w", "60", "-F", "AZURE-NPM-EGRESS"}},
		{Cmd: []string{"iptables", "-w", "60", "-F", "AZURE-NPM-INGRESS-PORT"}},
		{Cmd: []string{"iptables", "-w", "60", "-F", "AZURE-NPM-INGRESS-FROM"}},
		{Cmd: []string{"iptables", "-w", "60", "-F", "AZURE-NPM-EGRESS-PORT"}},
		{Cmd: []string{"iptables", "-w", "60", "-F", "AZURE-NPM-EGRESS-TO"}},
		{Cmd: []string{"iptables", "-w", "60", "-F", "AZURE-NPM-INGRESS-DROPS"}},
		{Cmd: []string{"iptables", "-w", "60", "-F", "AZURE-NPM-EGRESS-DROPS"}},
		{Cmd: []string{"iptables", "-w", "60", "-F", "AZURE-NPM-TARGET-SETS"}},
		{Cmd: []string{"iptables", "-w", "60", "-F", "AZURE-NPM-INRGESS-DROPS"}}, // can we remove this rule now?
		{Cmd: []string{"iptables", "-w", "60", "-X", "AZURE-NPM"}},
		{Cmd: []string{"iptables", "-w", "60", "-X", "AZURE-NPM-ACCEPT"}},
		{Cmd: []string{"iptables", "-w", "60", "-X", "AZURE-NPM-INGRESS"}},
		{Cmd: []string{"iptables", "-w", "60", "-X", "AZURE-NPM-EGRESS"}},
		{Cmd: []string{"iptables", "-w", "60", "-X", "AZURE-NPM-INGRESS-PORT"}},
		{Cmd: []string{"iptables", "-w", "60", "-X", "AZURE-NPM-INGRESS-FROM"}},
		{Cmd: []string{"iptables", "-w", "60", "-X", "AZURE-NPM-EGRESS-PORT"}},
		{Cmd: []string{"iptables", "-w", "60", "-X", "AZURE-NPM-EGRESS-TO"}},
		{Cmd: []string{"iptables", "-w", "60", "-X", "AZURE-NPM-INGRESS-DROPS"}},
		{Cmd: []string{"iptables", "-w", "60", "-X", "AZURE-NPM-EGRESS-DROPS"}},
		{Cmd: []string{"iptables", "-w", "60", "-X", "AZURE-NPM-TARGET-SETS"}},
		{Cmd: []string{"iptables", "-w", "60", "-X", "AZURE-NPM-INRGESS-DROPS"}}, // can we delete this rule now?
	}
)

func TestInitNpmChains(t *testing.T) {
	calls := initCalls

	fexec := testutils.GetFakeExecWithScripts(calls)
	defer testutils.VerifyCalls(t, fexec, calls)
	iptMgr := NewIptablesManager(fexec, NewFakeIptOperationShim())

	err := iptMgr.InitNpmChains()
	require.NoError(t, err)
}

func TestUninitNpmChains(t *testing.T) {
	calls := unInitCalls

	fexec := testutils.GetFakeExecWithScripts(calls)
	defer testutils.VerifyCalls(t, fexec, calls)
	iptMgr := NewIptablesManager(fexec, NewFakeIptOperationShim())

	if err := iptMgr.UninitNpmChains(); err != nil {
		t.Errorf("TestUninitNpmChains @ iptMgr.UninitNpmChains")
	}
}

func TestExists(t *testing.T) {
	calls := []testutils.TestCmd{
		{Cmd: []string{"iptables", "-w", "60", "-C", "FORWARD", "-j", "ACCEPT"}},
	}

	fexec := testutils.GetFakeExecWithScripts(calls)
	defer testutils.VerifyCalls(t, fexec, calls)
	iptMgr := NewIptablesManager(fexec, NewFakeIptOperationShim())

	iptMgr.OperationFlag = util.IptablesCheckFlag
	entry := &IptEntry{
		Chain: util.IptablesForwardChain,
		Specs: []string{
			util.IptablesJumpFlag,
			util.IptablesAccept,
		},
	}
	if _, err := iptMgr.exists(entry); err != nil {
		t.Errorf("TestExists failed @ iptMgr.exists")
	}
}

func TestAddChain(t *testing.T) {
	calls := []testutils.TestCmd{
		{Cmd: []string{"iptables", "-w", "60", "-N", "TEST-CHAIN"}},
	}

	fexec := testutils.GetFakeExecWithScripts(calls)
	defer testutils.VerifyCalls(t, fexec, calls)
	iptMgr := NewIptablesManager(fexec, NewFakeIptOperationShim())

	if err := iptMgr.addChain("TEST-CHAIN"); err != nil {
		t.Errorf("TestAddChain failed @ iptMgr.addChain")
	}
}

func TestDeleteChain(t *testing.T) {
	calls := []testutils.TestCmd{
		{Cmd: []string{"iptables", "-w", "60", "-N", "TEST-CHAIN"}},
		{Cmd: []string{"iptables", "-w", "60", "-X", "TEST-CHAIN"}},
	}

	fexec := testutils.GetFakeExecWithScripts(calls)
	defer testutils.VerifyCalls(t, fexec, calls)
	iptMgr := NewIptablesManager(fexec, NewFakeIptOperationShim())

	if err := iptMgr.addChain("TEST-CHAIN"); err != nil {
		t.Errorf("TestDeleteChain failed @ iptMgr.addChain")
	}

	if err := iptMgr.deleteChain("TEST-CHAIN"); err != nil {
		t.Errorf("TestDeleteChain failed @ iptMgr.deleteChain")
	}
}

func TestAdd(t *testing.T) {
	calls := []testutils.TestCmd{
		{Cmd: []string{"iptables", "-w", "60", "-I", "FORWARD", "-j", "REJECT"}},
	}

	fexec := testutils.GetFakeExecWithScripts(calls)
	defer testutils.VerifyCalls(t, fexec, calls)
	iptMgr := NewIptablesManager(fexec, NewFakeIptOperationShim())

	entry := &IptEntry{
		Chain: util.IptablesForwardChain,
		Specs: []string{
			util.IptablesJumpFlag,
			util.IptablesReject,
		},
	}

	gaugeVal, err1 := promutil.GetValue(metrics.NumIPTableRules)
	countVal, err2 := promutil.GetCountValue(metrics.AddIPTableRuleExecTime)

	if err := iptMgr.Add(entry); err != nil {
		t.Errorf("TestAdd failed @ iptMgr.Add")
	}

	newGaugeVal, err3 := promutil.GetValue(metrics.NumIPTableRules)
	newCountVal, err4 := promutil.GetCountValue(metrics.AddIPTableRuleExecTime)
	promutil.NotifyIfErrors(t, err1, err2, err3, err4)
	if newGaugeVal != gaugeVal+1 {
		t.Errorf("Change in iptable rule number didn't register in prometheus")
	}
	if newCountVal != countVal+1 {
		t.Errorf("Execution time didn't register in prometheus")
	}
}

func TestDelete(t *testing.T) {
	calls := []testutils.TestCmd{
		{Cmd: []string{"iptables", "-w", "60", "-I", "FORWARD", "-j", "REJECT"}},
		{Cmd: []string{"iptables", "-w", "60", "-C", "FORWARD", "-j", "REJECT"}},
		{Cmd: []string{"iptables", "-w", "60", "-D", "FORWARD", "-j", "REJECT"}},
	}

	fexec := testutils.GetFakeExecWithScripts(calls)
	defer testutils.VerifyCalls(t, fexec, calls)
	iptMgr := NewIptablesManager(fexec, NewFakeIptOperationShim())

	entry := &IptEntry{
		Chain: util.IptablesForwardChain,
		Specs: []string{
			util.IptablesJumpFlag,
			util.IptablesReject,
		},
	}
	if err := iptMgr.Add(entry); err != nil {
		t.Errorf("TestDelete failed @ iptMgr.Add")
	}

	gaugeVal, err1 := promutil.GetValue(metrics.NumIPTableRules)

	if err := iptMgr.Delete(entry); err != nil {
		t.Errorf("TestDelete failed @ iptMgr.Delete")
	}

	newGaugeVal, err2 := promutil.GetValue(metrics.NumIPTableRules)
	promutil.NotifyIfErrors(t, err1, err2)
	if newGaugeVal != gaugeVal-1 {
		t.Errorf("Change in iptable rule number didn't register in prometheus")
	}
}

func TestRun(t *testing.T) {
	calls := []testutils.TestCmd{
		{Cmd: []string{"iptables", "-w", "60", "-N", "TEST-CHAIN"}},
	}

	fexec := testutils.GetFakeExecWithScripts(calls)
	defer testutils.VerifyCalls(t, fexec, calls)
	iptMgr := NewIptablesManager(fexec, NewFakeIptOperationShim())

	iptMgr.OperationFlag = util.IptablesChainCreationFlag
	entry := &IptEntry{
		Chain: "TEST-CHAIN",
	}
	if _, err := iptMgr.run(entry); err != nil {
		t.Errorf("TestRun failed @ iptMgr.run")
	}
}

func TestGetChainLineNumber(t *testing.T) {
	calls := []testutils.TestCmd{
		{Cmd: []string{"iptables", "-t", "filter", "-n", "--list", "FORWARD", "--line-numbers"}, Stdout: "3    AZURE-NPM  all  --  0.0.0.0/0            0.0.0.0/0  "}, // expected output from iptables
		{Cmd: []string{"grep", "AZURE-NPM"}},
	}

	fexec := testutils.GetFakeExecWithScripts(calls)
	defer testutils.VerifyCalls(t, fexec, calls)
	iptMgr := NewIptablesManager(fexec, NewFakeIptOperationShim())

	lineNum, err := iptMgr.getChainLineNumber(util.IptablesAzureChain, util.IptablesForwardChain)
	require.NoError(t, err)
	require.Equal(t, lineNum, 3)
}

func TestMain(m *testing.M) {
	metrics.InitializeAll()

	exitCode := m.Run()

	os.Exit(exitCode)
}
