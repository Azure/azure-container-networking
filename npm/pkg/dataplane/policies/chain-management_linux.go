package policies

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ioutil"
	"github.com/Azure/azure-container-networking/npm/util"
	npmerrors "github.com/Azure/azure-container-networking/npm/util/errors"
	"k8s.io/klog"
	utilexec "k8s.io/utils/exec"
)

const (
	defaultlockWaitTimeInSeconds string = "60"
	reconcileChainTimeInMinutes  int    = 5

	doesNotExistErrorCode      int = 1 // Bad rule (does a matching rule exist in that chain?)
	couldntLoadTargetErrorCode int = 2 // Couldn't load target `AZURE-NPM-EGRESS':No such file or directory

	minLineNumberStringLength int = 3
	minChainStringLength      int = 7
)

var (
	iptablesAzureChains = []string{
		util.IptablesAzureChain,
		util.IptablesAzureIngressChain,
		util.IptablesAzureIngressAllowMarkChain,
		util.IptablesAzureEgressChain,
		util.IptablesAzureAcceptChain,
	}
	iptablesAzureDeprecatedChains = []string{
		// NPM v1
		util.IptablesAzureIngressFromChain,
		util.IptablesAzureIngressPortChain,
		util.IptablesAzureIngressDropsChain,
		util.IptablesAzureEgressToChain,
		util.IptablesAzureEgressPortChain,
		util.IptablesAzureEgressDropsChain,
		// older
		util.IptablesAzureTargetSetsChain,
		util.IptablesAzureIngressWrongDropsChain,
	}
	iptablesOldAndNewChains = append(iptablesAzureChains, iptablesAzureDeprecatedChains...)

	jumpToAzureChainArgs            = []string{util.IptablesJumpFlag, util.IptablesAzureChain, util.IptablesModuleFlag, util.IptablesCtstateModuleFlag, util.IptablesCtstateFlag, util.IptablesNewState}
	jumpFromForwardToAzureChainArgs = append([]string{util.IptablesForwardChain}, jumpToAzureChainArgs...)

	ingressOrEgressPolicyChainPattern = fmt.Sprintf("'Chain %s-\\|Chain %s-'", util.IptablesAzureIngressPolicyChainPrefix, util.IptablesAzureEgressPolicyChainPrefix)
)

func (pMgr *PolicyManager) initialize() error {
	if err := pMgr.initializeNPMChains(); err != nil {
		return npmerrors.SimpleErrorWrapper("failed to initialize NPM chains", err)
	}
	return nil
}

func (pMgr *PolicyManager) reset() error {
	if err := pMgr.removeNPMChains(); err != nil {
		return npmerrors.SimpleErrorWrapper("failed to remove NPM chains", err)
	}
	return nil
}

// initializeNPMChains creates all chains/rules and makes sure the jump from FORWARD chain to
// AZURE-NPM chain is after the jumps to KUBE-FORWARD & KUBE-SERVICES chains (if they exist).
func (pMgr *PolicyManager) initializeNPMChains() error {
	klog.Infof("Initializing AZURE-NPM chains.")
	creator := pMgr.getCreatorForInitChains()
	err := restore(creator)
	if err != nil {
		return npmerrors.SimpleErrorWrapper("failed to create chains and rules", err)
	}

	// add the jump rule from FORWARD chain to AZURE-NPM chain
	if err := pMgr.positionAzureChainJumpRule(); err != nil {
		baseErrString := "failed to add/reposition jump from FORWARD chain to AZURE-NPM chain"
		metrics.SendErrorLogAndMetric(util.IptmID, "Error: %s with error: %s", baseErrString, err.Error())
		return npmerrors.SimpleErrorWrapper(baseErrString, err) // we used to ignore this error in v1
	}
	return nil
}

// removeNPMChains removes the jump rule from FORWARD chain to AZURE-NPM chain
// and flushes and deletes all NPM Chains.
func (pMgr *PolicyManager) removeNPMChains() error {
	deleteErrCode, deleteErr := pMgr.runIPTablesCommand(util.IptablesDeletionFlag, jumpFromForwardToAzureChainArgs...)
	hadDeleteError := deleteErr != nil && deleteErrCode != couldntLoadTargetErrorCode
	if hadDeleteError {
		baseErrString := "failed to delete jump from FORWARD chain to AZURE-NPM chain"
		metrics.SendErrorLogAndMetric(util.IptmID, "Error: %s with exit code %d and error: %s", baseErrString, deleteErrCode, deleteErr.Error())
		// FIXME update ID
		return npmerrors.SimpleErrorWrapper(baseErrString, deleteErr)
	}

	// flush all chains (will create any chain, including deprecated ones, if they don't exist)
	creatorToFlush, chainsToDelete := pMgr.getCreatorAndChainsForReset()
	restoreError := restore(creatorToFlush)
	if restoreError != nil {
		return npmerrors.SimpleErrorWrapper("failed to flush chains", restoreError)
	}

	// TODO aggregate an error for each chain that failed to delete
	var anyDeleteErr error
	for _, chainName := range chainsToDelete {
		errCode, err := pMgr.runIPTablesCommand(util.IptablesDestroyFlag, chainName)
		if err != nil {
			klog.Infof("couldn't delete chain %s with error [%v] and exit code [%d]", chainName, err, errCode)
			anyDeleteErr = err
		}
	}

	if anyDeleteErr != nil {
		return npmerrors.SimpleErrorWrapper("couldn't delete all chains", anyDeleteErr)
	}
	return nil
}

// ReconcileChains periodically creates the jump rule from FORWARD chain to AZURE-NPM chain (if it d.n.e)
// and makes sure it's after the jumps to KUBE-FORWARD & KUBE-SERVICES chains (if they exist).
func (pMgr *PolicyManager) ReconcileChains(stopChannel <-chan struct{}) {
	go pMgr.reconcileChains(stopChannel)
}

func (pMgr *PolicyManager) reconcileChains(stopChannel <-chan struct{}) {
	ticker := time.NewTicker(time.Minute * time.Duration(reconcileChainTimeInMinutes))
	defer ticker.Stop()

	for {
		select {
		case <-stopChannel:
			return
		case <-ticker.C:
			if err := pMgr.positionAzureChainJumpRule(); err != nil {
				metrics.SendErrorLogAndMetric(util.NpmID, "Error: failed to reconcile jump rule to Azure-NPM due to %s", err.Error())
			}
		}
	}
}

// this function has a direct comparison in NPM v1 iptables manager (iptm.go)
func (pMgr *PolicyManager) runIPTablesCommand(operationFlag string, args ...string) (int, error) {
	allArgs := []string{util.IptablesWaitFlag, defaultlockWaitTimeInSeconds, operationFlag}
	allArgs = append(allArgs, args...)

	if operationFlag != util.IptablesCheckFlag {
		klog.Infof("Executing iptables command with args %v", allArgs)
	}

	command := pMgr.ioShim.Exec.Command(util.Iptables, allArgs...)
	output, err := command.CombinedOutput()

	var exitError utilexec.ExitError
	if ok := errors.As(err, &exitError); ok {
		errCode := exitError.ExitStatus()
		allArgsString := strings.Join(allArgs, " ")
		msgStr := strings.TrimSuffix(string(output), "\n")
		if errCode > 0 && operationFlag != util.IptablesCheckFlag {
			metrics.SendErrorLogAndMetric(util.IptmID, "Error: There was an error running command: [%s %s] Stderr: [%v, %s]", util.Iptables, allArgsString, exitError, msgStr)
		}
		return errCode, npmerrors.SimpleErrorWrapper(fmt.Sprintf("failed to run iptables command [%s %s] Stderr: [%s]", util.Iptables, allArgsString, msgStr), exitError)
	}
	return 0, nil
}

func (pMgr *PolicyManager) getCreatorForInitChains() *ioutil.FileCreator {
	creator := pMgr.getNewCreatorWithChains(iptablesAzureChains)

	// add AZURE-NPM chain rules
	creator.AddLine("", nil, util.IptablesAppendFlag, util.IptablesAzureChain, util.IptablesJumpFlag, util.IptablesAzureIngressChain)
	creator.AddLine("", nil, util.IptablesAppendFlag, util.IptablesAzureChain, util.IptablesJumpFlag, util.IptablesAzureEgressChain)
	creator.AddLine("", nil, util.IptablesAppendFlag, util.IptablesAzureChain, util.IptablesJumpFlag, util.IptablesAzureAcceptChain)

	// add AZURE-NPM-INGRESS chain rules
	ingressDropSpecs := []string{util.IptablesAppendFlag, util.IptablesAzureIngressChain, util.IptablesJumpFlag, util.IptablesDrop}
	ingressDropSpecs = append(ingressDropSpecs, getOnMarkSpecs(util.IptablesAzureIngressDropMarkHex)...)
	ingressDropSpecs = append(ingressDropSpecs, getCommentSpecs(fmt.Sprintf("DROP-ON-INGRESS-DROP-MARK-%s", util.IptablesAzureIngressDropMarkHex))...)
	creator.AddLine("", nil, ingressDropSpecs...)

	// add AZURE-NPM-INGRESS-ALLOW-MARK chain
	markIngressAllowSpecs := []string{util.IptablesAppendFlag, util.IptablesAzureIngressAllowMarkChain}
	markIngressAllowSpecs = append(markIngressAllowSpecs, getSetMarkSpecs(util.IptablesAzureIngressAllowMarkHex)...)
	markIngressAllowSpecs = append(markIngressAllowSpecs, getCommentSpecs(fmt.Sprintf("SET-INGRESS-ALLOW-MARK-%s", util.IptablesAzureIngressAllowMarkHex))...)
	creator.AddLine("", nil, markIngressAllowSpecs...)
	creator.AddLine("", nil, util.IptablesAppendFlag, util.IptablesAzureIngressAllowMarkChain, util.IptablesJumpFlag, util.IptablesAzureEgressChain)

	// add AZURE-NPM-EGRESS chain rules
	egressDropSpecs := []string{util.IptablesAppendFlag, util.IptablesAzureEgressChain, util.IptablesJumpFlag, util.IptablesDrop}
	egressDropSpecs = append(egressDropSpecs, getOnMarkSpecs(util.IptablesAzureEgressDropMarkHex)...)
	egressDropSpecs = append(egressDropSpecs, getCommentSpecs(fmt.Sprintf("DROP-ON-EGRESS-DROP-MARK-%s", util.IptablesAzureEgressDropMarkHex))...)
	creator.AddLine("", nil, egressDropSpecs...)

	jumpOnIngressMatchSpecs := []string{util.IptablesAppendFlag, util.IptablesAzureEgressChain, util.IptablesJumpFlag, util.IptablesAzureAcceptChain}
	jumpOnIngressMatchSpecs = append(jumpOnIngressMatchSpecs, getOnMarkSpecs(util.IptablesAzureIngressAllowMarkHex)...)
	jumpOnIngressMatchSpecs = append(jumpOnIngressMatchSpecs, getCommentSpecs(fmt.Sprintf("ACCEPT-ON-INGRESS-ALLOW-MARK-%s", util.IptablesAzureIngressAllowMarkHex))...)
	creator.AddLine("", nil, jumpOnIngressMatchSpecs...)

	// add AZURE-NPM-ACCEPT chain rules
	clearSpecs := []string{util.IptablesAppendFlag, util.IptablesAzureAcceptChain}
	clearSpecs = append(clearSpecs, getSetMarkSpecs(util.IptablesAzureClearMarkHex)...)
	clearSpecs = append(clearSpecs, getCommentSpecs("Clear-AZURE-NPM-MARKS")...)
	creator.AddLine("", nil, clearSpecs...)
	creator.AddLine("", nil, util.IptablesAppendFlag, util.IptablesAzureAcceptChain, util.IptablesJumpFlag, util.IptablesAccept)
	creator.AddLine("", nil, util.IptablesRestoreCommit)
	return creator
}

// add/reposition AZURE-NPM chain after KUBE-FORWARD and KUBE-SERVICE chains if they exist
// this function has a direct comparison in NPM v1 iptables manager (iptm.go)
func (pMgr *PolicyManager) positionAzureChainJumpRule() error {
	kubeServicesLine, kubeServicesLineNumErr := pMgr.getChainLineNumber(util.IptablesKubeServicesChain)
	if kubeServicesLineNumErr != nil {
		// not possible to cover this branch currently because of testing limitations for pipeCommandToGrep()
		baseErrString := "failed to get index of jump from KUBE-SERVICES chain to FORWARD chain with error"
		metrics.SendErrorLogAndMetric(util.IptmID, "Error: %s: %s", baseErrString, kubeServicesLineNumErr.Error())
		return npmerrors.SimpleErrorWrapper(baseErrString, kubeServicesLineNumErr)
	}

	index := kubeServicesLine + 1

	// TODO could call getChainLineNumber instead, and say it doesn't exist for lineNum == 0
	jumpRuleErrCode, checkErr := pMgr.runIPTablesCommand(util.IptablesCheckFlag, jumpFromForwardToAzureChainArgs...)
	hadCheckError := checkErr != nil && jumpRuleErrCode != doesNotExistErrorCode
	if hadCheckError {
		baseErrString := "failed to check if jump from FORWARD chain to AZURE-NPM chain exists"
		metrics.SendErrorLogAndMetric(util.IptmID, "Error: %s: %s", baseErrString, checkErr.Error())
		return npmerrors.SimpleErrorWrapper(baseErrString, checkErr)
	}
	jumpRuleExists := jumpRuleErrCode != doesNotExistErrorCode

	if !jumpRuleExists {
		klog.Infof("Inserting jump from FORWARD chain to AZURE-NPM chain")
		jumpRuleInsertionArgs := append([]string{util.IptablesForwardChain, strconv.Itoa(index)}, jumpToAzureChainArgs...)
		if insertErrCode, insertErr := pMgr.runIPTablesCommand(util.IptablesInsertionFlag, jumpRuleInsertionArgs...); insertErr != nil {
			baseErrString := "failed to insert jump from FORWARD chain to AZURE-NPM chain"
			metrics.SendErrorLogAndMetric(util.IptmID, "Error: %s with error code %d and error %s", baseErrString, insertErrCode, insertErr.Error())
			// FIXME update ID
			return npmerrors.SimpleErrorWrapper(baseErrString, insertErr)
		}
		return nil
	}

	if kubeServicesLine <= 1 {
		// jump to KUBE-SERVICES chain doesn't exist or is the first rule
		return nil
	}

	npmChainLine, npmLineNumErr := pMgr.getChainLineNumber(util.IptablesAzureChain)
	if npmLineNumErr != nil {
		// not possible to cover this branch currently because of testing limitations for pipeCommandToGrep()
		baseErrString := "failed to get index of jump from FORWARD chain to AZURE-NPM chain"
		metrics.SendErrorLogAndMetric(util.IptmID, "Error: %s: %s", baseErrString, npmLineNumErr.Error())
		// FIXME update ID
		return npmerrors.SimpleErrorWrapper(baseErrString, npmLineNumErr)
	}

	// Kube-services line number is less than npm chain line number then all good
	if kubeServicesLine < npmChainLine {
		return nil
	}

	// AZURE-NPM chain is before KUBE-SERVICES then
	// delete existing jump rule and add it in the right order
	metrics.SendErrorLogAndMetric(util.IptmID, "Info: Reconciler deleting and re-adding jump from FORWARD chain to AZURE-NPM chain table.")
	if deleteErrCode, deleteErr := pMgr.runIPTablesCommand(util.IptablesDeletionFlag, jumpFromForwardToAzureChainArgs...); deleteErr != nil {
		baseErrString := "failed to delete jump from FORWARD chain to AZURE-NPM chain"
		metrics.SendErrorLogAndMetric(util.IptmID, "Error: %s with error code %d and error %s", baseErrString, deleteErrCode, deleteErr.Error())
		// FIXME update ID
		return npmerrors.SimpleErrorWrapper(baseErrString, deleteErr)
	}

	// Reduce index for deleted AZURE-NPM chain
	if index > 1 {
		index--
	}
	jumpRuleInsertionArgs := append([]string{util.IptablesForwardChain, strconv.Itoa(index)}, jumpToAzureChainArgs...)
	if insertErrCode, insertErr := pMgr.runIPTablesCommand(util.IptablesInsertionFlag, jumpRuleInsertionArgs...); insertErr != nil {
		baseErrString := "after deleting, failed to insert jump from FORWARD chain to AZURE-NPM chain"
		// FIXME update ID
		metrics.SendErrorLogAndMetric(util.IptmID, "Error: %s with error code %d and error %s", baseErrString, insertErrCode, insertErr.Error())
		return npmerrors.SimpleErrorWrapper(baseErrString, insertErr)
	}

	return nil
}

// returns 0 if the chain d.n.e.
// this function has a direct comparison in NPM v1 iptables manager (iptm.go)
func (pMgr *PolicyManager) getChainLineNumber(chain string) (int, error) {
	// TODO could call this once and use regex instead of grep to cut down on OS calls
	listForwardEntriesCommand := pMgr.ioShim.Exec.Command(util.Iptables,
		util.IptablesWaitFlag, defaultlockWaitTimeInSeconds, util.IptablesTableFlag, util.IptablesFilterTable,
		util.IptablesNumericFlag, util.IptablesListFlag, util.IptablesForwardChain, util.IptablesLineNumbersFlag,
	)
	grepCommand := pMgr.ioShim.Exec.Command("grep", chain)
	searchResults, gotMatches, err := pipeCommandToGrep(listForwardEntriesCommand, grepCommand)
	if err != nil {
		// not possible to cover this branch currently because of testing limitations for pipeCommandToGrep()
		return 0, npmerrors.SimpleErrorWrapper(fmt.Sprintf("failed to determine line number for jump from FORWARD chain to %s chain", chain), err)
	}
	if !gotMatches {
		return 0, nil
	}
	if len(searchResults) >= minLineNumberStringLength {
		lineNum, _ := strconv.Atoi(string(searchResults[0]))
		return lineNum, nil
	}
	return 0, nil
}

func pipeCommandToGrep(command, grepCommand utilexec.Cmd) (searchResults []byte, gotMatches bool, commandError error) {
	pipe, commandError := command.StdoutPipe()
	if commandError != nil {
		return
	}

	closePipe := func() { _ = pipe.Close() } // appease go lint
	defer closePipe()

	commandError = command.Start()
	if commandError != nil {
		return
	}

	// Without this wait, defunct iptable child process are created
	wait := func() { _ = command.Wait() } // appease go lint
	defer wait()

	output, err := grepCommand.CombinedOutput()
	if err != nil {
		// grep returns err status 1 if nothing is found
		return
	}
	searchResults = output
	gotMatches = true
	return
}

// make this a function for easier testing
func (pMgr *PolicyManager) getCreatorAndChainsForReset() (creator *ioutil.FileCreator, chainsToFlush []string) {
	oldPolicyChains, err := pMgr.getPolicyChainNames()
	if err != nil {
		// not possible to cover this branch currently because of testing limitations for pipeCommandToGrep()
		metrics.SendErrorLogAndMetric(util.IptmID, "Error: failed to determine NPM ingress/egress policy chains to delete")
	}
	chainsToFlush = iptablesOldAndNewChains
	chainsToFlush = append(chainsToFlush, oldPolicyChains...) // will work even if oldPolicyChains is nil
	creator = pMgr.getNewCreatorWithChains(chainsToFlush)
	creator.AddLine("", nil, util.IptablesRestoreCommit)
	return
}

func (pMgr *PolicyManager) getPolicyChainNames() ([]string, error) {
	iptablesListCommand := pMgr.ioShim.Exec.Command(util.Iptables,
		util.IptablesWaitFlag, defaultlockWaitTimeInSeconds, util.IptablesTableFlag, util.IptablesFilterTable,
		util.IptablesNumericFlag, util.IptablesListFlag,
	)
	grepCommand := pMgr.ioShim.Exec.Command("grep", ingressOrEgressPolicyChainPattern)
	searchResults, gotMatches, err := pipeCommandToGrep(iptablesListCommand, grepCommand)
	if err != nil {
		// not possible to cover this branch currently because of testing limitations for pipeCommandToGrep()
		return nil, npmerrors.SimpleErrorWrapper("failed to get policy chain names", err)
	}
	if !gotMatches {
		return nil, nil
	}
	lines := strings.Split(string(searchResults), "\n")
	chainNames := make([]string, 0, len(lines)) // don't want to preallocate size in case of have malformed lines
	for _, line := range lines {
		if len(line) < minChainStringLength {
			klog.Errorf("got unexpected grep output for ingress/egress policy chains")
		} else {
			chainNames = append(chainNames, line[minChainStringLength-1:])
		}
	}
	return chainNames, nil
}

func getOnMarkSpecs(mark string) []string {
	return []string{
		util.IptablesModuleFlag,
		util.IptablesMarkVerb,
		util.IptablesMarkFlag,
		mark,
	}
}
