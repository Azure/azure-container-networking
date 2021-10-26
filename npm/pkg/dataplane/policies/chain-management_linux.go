package policies

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ioutil"
	"github.com/Azure/azure-container-networking/npm/util"
	utilexec "k8s.io/utils/exec"
)

const (
	defaultlockWaitTimeInSeconds string = "60"
	iptablesErrDoesNotExist      int    = 1
	reconcileChainTimeInMinutes         = 5
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

func (pMgr *PolicyManager) reset() error {
	if err := pMgr.removeNPMChains(); err != nil {
		return fmt.Errorf("failed to remove NPM chains: %w", err)
	}
	if err := pMgr.initializeNPMChains(); err != nil {
		return fmt.Errorf("failed to initialize NPM chains: %w", err)
	}
	return nil
}

// initializeNPMChains creates all chains/rules and makes sure the jump from FORWARD chain to
// AZURE-NPM chain is after the jumps to KUBE-FORWARD & KUBE-SERVICES chains (if they exist).
func (pMgr *PolicyManager) initializeNPMChains() error {
	log.Logf("Initializing AZURE-NPM chains.")
	creator := pMgr.getCreatorForInitChains()
	restoreError := restore(creator)
	if restoreError != nil {
		return restoreError
	}

	// add the jump rule from FORWARD chain to AZURE-NPM chain
	if err := pMgr.positionAzureChainJumpRule(); err != nil {
		metrics.SendErrorLogAndMetric(util.IptmID, "Error: failed to add/reposition jump from FORWARD chain to AZURE-NPM chain: %s", err.Error())
		return err // we used to ignore this error in v1
	}
	return nil
}

// removeNPMChains removes the jump rule from FORWARD chain to AZURE-NPM chain
// and flushes and deletes all NPM Chains.
func (pMgr *PolicyManager) removeNPMChains() error {
	errCode, err := pMgr.runIPTablesCommand(util.IptablesDeletionFlag, jumpFromForwardToAzureChainArgs...)
	if errCode != iptablesErrDoesNotExist && err != nil {
		metrics.SendErrorLogAndMetric(util.IptmID, "Error: failed to delete AZURE-NPM from FORWARD chain")
		// FIXME update ID
		return err
	}

	// flush all chains (will create any chain, including deprecated ones, if they don't exist)
	creator, chainsToFlush := pMgr.getCreatorAndChainsForReset()
	restoreError := restore(creator)
	if restoreError != nil {
		return restoreError
	}

	for _, chainName := range chainsToFlush {
		errCode, err = pMgr.runIPTablesCommand(util.IptablesDestroyFlag, chainName)
		if err != nil {
			log.Logf("couldn't delete chain %s with error [%w] and exit code [%d]", chainName, err, errCode)
		}
	}

	if err != nil {
		return fmt.Errorf("couldn't delete all chains")
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
		log.Logf("Executing iptables command with args %v", allArgs)
	}

	command := pMgr.ioShim.Exec.Command(util.Iptables, allArgs...)
	output, err := command.CombinedOutput()
	if msg, failed := err.(utilexec.ExitError); failed {
		errCode := msg.ExitStatus()
		if errCode > 0 && operationFlag != util.IptablesCheckFlag {
			msgStr := strings.TrimSuffix(string(output), "\n")
			if strings.Contains(msgStr, "Chain already exists") && operationFlag == util.IptablesChainCreationFlag {
				return 0, nil
			}
			metrics.SendErrorLogAndMetric(util.IptmID, "Error: There was an error running command: [%s %v] Stderr: [%v, %s]", util.Iptables, strings.Join(allArgs, " "), err, msgStr)
		}
		return errCode, err
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
	kubeServicesLine, err := pMgr.getChainLineNumber(util.IptablesKubeServicesChain)
	if err != nil {
		metrics.SendErrorLogAndMetric(util.IptmID, "failed to get index of jump from KUBE-SERVICES chain to FORWARD chain with error: %s", err.Error())
		return err
	}

	index := kubeServicesLine + 1

	// TODO could call getChainLineNumber instead, and say it doesn't exist for lineNum == 0
	jumpRuleErrCode, err := pMgr.runIPTablesCommand(util.IptablesCheckFlag, jumpFromForwardToAzureChainArgs...)
	if jumpRuleErrCode != iptablesErrDoesNotExist && err != nil {
		return fmt.Errorf("couldn't check if jump from FORWARD chain to AZURE-NPM chain exists: %w", err)
	}
	jumpRuleExists := jumpRuleErrCode != iptablesErrDoesNotExist

	if !jumpRuleExists {
		log.Logf("Inserting jump from FORWARD chain to AZURE-NPM chain")
		jumpRuleInsertionArgs := append([]string{util.IptablesForwardChain, strconv.Itoa(index)}, jumpToAzureChainArgs...)
		if errCode, err := pMgr.runIPTablesCommand(util.IptablesInsertionFlag, jumpRuleInsertionArgs...); err != nil {
			metrics.SendErrorLogAndMetric(util.IptmID, "Error: failed to insert jump from FORWARD chain to AZURE-NPM chain with error code %d.", errCode)
			// FIXME update ID
			return err
		}
		return nil
	}

	if kubeServicesLine <= 1 {
		// jumpt to KUBE-SERVICES chain doesn't exist or is the first rule
		return nil
	}

	npmChainLine, err := pMgr.getChainLineNumber(util.IptablesAzureChain)
	if err != nil {
		metrics.SendErrorLogAndMetric(util.IptmID, "Error: failed to get index of jump from FORWARD chain to AZURE-NPM chain with error: %s", err.Error())
		return err
	}

	// Kube-services line number is less than npm chain line number then all good
	if kubeServicesLine < npmChainLine {
		return nil
	}

	// AZURE-NPM chain is before KUBE-SERVICES then
	// delete existing jump rule and add it in the right order
	metrics.SendErrorLogAndMetric(util.IptmID, "Info: Reconciler deleting and re-adding jump from FORWARD chain to AZURE-NPM chain table.")
	if errCode, err := pMgr.runIPTablesCommand(util.IptablesDeletionFlag, jumpFromForwardToAzureChainArgs...); err != nil {
		metrics.SendErrorLogAndMetric(util.IptmID, "Error: failed to delete jump from FORWARD chain to AZURE-NPM chain with error code %d.", errCode)
		return err
	}

	// Reduce index for deleted AZURE-NPM chain
	if index > 1 {
		index--
	}
	jumpRuleInsertionArgs := append([]string{util.IptablesForwardChain, strconv.Itoa(index)}, jumpToAzureChainArgs...)
	if errCode, err := pMgr.runIPTablesCommand(util.IptablesInsertionFlag, jumpRuleInsertionArgs...); err != nil {
		metrics.SendErrorLogAndMetric(util.IptmID, "Error: after deleting, failed to insert jump from FORWARD chain to AZURE-NPM chain with error code %d.", errCode)
		return err
	}

	return nil
}

// returns 0 if the chain d.n.e.
// this function has a direct comparison in NPM v1 iptables manager (iptm.go)
func (pMgr *PolicyManager) getChainLineNumber(chain string) (int, error) {
	// TODO could call this once and use regex instead of grep to cut down on OS calls
	listForwardEntriesCommand := pMgr.ioShim.Exec.Command(util.Iptables, util.IptablesWaitFlag, defaultlockWaitTimeInSeconds, util.IptablesTableFlag, util.IptablesFilterTable, util.IptablesNumericFlag, util.IptablesListFlag, util.IptablesForwardChain, util.IptablesLineNumbersFlag)
	grepCommand := pMgr.ioShim.Exec.Command("grep", chain)
	pipe, err := listForwardEntriesCommand.StdoutPipe()
	if err != nil {
		return 0, err
	}
	defer pipe.Close()
	grepCommand.SetStdin(pipe)

	if err := listForwardEntriesCommand.Start(); err != nil {
		return 0, err
	}
	// Without this wait, defunct iptable child process are created
	defer listForwardEntriesCommand.Wait()

	output, err := grepCommand.CombinedOutput()
	if err != nil {
		// grep returns err status 1 if not founds
		return 0, nil
	}

	if len(output) > 2 {
		lineNum, _ := strconv.Atoi(string(output[0]))
		return lineNum, nil
	}
	return 0, nil
}

// make this a function for easier testing
func (pMgr *PolicyManager) getCreatorAndChainsForReset() (*ioutil.FileCreator, []string) {
	oldPolicyChains, err := pMgr.getPolicyChainNames()
	if err != nil {
		metrics.SendErrorLogAndMetric(util.IptmID, "Error: failed to determine NPM ingress/egress policy chains to delete")
	}
	chainsToFlush := iptablesOldAndNewChains
	chainsToFlush = append(chainsToFlush, oldPolicyChains...) // will work even if oldPolicyChains is nil
	creator := pMgr.getNewCreatorWithChains(chainsToFlush)
	creator.AddLine("", nil, util.IptablesRestoreCommit)
	return creator, chainsToFlush
}

func (pMgr *PolicyManager) getPolicyChainNames() ([]string, error) {
	iptablesListCommand := pMgr.ioShim.Exec.Command(util.Iptables, util.IptablesWaitFlag, defaultlockWaitTimeInSeconds, util.IptablesTableFlag, util.IptablesFilterTable, util.IptablesNumericFlag, util.IptablesListFlag)
	grepCommand := pMgr.ioShim.Exec.Command("grep", ingressOrEgressPolicyChainPattern)
	pipe, err := iptablesListCommand.StdoutPipe()
	if err != nil {
		return nil, err
	}
	defer pipe.Close()
	grepCommand.SetStdin(pipe)

	if err := iptablesListCommand.Start(); err != nil {
		return nil, err
	}
	// Without this wait, defunct iptable child process are created
	defer iptablesListCommand.Wait()

	output, err := grepCommand.CombinedOutput()
	if err != nil {
		// grep returns err status 1 if not found
		return nil, nil
	}

	lines := strings.Split(string(output), "\n")
	chainNames := make([]string, 0, len(lines)) // don't want to preallocate size in case of have malformed lines
	for _, line := range lines {
		if len(line) < 7 {
			log.Errorf("got unexpected grep output for ingress/egress chains")
		} else {
			chainNames = append(chainNames, line[6:])
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
