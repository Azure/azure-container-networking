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

		// NOTE the following grep call stdouts are misleading. The first grep returns 3, and the second one returns "" (i.e. line 0)
		// a fix is coming for fakeexec stdout and exit code problems from piping commands (e.g. what we do with grep)
		{Cmd: []string{"iptables", "-t", "filter", "-n", "--list", "FORWARD", "--line-numbers"}, Stdout: "3  "}, // THIS IS THE GREP CALL
		{Cmd: []string{"grep", "KUBE-SERVICES"}, Stdout: "4  "},

		{Cmd: []string{"iptables", "-w", "60", "-C", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
		{Cmd: []string{"iptables", "-t", "filter", "-n", "--list", "FORWARD", "--line-numbers"}, Stdout: "3  "}, // THIS IS THE GREP CALL
		{Cmd: []string{"grep", "AZURE-NPM"}, Stdout: "4  "},
		{Cmd: []string{"iptables", "-w", "60", "-D", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
		{Cmd: []string{"iptables", "-w", "60", "-I", "FORWARD", "3", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},

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
		// /////////
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
		{Cmd: []string{"iptables", "-w", "60", "-D", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
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

	initWithJumpToAzureAtTopCalls = []testutils.TestCmd{
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

		{Cmd: []string{"iptables", "-w", "60", "-C", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}, ExitCode: 1},
		{Cmd: []string{"iptables", "-w", "60", "-I", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},

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
		// /////////
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
)

func TestInitNpmChains(t *testing.T) {
	calls := initCalls

	fexec := testutils.GetFakeExecWithScripts(calls)
	defer testutils.VerifyCalls(t, fexec, calls)
	iptMgr := NewIptablesManager(fexec, NewFakeIptOperationShim(), util.PlaceAzureChainAfterKubeServices)

	err := iptMgr.InitNpmChains()
	require.NoError(t, err)
}

func TestUninitNpmChains(t *testing.T) {
	calls := unInitCalls

	fexec := testutils.GetFakeExecWithScripts(calls)
	defer testutils.VerifyCalls(t, fexec, calls)
	iptMgr := NewIptablesManager(fexec, NewFakeIptOperationShim(), util.PlaceAzureChainAfterKubeServices)

	if err := iptMgr.UninitNpmChains(); err != nil {
		t.Errorf("TestUninitNpmChains @ iptMgr.UninitNpmChains")
	}
}

func TestInitWithJumpToAzureAtTop(t *testing.T) {
	calls := initWithJumpToAzureAtTopCalls

	fexec := testutils.GetFakeExecWithScripts(calls)
	defer testutils.VerifyCalls(t, fexec, calls)
	iptMgr := NewIptablesManager(fexec, NewFakeIptOperationShim(), util.PlaceAzureChainFirst)

	err := iptMgr.InitNpmChains()
	require.NoError(t, err)
}

func TestCheckAndAddForwardChain(t *testing.T) {
	type args struct {
		calls                []testutils.TestCmd
		placeAzureChainFirst bool
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "add missing jump to azure at top",
			args: args{
				calls: []testutils.TestCmd{
					{Cmd: []string{"iptables", "-w", "60", "-N", "AZURE-NPM"}},
					{Cmd: []string{"iptables", "-w", "60", "-C", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}, ExitCode: 1}, // "rule does not exist"
					{Cmd: []string{"iptables", "-w", "60", "-I", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
				},
				placeAzureChainFirst: util.PlaceAzureChainFirst,
			},
		},
		{
			name: "add missing jump to azure after kube services",
			args: args{
				calls: []testutils.TestCmd{
					{Cmd: []string{"iptables", "-w", "60", "-N", "AZURE-NPM"}},
					{Cmd: []string{"iptables", "-t", "filter", "-n", "--list", "FORWARD", "--line-numbers"}, Stdout: "3  "}, // THIS IS THE GREP CALL STDOUT
					{Cmd: []string{"grep", "KUBE-SERVICES"}, ExitCode: 1},                                                   // THIS IS THE EXIT CODE FOR CHECK command below ("rule doesn't exist")
					{Cmd: []string{"iptables", "-w", "60", "-C", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
					{Cmd: []string{"iptables", "-w", "60", "-I", "FORWARD", "4", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
				},
				placeAzureChainFirst: util.PlaceAzureChainAfterKubeServices,
			},
		},
		{
			name: "jump to azure already at top",
			args: args{
				calls: []testutils.TestCmd{
					{Cmd: []string{"iptables", "-w", "60", "-N", "AZURE-NPM"}},
					{Cmd: []string{"iptables", "-w", "60", "-C", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
					{Cmd: []string{"iptables", "-t", "filter", "-n", "--list", "FORWARD", "--line-numbers"}, Stdout: "1  "}, // THIS IS THE GREP CALL STDOUT
					{Cmd: []string{"grep", "AZURE-NPM"}},
				},
				placeAzureChainFirst: util.PlaceAzureChainFirst,
			},
		},
		{
			name: "jump to azure already after kube services",
			args: args{
				calls: []testutils.TestCmd{
					{Cmd: []string{"iptables", "-w", "60", "-N", "AZURE-NPM"}},
					{Cmd: []string{"iptables", "-t", "filter", "-n", "--list", "FORWARD", "--line-numbers"}, Stdout: "3  "}, // THIS IS THE GREP CALL STDOUT
					{Cmd: []string{"grep", "KUBE-SERVICES"}},
					{Cmd: []string{"iptables", "-w", "60", "-C", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}, Stdout: "4  "}, // THIS IS THE GREP CALL STDOUT
					{Cmd: []string{"iptables", "-t", "filter", "-n", "--list", "FORWARD", "--line-numbers"}},
					{Cmd: []string{"grep", "AZURE-NPM"}},
				},
				placeAzureChainFirst: util.PlaceAzureChainAfterKubeServices,
			},
		},
		{
			name: "move jump to azure to top",
			args: args{
				calls: []testutils.TestCmd{
					{Cmd: []string{"iptables", "-w", "60", "-N", "AZURE-NPM"}},
					{Cmd: []string{"iptables", "-w", "60", "-C", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
					{Cmd: []string{"iptables", "-t", "filter", "-n", "--list", "FORWARD", "--line-numbers"}, Stdout: "5  "}, // THIS IS THE GREP CALL STDOUT
					{Cmd: []string{"grep", "AZURE-NPM"}},
					{Cmd: []string{"iptables", "-w", "60", "-D", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
					{Cmd: []string{"iptables", "-w", "60", "-I", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
				},
				placeAzureChainFirst: util.PlaceAzureChainFirst,
			},
		},
		{
			name: "move jump to azure after kube services",
			args: args{
				calls: []testutils.TestCmd{
					{Cmd: []string{"iptables", "-w", "60", "-N", "AZURE-NPM"}},
					{Cmd: []string{"iptables", "-t", "filter", "-n", "--list", "FORWARD", "--line-numbers"}, Stdout: "3  "}, // THIS IS THE GREP CALL STDOUT
					{Cmd: []string{"grep", "KUBE-SERVICES"}},
					{Cmd: []string{"iptables", "-w", "60", "-C", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}, Stdout: "2  "}, // THIS IS THE GREP CALL STDOUT
					{Cmd: []string{"iptables", "-t", "filter", "-n", "--list", "FORWARD", "--line-numbers"}},
					{Cmd: []string{"grep", "AZURE-NPM"}},
					{Cmd: []string{"iptables", "-w", "60", "-D", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
					{Cmd: []string{"iptables", "-w", "60", "-I", "FORWARD", "3", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
				},
				placeAzureChainFirst: util.PlaceAzureChainAfterKubeServices,
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			fexec := testutils.GetFakeExecWithScripts(tt.args.calls)
			defer testutils.VerifyCalls(t, fexec, tt.args.calls)
			iptMgr := NewIptablesManager(fexec, NewFakeIptOperationShim(), tt.args.placeAzureChainFirst)
			err := iptMgr.checkAndAddForwardChain()
			require.NoError(t, err)
		})
	}
}

func TestExists(t *testing.T) {
	calls := []testutils.TestCmd{
		{Cmd: []string{"iptables", "-w", "60", "-C", "FORWARD", "-j", "ACCEPT"}},
	}

	fexec := testutils.GetFakeExecWithScripts(calls)
	defer testutils.VerifyCalls(t, fexec, calls)
	iptMgr := NewIptablesManager(fexec, NewFakeIptOperationShim(), util.PlaceAzureChainAfterKubeServices)

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
	iptMgr := NewIptablesManager(fexec, NewFakeIptOperationShim(), util.PlaceAzureChainAfterKubeServices)

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
	iptMgr := NewIptablesManager(fexec, NewFakeIptOperationShim(), util.PlaceAzureChainAfterKubeServices)

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
	iptMgr := NewIptablesManager(fexec, NewFakeIptOperationShim(), util.PlaceAzureChainAfterKubeServices)

	execCount := resetPrometheusAndGetExecCount(t)
	defer testPrometheusMetrics(t, 1, execCount+1)

	entry := &IptEntry{
		Chain: util.IptablesForwardChain,
		Specs: []string{
			util.IptablesJumpFlag,
			util.IptablesReject,
		},
	}

	if err := iptMgr.Add(entry); err != nil {
		t.Errorf("TestAdd failed @ iptMgr.Add")
	}
}

func resetPrometheusAndGetExecCount(t *testing.T) int {
	metrics.ResetNumACLRules()
	execCount, err := metrics.GetACLRuleExecCount()
	promutil.NotifyIfErrors(t, err)
	return execCount
}

func testPrometheusMetrics(t *testing.T, expectedNumACLRules, expectedExecCount int) {
	numACLRules, err := metrics.GetNumACLRules()
	promutil.NotifyIfErrors(t, err)
	if numACLRules != expectedNumACLRules {
		require.FailNowf(t, "", "Number of ACL Rules didn't register correctly in Prometheus. Expected %d. Got %d.", expectedNumACLRules, numACLRules)
	}

	execCount, err := metrics.GetACLRuleExecCount()
	promutil.NotifyIfErrors(t, err)
	if execCount != expectedExecCount {
		require.FailNowf(t, "", "Count for execution time didn't register correctly in Prometheus. Expected %d. Got %d.", expectedExecCount, execCount)
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
	iptMgr := NewIptablesManager(fexec, NewFakeIptOperationShim(), util.PlaceAzureChainAfterKubeServices)

	execCount := resetPrometheusAndGetExecCount(t)
	defer testPrometheusMetrics(t, 0, execCount+1)

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

	if err := iptMgr.Delete(entry); err != nil {
		t.Errorf("TestDelete failed @ iptMgr.Delete")
	}
}

func TestRun(t *testing.T) {
	calls := []testutils.TestCmd{
		{Cmd: []string{"iptables", "-w", "60", "-N", "TEST-CHAIN"}},
	}

	fexec := testutils.GetFakeExecWithScripts(calls)
	defer testutils.VerifyCalls(t, fexec, calls)
	iptMgr := NewIptablesManager(fexec, NewFakeIptOperationShim(), util.PlaceAzureChainAfterKubeServices)

	iptMgr.OperationFlag = util.IptablesChainCreationFlag
	entry := &IptEntry{
		Chain: "TEST-CHAIN",
	}
	if _, err := iptMgr.run(entry); err != nil {
		t.Errorf("TestRun failed @ iptMgr.run")
	}
}

func TestGetChainLineNumber(t *testing.T) {
	tests := []struct {
		name     string
		exitCode int
		stdout   string
		want     int
	}{
		{
			name:     "no match",
			exitCode: 1,
			want:     0,
		},
		{
			name:     "match",
			exitCode: 0,
			stdout:   "24    AZURE-NPM  all  --",
			want:     24,
		},
		{
			name:     "unexpected output (no line number)",
			exitCode: 0,
			stdout:   "no line number",
			want:     0,
		},
		{
			name:     "unexpected output (no space)",
			exitCode: 0,
			stdout:   "123456",
			want:     0,
		},
		{
			name:     "unexpected output (too short)",
			exitCode: 0,
			stdout:   "12",
			want:     0,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			calls := []testutils.TestCmd{
				{Cmd: []string{"iptables", "-t", "filter", "-n", "--list", "FORWARD", "--line-numbers"}, PipedToCommand: true},
				{Cmd: []string{"grep", "AZURE-NPM"}, Stdout: tt.stdout, ExitCode: tt.exitCode},
			}
			fexec := testutils.GetFakeExecWithScripts(calls)
			iptMgr := NewIptablesManager(fexec, NewFakeIptOperationShim(), util.PlaceAzureChainAfterKubeServices)

			lineNum, err := iptMgr.getChainLineNumber(util.IptablesAzureChain, util.IptablesForwardChain)
			require.NoError(t, err)
			require.Equal(t, tt.want, lineNum)

			testutils.VerifyCalls(t, fexec, calls)
		})
	}
}

func TestMain(m *testing.M) {
	metrics.InitializeAll()

	exitCode := m.Run()

	os.Exit(exitCode)
}
