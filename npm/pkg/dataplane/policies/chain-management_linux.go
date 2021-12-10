package policies

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ioutil"
	"github.com/Azure/azure-container-networking/npm/util"
	npmerrors "github.com/Azure/azure-container-networking/npm/util/errors"
	"k8s.io/klog"
	utilexec "k8s.io/utils/exec"
)

const (
	// TODO replace all util constants with local constants
	defaultlockWaitTimeInSeconds string = "60"

	doesNotExistErrorCode      int = 1 // Bad rule (does a matching rule exist in that chain?)
	couldntLoadTargetErrorCode int = 2 // Couldn't load target `AZURE-NPM-EGRESS':No such file or directory

	minLineNumberStringLength int = 3 // TODO transferred from iptm.go and not sure why this length is important, but will update the function its used in later anyways

	azureChainGrepPattern   string = "Chain AZURE-NPM"
	minAzureChainNameLength int    = len("AZURE-NPM")
	// the minimum number of sections when "Chain NAME (1 references)" is split on spaces (" ")
	minSpacedSectionsForChainLine int = 2
)

var (
	iptablesAzureChains = []string{
		util.IptablesAzureChain,
		util.IptablesAzureIngressChain,
		util.IptablesAzureIngressAllowMarkChain,
		util.IptablesAzureEgressChain,
		util.IptablesAzureAcceptChain,
	}
	jumpFromForwardToAzureChainArgs = []string{
		util.IptablesForwardChain,
		util.IptablesJumpFlag,
		util.IptablesAzureChain,
		util.IptablesModuleFlag,
		util.IptablesCtstateModuleFlag,
		util.IptablesCtstateFlag,
		util.IptablesNewState,
	}
	deprecatedJumpFromForwardToAzureChainArgs = []string{
		util.IptablesForwardChain,
		util.IptablesJumpFlag,
		util.IptablesAzureChain,
	}
)

type staleChains struct {
	chainsToCleanup map[string]struct{}
}

func newStaleChains() *staleChains {
	return &staleChains{make(map[string]struct{})}
}

func (s *staleChains) add(chain string) {
	s.chainsToCleanup[chain] = struct{}{}
}

func (s *staleChains) remove(chain string) {
	delete(s.chainsToCleanup, chain)
}

func (s *staleChains) emptyAndGetAll() []string {
	result := make([]string, len(s.chainsToCleanup))
	k := 0
	for chain := range s.chainsToCleanup {
		result[k] = chain
		s.remove(chain)
		k++
	}
	return result
}

func (s *staleChains) empty() {
	s.chainsToCleanup = make(map[string]struct{})
}

// A proactive approach to avoid time to install default chains when the first networkpolicy comes again.
// Different from v1, which uninits when there are no policies and initializes when there are policies.
// The dataplane also initializes when it's created, so this keeps the policymanager in-line with that philosophy of having chains initialized at all times.
func (pMgr *PolicyManager) reboot() error {
	// TODO for the sake of UTs, need to have a pMgr config specifying whether or not this reboot happens
	// if err := pMgr.reset(); err != nil {
	// 	return npmerrors.SimpleErrorWrapper("failed to remove NPM chains while rebooting", err)
	// }
	// if err := pMgr.initialize(); err != nil {
	// 	return npmerrors.SimpleErrorWrapper("failed to initialize NPM chains while rebooting", err)
	// }
	return nil
}

func (pMgr *PolicyManager) initialize() error {
	if err := pMgr.initializeNPMChains(); err != nil {
		return npmerrors.SimpleErrorWrapper("failed to initialize NPM chains", err)
	}
	return nil
}

func (pMgr *PolicyManager) reset(_ []string) error {
	if err := pMgr.removeNPMChains(); err != nil {
		return npmerrors.SimpleErrorWrapper("failed to remove NPM chains", err)
	}
	pMgr.staleChains.empty()
	return nil
}

// initializeNPMChains creates all chains/rules and makes sure the jump from FORWARD chain to
// AZURE-NPM chain is after the jumps to KUBE-FORWARD & KUBE-SERVICES chains (if they exist).
func (pMgr *PolicyManager) initializeNPMChains() error {
	klog.Infof("Initializing AZURE-NPM chains.")
	creator := pMgr.creatorForInitChains()
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
	// 1.1 delete the deprecated jump rule from FORWARD chain to AZURE-NPM chain, if it exists
	deprecatedErrCode, deprecatedErr := pMgr.runIPTablesCommand(util.IptablesDeletionFlag, deprecatedJumpFromForwardToAzureChainArgs...)
	if deprecatedErr == nil {
		klog.Infof("deleted deprecated jump rule from FORWARD chain to AZURE-NPM chain")
	} else {
		switch deprecatedErrCode {
		case couldntLoadTargetErrorCode:
			// couldntLoadTargetErrorCode happens when AZURE-NPM chain doesn't exist (and hence the jump rule doesn't exist too)
			klog.Infof("didn't delete deprecated jump rule from FORWARD chain to AZURE-NPM chain likely because AZURE-NPM chain doesn't exist. Exit code %d and error: %s", deprecatedErrCode, deprecatedErr)
		case doesNotExistErrorCode:
			// doesNotExistErrorCode happens when AZURE-NPM chain exists, but this jump rule doesn't exist
			klog.Infof("didn't delete deprecated jump rule from FORWARD chain to AZURE-NPM chain likely because NPM v1 was not used prior. Exit code %d and error: %s", deprecatedErrCode, deprecatedErr)
		default:
			klog.Errorf("failed to delete deprecated jump rule from FORWARD chain to AZURE-NPM chain for unexpected reason with exit code %d and error: %s", deprecatedErrCode, deprecatedErr.Error())
		}
	}

	// 1.2 delete the jump rule that has ctstate NEW
	deleteErrCode, deleteErr := pMgr.runIPTablesCommand(util.IptablesDeletionFlag, jumpFromForwardToAzureChainArgs...)
	if deleteErr != nil {
		switch deleteErrCode {
		case couldntLoadTargetErrorCode:
			klog.Infof("didn't delete jump from FORWARD chain to AZURE-NPM chain likely because AZURE-NPM chain doesn't exist. Exit code %d and error: %s", deleteErrCode, deleteErr.Error())
		case doesNotExistErrorCode:
			klog.Infof("didn't delete jump from FORWARD chain to AZURE-NPM chain likely because we're transitioning from v1.4.9 or older. Exit code %d and error: %s", deleteErrCode, deleteErr.Error())
		default:
			klog.Errorf("failed to delete jump from FORWARD chain to AZURE-NPM chain for unexpected reason with exit code %d and error: %s", deleteErrCode, deleteErr.Error())
		}
	}

	// 2. flush current NPM chains
	creatorToFlush, chainsToDelete := pMgr.creatorAndChainsForReset()
	if len(chainsToDelete) == 0 {
		return nil
	}
	restoreError := restore(creatorToFlush)
	if restoreError != nil {
		return npmerrors.SimpleErrorWrapper("failed to flush chains", restoreError)
	}

	// 3. delete current NPM chains
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

// reconcile does the following:
// - cleans up stale policy chains
// - creates the jump rule from FORWARD chain to AZURE-NPM chain (if it does not exist) and makes sure it's after the jumps to KUBE-FORWARD & KUBE-SERVICES chains (if they exist).
func (pMgr *PolicyManager) reconcile() {
	klog.Infof("repositioning azure chain jump rule")
	if err := pMgr.positionAzureChainJumpRule(); err != nil {
		klog.Errorf("failed to reconcile jump rule to Azure-NPM due to %s", err.Error())
	}
	staleChains := pMgr.staleChains.emptyAndGetAll()
	klog.Infof("cleaning up these stale chains: %+v", staleChains)
	if err := pMgr.cleanupChains(staleChains); err != nil {
		klog.Errorf("failed to clean up old policy chains with the following error %s", err.Error())
	}
}

// have to use slice argument for deterministic behavior for ioshim in UTs
func (pMgr *PolicyManager) cleanupChains(chains []string) error {
	var aggregateError error
	for _, chain := range chains {
		errCode, err := pMgr.runIPTablesCommand(util.IptablesDestroyFlag, chain)
		if err != nil && errCode != doesNotExistErrorCode {
			pMgr.staleChains.add(chain)
			currentErrString := fmt.Sprintf("failed to clean up policy chain %s with err [%v]", chain, err)
			if aggregateError == nil {
				aggregateError = npmerrors.SimpleError(currentErrString)
			} else {
				aggregateError = npmerrors.SimpleErrorWrapper(fmt.Sprintf("%s and had previous error", currentErrString), aggregateError)
			}
		}
	}
	if aggregateError != nil {
		return npmerrors.SimpleErrorWrapper("failed to clean up some policy chains with errors", aggregateError)
	}
	return nil
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

func (pMgr *PolicyManager) creatorForInitChains() *ioutil.FileCreator {
	creator := pMgr.newCreatorWithChains(iptablesAzureChains)

	// add AZURE-NPM chain rules
	creator.AddLine("", nil, util.IptablesAppendFlag, util.IptablesAzureChain, util.IptablesJumpFlag, util.IptablesAzureIngressChain)
	creator.AddLine("", nil, util.IptablesAppendFlag, util.IptablesAzureChain, util.IptablesJumpFlag, util.IptablesAzureEgressChain)
	creator.AddLine("", nil, util.IptablesAppendFlag, util.IptablesAzureChain, util.IptablesJumpFlag, util.IptablesAzureAcceptChain)

	// add AZURE-NPM-INGRESS chain rules
	ingressDropSpecs := []string{util.IptablesAppendFlag, util.IptablesAzureIngressChain, util.IptablesJumpFlag, util.IptablesDrop}
	ingressDropSpecs = append(ingressDropSpecs, onMarkSpecs(util.IptablesAzureIngressDropMarkHex)...)
	ingressDropSpecs = append(ingressDropSpecs, commentSpecs(fmt.Sprintf("DROP-ON-INGRESS-DROP-MARK-%s", util.IptablesAzureIngressDropMarkHex))...)
	creator.AddLine("", nil, ingressDropSpecs...)

	// add AZURE-NPM-INGRESS-ALLOW-MARK chain
	markIngressAllowSpecs := []string{util.IptablesAppendFlag, util.IptablesAzureIngressAllowMarkChain}
	markIngressAllowSpecs = append(markIngressAllowSpecs, setMarkSpecs(util.IptablesAzureIngressAllowMarkHex)...)
	markIngressAllowSpecs = append(markIngressAllowSpecs, commentSpecs(fmt.Sprintf("SET-INGRESS-ALLOW-MARK-%s", util.IptablesAzureIngressAllowMarkHex))...)
	creator.AddLine("", nil, markIngressAllowSpecs...)
	creator.AddLine("", nil, util.IptablesAppendFlag, util.IptablesAzureIngressAllowMarkChain, util.IptablesJumpFlag, util.IptablesAzureEgressChain)

	// add AZURE-NPM-EGRESS chain rules
	egressDropSpecs := []string{util.IptablesAppendFlag, util.IptablesAzureEgressChain, util.IptablesJumpFlag, util.IptablesDrop}
	egressDropSpecs = append(egressDropSpecs, onMarkSpecs(util.IptablesAzureEgressDropMarkHex)...)
	egressDropSpecs = append(egressDropSpecs, commentSpecs(fmt.Sprintf("DROP-ON-EGRESS-DROP-MARK-%s", util.IptablesAzureEgressDropMarkHex))...)
	creator.AddLine("", nil, egressDropSpecs...)

	jumpOnIngressMatchSpecs := []string{util.IptablesAppendFlag, util.IptablesAzureEgressChain, util.IptablesJumpFlag, util.IptablesAzureAcceptChain}
	jumpOnIngressMatchSpecs = append(jumpOnIngressMatchSpecs, onMarkSpecs(util.IptablesAzureIngressAllowMarkHex)...)
	jumpOnIngressMatchSpecs = append(jumpOnIngressMatchSpecs, commentSpecs(fmt.Sprintf("ACCEPT-ON-INGRESS-ALLOW-MARK-%s", util.IptablesAzureIngressAllowMarkHex))...)
	creator.AddLine("", nil, jumpOnIngressMatchSpecs...)

	// add AZURE-NPM-ACCEPT chain rules
	clearSpecs := []string{util.IptablesAppendFlag, util.IptablesAzureAcceptChain}
	clearSpecs = append(clearSpecs, setMarkSpecs(util.IptablesAzureClearMarkHex)...)
	clearSpecs = append(clearSpecs, commentSpecs("Clear-AZURE-NPM-MARKS")...)
	creator.AddLine("", nil, clearSpecs...)
	creator.AddLine("", nil, util.IptablesAppendFlag, util.IptablesAzureAcceptChain, util.IptablesJumpFlag, util.IptablesAccept)
	creator.AddLine("", nil, util.IptablesRestoreCommit)
	return creator
}

// add/reposition the jump from FORWARD chain to AZURE-NPM chain so that it is the first rule in the chain
func (pMgr *PolicyManager) positionAzureChainJumpRule() error {
	azureChainLineNum, lineNumErr := pMgr.chainLineNumber(util.IptablesAzureChain)
	if lineNumErr != nil {
		baseErrString := "failed to get index of jump from FORWARD chain to AZURE-NPM chain"
		metrics.SendErrorLogAndMetric(util.IptmID, "Error: %s: %s", baseErrString, lineNumErr.Error())
		// FIXME update ID
		return npmerrors.SimpleErrorWrapper(baseErrString, lineNumErr)
	}

	// 1. the jump to azure chain is already the first rule , as it should be
	if azureChainLineNum == 1 {
		return nil
	}
	// 2. the jump to auzre chain does not exist, so we need to add it
	if azureChainLineNum == 0 {
		klog.Infof("Inserting jump from FORWARD chain to AZURE-NPM chain")
		if insertErrCode, insertErr := pMgr.runIPTablesCommand(util.IptablesInsertionFlag, jumpFromForwardToAzureChainArgs...); insertErr != nil {
			baseErrString := "failed to insert jump from FORWARD chain to AZURE-NPM chain"
			metrics.SendErrorLogAndMetric(util.IptmID, "Error: %s with error code %d and error %s", baseErrString, insertErrCode, insertErr.Error())
			// FIXME update ID
			return npmerrors.SimpleErrorWrapper(baseErrString, insertErr)
		}
		return nil
	}
	// 3. the jump to azure chain is not the first rule, so we need to reposition it
	metrics.SendErrorLogAndMetric(util.IptmID, "Info: Reconciler deleting and re-adding jump from FORWARD chain to AZURE-NPM chain table.")
	if deleteErrCode, deleteErr := pMgr.runIPTablesCommand(util.IptablesDeletionFlag, jumpFromForwardToAzureChainArgs...); deleteErr != nil {
		baseErrString := "failed to delete jump from FORWARD chain to AZURE-NPM chain"
		metrics.SendErrorLogAndMetric(util.IptmID, "Error: %s with error code %d and error %s", baseErrString, deleteErrCode, deleteErr.Error())
		// FIXME update ID
		return npmerrors.SimpleErrorWrapper(baseErrString, deleteErr)
	}
	if insertErrCode, insertErr := pMgr.runIPTablesCommand(util.IptablesInsertionFlag, jumpFromForwardToAzureChainArgs...); insertErr != nil {
		baseErrString := "after deleting, failed to insert jump from FORWARD chain to AZURE-NPM chain"
		// FIXME update ID
		metrics.SendErrorLogAndMetric(util.IptmID, "Error: %s with error code %d and error %s", baseErrString, insertErrCode, insertErr.Error())
		return npmerrors.SimpleErrorWrapper(baseErrString, insertErr)
	}
	return nil
}

// returns 0 if the chain does not exist
// this function has a direct comparison in NPM v1 iptables manager (iptm.go)
func (pMgr *PolicyManager) chainLineNumber(chain string) (int, error) {
	listForwardEntriesCommand := pMgr.ioShim.Exec.Command(util.Iptables,
		util.IptablesWaitFlag, defaultlockWaitTimeInSeconds, util.IptablesTableFlag, util.IptablesFilterTable,
		util.IptablesNumericFlag, util.IptablesListFlag, util.IptablesForwardChain, util.IptablesLineNumbersFlag,
	)
	grepCommand := pMgr.ioShim.Exec.Command(ioutil.Grep, chain)
	searchResults, gotMatches, err := ioutil.PipeCommandToGrep(listForwardEntriesCommand, grepCommand)
	if err != nil {
		return 0, npmerrors.SimpleErrorWrapper(fmt.Sprintf("failed to determine line number for jump from FORWARD chain to %s chain", chain), err)
	}
	if !gotMatches {
		return 0, nil
	}
	if len(searchResults) >= minLineNumberStringLength {
		lineNum, _ := strconv.Atoi(string(searchResults[0])) // FIXME this returns the first digit of the line number. What if the chain was at line 11? Then we would think it's at line 1
		return lineNum, nil
	}
	return 0, nil
}

// make this a function for easier testing
func (pMgr *PolicyManager) creatorAndChainsForReset() (creator *ioutil.FileCreator, chainsToFlush []string) {
	// get current chains because including them in the restore file would create them if they don't exist
	chainsToFlush, err := pMgr.allCurrentAzureChains()
	if err != nil {
		metrics.SendErrorLogAndMetric(util.IptmID, "Error: failed to determine NPM ingress/egress policy chains to delete")
	}
	creator = pMgr.newCreatorWithChains(chainsToFlush)
	creator.AddLine("", nil, util.IptablesRestoreCommit)
	return
}

func (pMgr *PolicyManager) allCurrentAzureChains() ([]string, error) {
	iptablesListCommand := pMgr.ioShim.Exec.Command(util.Iptables,
		util.IptablesWaitFlag, defaultlockWaitTimeInSeconds, util.IptablesTableFlag, util.IptablesFilterTable,
		util.IptablesNumericFlag, util.IptablesListFlag,
	)
	grepCommand := pMgr.ioShim.Exec.Command(ioutil.Grep, azureChainGrepPattern)
	searchResults, gotMatches, err := ioutil.PipeCommandToGrep(iptablesListCommand, grepCommand)
	if err != nil {
		return nil, npmerrors.SimpleErrorWrapper("failed to get policy chain names", err)
	}
	if !gotMatches {
		return nil, nil
	}
	lines := strings.Split(string(searchResults), "\n")
	chainNames := make([]string, 0, len(lines)) // don't want to preallocate size in case of have malformed lines
	for _, line := range lines {
		// line of the form "Chain NAME (1 references)"
		spaceSeparatedLine := strings.Split(line, " ")
		if len(spaceSeparatedLine) < minSpacedSectionsForChainLine || len(spaceSeparatedLine[1]) < minAzureChainNameLength {
			klog.Errorf("got unexpected grep output [%s] for ingress/egress policy chains", line)
		} else {
			chainNames = append(chainNames, spaceSeparatedLine[1])
		}
	}
	return chainNames, nil
}

func onMarkSpecs(mark string) []string {
	return []string{
		util.IptablesModuleFlag,
		util.IptablesMarkVerb,
		util.IptablesMarkFlag,
		mark,
	}
}
