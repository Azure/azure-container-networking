package iptm

import (
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/npm/fakes"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/metrics/promutil"
	"github.com/Azure/azure-container-networking/npm/util"
	testutils "github.com/Azure/azure-container-networking/test/utils"
	"k8s.io/utils/exec"
)

func TestSave(t *testing.T) {
	var calls = []testutils.TestCmd{
		{Cmd: []string{"iptables-save"}},
	}

	fexec, fcmd := testutils.GetFakeExecWithScripts(calls)
	iptMgr := NewIptablesManager(fexec, &fakes.FakeIptOperationShim{})

	if err := iptMgr.Save(util.IptablesTestConfigFile); err != nil {
		t.Errorf("TestSave failed @ iptMgr.Save")
	}

	testutils.VerifyCallsMatch(t, calls, fexec, fcmd)
}

func TestRestore(t *testing.T) {
	var calls = []testutils.TestCmd{
		{Cmd: []string{"iptables-restore"}},
	}

	fexec, fcmd := testutils.GetFakeExecWithScripts(calls)
	iptMgr := NewIptablesManager(fexec, fakes.NewFakeIptOperationShim())

	if err := iptMgr.Restore(util.IptablesTestConfigFile); err != nil {
		t.Errorf("TestRestore failed @ iptMgr.Restore with err %v", err)
	}

	testutils.VerifyCallsMatch(t, calls, fexec, fcmd)
}

func TestInitNpmChains(t *testing.T) {
	var calls = []testutils.TestCmd{
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
		{Cmd: []string{"iptables", "-w", "60", "-C", "FORWARD", "-j", "AZURE-NPM"}},
		{Cmd: []string{"iptables", "-w", "60", "-C", "AZURE-NPM", "-j", "AZURE-NPM-INGRESS"}},
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
		{Cmd: []string{"iptables", "-w", "60", "-C", "AZURE-NPM-INGRESS-FROM", "-j", "RETURN", "-m", "mark", "--mark", "0x2000", "-m", "comment", "--comment", "RETURN-on-INGRESS-mark-0x2000"}},
		{Cmd: []string{"iptables", "-w", "60", "-C", "AZURE-NPM-EGRESS", "-j", "AZURE-NPM-EGRESS-PORT"}},
		{Cmd: []string{"iptables", "-w", "60", "-C", "AZURE-NPM-EGRESS", "-j", "RETURN", "-m", "mark", "--mark", "0x3000", "-m", "comment", "--comment", "RETURN-on-EGRESS-and-INGRESS-mark-0x3000"}},
		{Cmd: []string{"iptables", "-w", "60", "-C", "AZURE-NPM-EGRESS", "-j", "RETURN", "-m", "mark", "--mark", "0x1000", "-m", "comment", "--comment", "RETURN-on-EGRESS-mark-0x1000"}},
		{Cmd: []string{"iptables", "-w", "60", "-C", "AZURE-NPM-EGRESS", "-j", "AZURE-NPM-EGRESS-DROPS"}},
		{Cmd: []string{"iptables", "-w", "60", "-C", "AZURE-NPM-EGRESS-PORT", "-j", "RETURN", "-m", "mark", "--mark", "0x3000", "-m", "comment", "--comment", "RETURN-on-EGRESS-and-INGRESS-mark-0x3000"}},
		{Cmd: []string{"iptables", "-w", "60", "-C", "AZURE-NPM-EGRESS-PORT", "-j", "RETURN", "-m", "mark", "--mark", "0x1000", "-m", "comment", "--comment", "RETURN-on-EGRESS-mark-0x1000"}},
		{Cmd: []string{"iptables", "-w", "60", "-C", "AZURE-NPM-EGRESS-TO", "-j", "RETURN", "-m", "mark", "--mark", "0x3000", "-m", "comment", "--comment", "RETURN-on-EGRESS-and-INGRESS-mark-0x3000"}},
		{Cmd: []string{"iptables", "-w", "60", "-C", "AZURE-NPM-EGRESS-TO", "-j", "RETURN", "-m", "mark", "--mark", "0x1000", "-m", "comment", "--comment", "RETURN-on-EGRESS-mark-0x1000"}},
	}

	fexec, fcmd := testutils.GetFakeExecWithScripts(calls)
	iptMgr := NewIptablesManager(fexec, fakes.NewFakeIptOperationShim())

	if err := iptMgr.InitNpmChains(); err != nil {
		t.Errorf("TestInitNpmChains @ iptMgr.InitNpmChains")
	}

	testutils.VerifyCallsMatch(t, calls, fexec, fcmd)
}

func TestUninitNpmChains(t *testing.T) {
	var calls = []testutils.TestCmd{
		{Cmd: []string{"iptables-save"}},
		{Cmd: []string{"iptables-restore"}},
		{Cmd: []string{"iptables-restore"}},
		{Cmd: []string{"iptables-restore"}},
		{Cmd: []string{"iptables-restore"}},
		{Cmd: []string{"iptables-restore"}},
		{Cmd: []string{"iptables-restore"}},
		{Cmd: []string{"iptables-restore"}},
		{Cmd: []string{"iptables-restore"}},
		{Cmd: []string{"iptables-save"}},
		{Cmd: []string{"iptables-restore"}},
		{Cmd: []string{"iptables-restore"}},
		{Cmd: []string{"iptables-restore"}},
		{Cmd: []string{"iptables-restore"}},
		{Cmd: []string{"iptables-restore"}},
		{Cmd: []string{"iptables-restore"}},
		{Cmd: []string{"iptables-restore"}},
		{Cmd: []string{"iptables-restore"}},
		{Cmd: []string{"iptables-save"}},
		{Cmd: []string{"iptables-restore"}},
		{Cmd: []string{"iptables-restore"}},
		{Cmd: []string{"iptables-restore"}},
		{Cmd: []string{"iptables-restore"}},
		{Cmd: []string{"iptables-restore"}},
		{Cmd: []string{"iptables-restore"}},
		{Cmd: []string{"iptables-restore"}},
		{Cmd: []string{"iptables-restore"}},
	}

	fexec, fcmd := testutils.GetFakeExecWithScripts(calls)
	iptMgr := NewIptablesManager(fexec, &fakes.FakeIptOperationShim{})

	if err := iptMgr.InitNpmChains(); err != nil {
		t.Errorf("TestUninitNpmChains @ iptMgr.InitNpmChains")
	}

	if err := iptMgr.UninitNpmChains(); err != nil {
		t.Errorf("TestUninitNpmChains @ iptMgr.UninitNpmChains")
	}

	testutils.VerifyCallsMatch(t, calls, fexec, fcmd)
}

func TestExists(t *testing.T) {
	iptMgr := &IptablesManager{}
	if err := iptMgr.Save(util.IptablesTestConfigFile); err != nil {
		t.Errorf("TestExists failed @ iptMgr.Save")
	}

	defer func() {
		if err := iptMgr.Restore(util.IptablesTestConfigFile); err != nil {
			t.Errorf("TestExists failed @ iptMgr.Restore")
		}
	}()

	iptMgr.OperationFlag = util.IptablesCheckFlag
	entry := &IptEntry{
		Chain: util.IptablesForwardChain,
		Specs: []string{
			util.IptablesJumpFlag,
			util.IptablesAccept,
		},
	}
	if _, err := iptMgr.Exists(entry); err != nil {
		t.Errorf("TestExists failed @ iptMgr.Exists")
	}
}

func TestAddChain(t *testing.T) {
	iptMgr := &IptablesManager{}
	if err := iptMgr.Save(util.IptablesTestConfigFile); err != nil {
		t.Errorf("TestAddChain failed @ iptMgr.Save")
	}

	defer func() {
		if err := iptMgr.Restore(util.IptablesTestConfigFile); err != nil {
			t.Errorf("TestAddChain failed @ iptMgr.Restore")
		}
	}()

	if err := iptMgr.AddChain("TEST-CHAIN"); err != nil {
		t.Errorf("TestAddChain failed @ iptMgr.AddChain")
	}
}

func TestDeleteChain(t *testing.T) {
	iptMgr := &IptablesManager{}
	if err := iptMgr.Save(util.IptablesTestConfigFile); err != nil {
		t.Errorf("TestDeleteChain failed @ iptMgr.Save")
	}

	defer func() {
		if err := iptMgr.Restore(util.IptablesTestConfigFile); err != nil {
			t.Errorf("TestDeleteChain failed @ iptMgr.Restore")
		}
	}()

	if err := iptMgr.AddChain("TEST-CHAIN"); err != nil {
		t.Errorf("TestDeleteChain failed @ iptMgr.AddChain")
	}

	if err := iptMgr.DeleteChain("TEST-CHAIN"); err != nil {
		t.Errorf("TestDeleteChain failed @ iptMgr.DeleteChain")
	}
}

func TestAdd(t *testing.T) {
	iptMgr := &IptablesManager{}
	if err := iptMgr.Save(util.IptablesTestConfigFile); err != nil {
		t.Errorf("TestAdd failed @ iptMgr.Save")
	}

	defer func() {
		if err := iptMgr.Restore(util.IptablesTestConfigFile); err != nil {
			t.Errorf("TestAdd failed @ iptMgr.Restore")
		}
	}()

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
	iptMgr := &IptablesManager{}
	if err := iptMgr.Save(util.IptablesTestConfigFile); err != nil {
		t.Errorf("TestDelete failed @ iptMgr.Save")
	}

	defer func() {
		if err := iptMgr.Restore(util.IptablesTestConfigFile); err != nil {
			t.Errorf("TestDelete failed @ iptMgr.Restore")
		}
	}()

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
	iptMgr := &IptablesManager{}
	if err := iptMgr.Save(util.IptablesTestConfigFile); err != nil {
		t.Errorf("TestRun failed @ iptMgr.Save")
	}

	defer func() {
		if err := iptMgr.Restore(util.IptablesTestConfigFile); err != nil {
			t.Errorf("TestRun failed @ iptMgr.Restore")
		}
	}()

	iptMgr.OperationFlag = util.IptablesChainCreationFlag
	entry := &IptEntry{
		Chain: "TEST-CHAIN",
	}
	if _, err := iptMgr.Run(entry); err != nil {
		t.Errorf("TestRun failed @ iptMgr.Run")
	}
}

func TestGetChainLineNumber(t *testing.T) {
	iptMgr := &IptablesManager{}

	var (
		lineNum    int
		err        error
		kubeExists bool
		npmExists  bool
	)

	if err = iptMgr.Save(util.IptablesTestConfigFile); err != nil {
		t.Errorf("TestGetChainLineNumber failed @ iptMgr.Save")
	}

	defer func() {
		if err = iptMgr.Restore(util.IptablesTestConfigFile); err != nil {
			t.Errorf("TestGetChainLineNumber failed @ iptMgr.Restore")
		}
	}()

	if err = iptMgr.AddChain(util.IptablesKubeServicesChain); err != nil {
		t.Errorf("TestGetChainLineNumber failed @ kube-services chain iptMgr.AddChain error: %s", err.Error())
	}

	iptMgr.OperationFlag = util.IptablesCheckFlag
	entry := &IptEntry{
		Chain: util.IptablesForwardChain,
		Specs: []string{
			util.IptablesJumpFlag,
			util.IptablesKubeServicesChain,
		},
	}

	if kubeExists, err = iptMgr.Exists(entry); err != nil {
		t.Errorf("TestGetChainLineNumber failed @ kube-services chain iptMgr.Exists error: %s", err.Error())
	}

	entry = &IptEntry{
		Chain: util.IptablesForwardChain,
		Specs: []string{
			util.IptablesJumpFlag,
			util.IptablesAzureChain,
		},
	}

	// Ignore not exists errors
	npmExists, _ = iptMgr.Exists(entry)

	lineNum, err = iptMgr.GetChainLineNumber(util.IptablesAzureChain, util.IptablesForwardChain)
	if err != nil {
		t.Errorf("TestGetChainLineNumber @ initial iptMgr.GetChainLineNumber error: %s", err.Error())
	}

	switch {
	case (npmExists && kubeExists):
		if lineNum != 3 {
			t.Errorf("TestGetChainLineNumber @ initial line number check iptMgr.GetChainLineNumber with npmExists: %t kubeExists: %t", npmExists, kubeExists)
		}
	case npmExists:
		if lineNum == 0 {
			t.Errorf("TestGetChainLineNumber @ initial line number check iptMgr.GetChainLineNumber with npmExists: %t kubeExists: %t", npmExists, kubeExists)
		}
	default:
		if lineNum != 0 {
			t.Errorf("TestGetChainLineNumber @ initial line number check iptMgr.GetChainLineNumber with npmExists: %t kubeExists: %t", npmExists, kubeExists)
		}
	}

	if err = iptMgr.InitNpmChains(); err != nil {
		t.Errorf("TestGetChainLineNumber @ iptMgr.InitNpmChains error: %s", err.Error())
	}

	entry = &IptEntry{
		Chain: util.IptablesForwardChain,
		Specs: []string{
			util.IptablesJumpFlag,
			util.IptablesAzureChain,
		},
	}

	if npmExists, err = iptMgr.Exists(entry); err != nil {
		t.Errorf("TestGetChainLineNumber failed @ azure-npm chain iptMgr.Exists error: %s", err.Error())
	}

	lineNum, err = iptMgr.GetChainLineNumber(util.IptablesAzureChain, util.IptablesForwardChain)
	if err != nil {
		t.Errorf("TestGetChainLineNumber @ after Init chains iptMgr.GetChainLineNumber error: %s", err.Error())
	}

	switch {
	case (npmExists && kubeExists):
		if lineNum < 2 {
			t.Errorf("TestGetChainLineNumber @ after Init chains line number check iptMgr.GetChainLineNumber with npmExists: %t kubeExists: %t", npmExists, kubeExists)
		}
	case npmExists:
		if lineNum == 0 {
			t.Errorf("TestGetChainLineNumber @ after Init chains line number check iptMgr.GetChainLineNumber with npmExists: %t kubeExists: %t", npmExists, kubeExists)
		}
	case !npmExists:
		t.Errorf("TestGetChainLineNumber @ after Init chains line number check iptMgr.GetChainLineNumber with failed to Add chain ")
	}

	if err = iptMgr.UninitNpmChains(); err != nil {
		t.Errorf("TestGetChainLineNumber @ iptMgr.UninitNpmChains")
	}
}

func TestMain(m *testing.M) {
	metrics.InitializeAll()
	ipt := &IptOperationShim{}
	iptMgr := NewIptablesManager(exec.New(), ipt)
	iptMgr.Save(util.IptablesConfigFile)

	exitCode := m.Run()

	iptMgr.Restore(util.IptablesConfigFile)

	os.Exit(exitCode)
}
