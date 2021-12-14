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
	// Must loop through a slice because we need a deterministic order for fexec commands for UTs.
	iptablesAzureChains = []string{
		util.IptablesAzureChain,
		util.IptablesAzureIngressChain,
		util.IptablesAzureIngressAllowMarkChain,
		util.IptablesAzureEgressChain,
		util.IptablesAzureAcceptChain,
	}
	// Should not be used directly. Initialized from iptablesAzureChains on first use of isAzureChain().
	iptablesAzureChainsMap map[string]struct{}

	jumpFromForwardToAzureChainArgs = []string{
		util.IptablesForwardChain,
		util.IptablesJumpFlag,
		util.IptablesAzureChain,
		util.IptablesModuleFlag,
		util.IptablesCtstateModuleFlag,
		util.IptablesCtstateFlag,
		util.IptablesNewState,
	}

	errInvalidGrepResult                      = errors.New("unexpectedly got no lines while grepping for current Azure chains")
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

// Adds the chain if it isn't one of the iptablesAzureChains.
// This protects against trying to delete any core NPM chain.
func (s *staleChains) add(chain string) {
	if !isBaseChain(chain) {
		s.chainsToCleanup[chain] = struct{}{}
	}
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

func isBaseChain(chain string) bool {
	if iptablesAzureChainsMap == nil {
		iptablesAzureChainsMap = make(map[string]struct{})
		for _, chain := range iptablesAzureChains {
			iptablesAzureChainsMap[chain] = struct{}{}
		}
	}
	_, exist := iptablesAzureChainsMap[chain]
	return exist
}

/*
	Called once at startup.
	Like the rest of PolicyManager, minimizes the number of OS calls by consolidating all possible actions into one iptables-restore call.

	1. Delete the deprecated jump from FORWARD to AZURE-NPM chain (if it exists).
	2. Cleanup old NPM chains, and configure base chains and their rules.
		1. Do the following via iptables-restore --noflush:
			- flush all deprecated chains
			- flush old v2 policy chains
			- create/flush the base chains
			- add rules for the base chains, except for AZURE-NPM (so that PolicyManager will be deactivated)
		2. In the background:
			- delete all deprecated chains
			- delete old v2 policy chains
	3. Add/reposition the jump from FORWARD chain to AZURE-NPM chain.
		TODO: add this jump (if necessary) in the iptables-restore call

	TODO: could use one grep call instead of one for getting the jump line num and one for getting deprecated chains and old v2 policy chains
		- should use a grep pattern like so: <line num...AZURE-NPM>|<Chain AZURE-NPM>
*/
func (pMgr *PolicyManager) bootup(_ []string) error {
	klog.Infof("booting up iptables Azure chains")

	// 1. delete the deprecated jump to AZURE-NPM
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

	currentChains, err := pMgr.allCurrentAzureChains()
	if err != nil {
		return npmerrors.SimpleErrorWrapper("failed to get current chains for bootup", err)
	}

	// 2. cleanup old NPM chains, and configure base chains and their rules.
	creator := pMgr.creatorForBootup(currentChains)
	if err := restore(creator); err != nil {
		return npmerrors.SimpleErrorWrapper("failed to run iptables-restore for bootup", err)
	}

	// 3. add/reposition the jump to AZURE-NPM
	if err := pMgr.positionAzureChainJumpRule(); err != nil {
		baseErrString := "failed to add/reposition jump from FORWARD chain to AZURE-NPM chain"
		metrics.SendErrorLogAndMetric(util.IptmID, "Error: %s with error: %s", baseErrString, err.Error())
		return npmerrors.SimpleErrorWrapper(baseErrString, err) // we used to ignore this error in v1
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
		klog.Errorf("failed to clean up old policy chains with the following error: %s", err.Error())
	}
}

// cleanupChains deletes all the chains in the given list.
// If a chain fails to delete and it isn't one of the iptablesAzureChains, then it is added to the staleChains.
// have to use slice argument for deterministic behavior for ioshim in UTs
func (pMgr *PolicyManager) cleanupChains(chains []string) error {
	var aggregateError error
	for _, chain := range chains {
		errCode, err := pMgr.runIPTablesCommand(util.IptablesDestroyFlag, chain)
		if err != nil && errCode != doesNotExistErrorCode {
			// add to staleChains if it's not one of the iptablesAzureChains
			pMgr.staleChains.add(chain)
			currentErrString := fmt.Sprintf("failed to clean up chain %s with err [%v]", chain, err)
			if aggregateError == nil {
				aggregateError = npmerrors.SimpleError(currentErrString)
			} else {
				aggregateError = npmerrors.SimpleErrorWrapper(fmt.Sprintf("%s and had previous error", currentErrString), aggregateError)
			}
		}
	}
	if aggregateError != nil {
		return npmerrors.SimpleErrorWrapper("failed to clean up some chains", aggregateError)
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

// Writes the restore file for bootup, and marks the following as stale: deprecated chains and old v2 policy chains.
// This is a separate function to help with UTs.
func (pMgr *PolicyManager) creatorForBootup(currentChains map[string]struct{}) *ioutil.FileCreator {
	pMgr.staleChains.empty()
	chainsToCreate := make([]string, 0, len(iptablesAzureChains))
	for _, chain := range iptablesAzureChains {
		_, exists := currentChains[chain]
		if !exists {
			chainsToCreate = append(chainsToCreate, chain)
		}
	}

	// Step 2.1 in bootup() comment: cleanup old NPM chains, and configure base chains and their rules
	// To leave NPM deactivated, don't specify any rules for AZURE-NPM chain.
	creator := pMgr.newCreatorWithChains(chainsToCreate)
	for chain := range currentChains {
		creator.AddLine("", nil, fmt.Sprintf("-F %s", chain))
		// Step 2.2 in bootup() comment: delete deprecated chains and old v2 policy chains in the background
		pMgr.staleChains.add(chain) // won't add base chains
	}

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
	clearSpecs = append(clearSpecs, commentSpecs("CLEAR-AZURE-NPM-MARKS")...)
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

func (pMgr *PolicyManager) allCurrentAzureChains() (map[string]struct{}, error) {
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
	if len(lines) == 1 && lines[0] == "" {
		// this should never happen: gotMatches is true, but there is no content in the searchResults
		return nil, errInvalidGrepResult
	}
	lastIndex := len(lines) - 1
	lastLine := lines[lastIndex]
	if lastLine == "" {
		// remove the last empty line (since each line ends with a newline)
		lines = lines[:lastIndex] // this line doesn't impact the array that the slice references
	} else {
		klog.Errorf(`while grepping for current Azure chains, expected last line to end in "" but got [%s]. full grep output: [%s]`, lastLine, string(searchResults))
	}
	chainNames := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		// line of the form "Chain NAME (1 references)"
		spaceSeparatedLine := strings.Split(line, " ")
		if len(spaceSeparatedLine) < minSpacedSectionsForChainLine || len(spaceSeparatedLine[1]) < minAzureChainNameLength {
			klog.Errorf("while grepping for current Azure chains, got unexpected line [%s] for all current azure chains. full grep output: [%s]", line, string(searchResults))
		} else {
			chainNames[spaceSeparatedLine[1]] = struct{}{}
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
