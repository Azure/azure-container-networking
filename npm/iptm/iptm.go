// Part of this file is modified from iptables package from Kuberenetes.
// https://github.com/kubernetes/kubernetes/blob/master/pkg/util/iptables

package iptm

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/util"
	utilexec "k8s.io/utils/exec"
	// utiliptables "k8s.io/kubernetes/pkg/util/iptables"
)

const (
	defaultlockWaitTimeInSeconds string = "60"
	iptablesErrDoesNotExist      int    = 1
	reconcileChainTimeInMinutes         = 5
)

// IptablesAzureChainList contains list of all NPM chains
var IptablesAzureChainList = []string{
	util.IptablesAzureChain,
	util.IptablesAzureAcceptChain,
	util.IptablesAzureIngressChain,
	util.IptablesAzureEgressChain,
	util.IptablesAzureIngressPortChain,
	util.IptablesAzureIngressFromChain,
	util.IptablesAzureEgressPortChain,
	util.IptablesAzureEgressToChain,
	util.IptablesAzureIngressDropsChain,
	util.IptablesAzureEgressDropsChain,
}

var deprecatedJumpToAzureEntry = &IptEntry{
	Chain: util.IptablesForwardChain,
	Specs: []string{
		util.IptablesJumpFlag,
		util.IptablesAzureChain,
	},
}

// IptEntry represents an iptables rule.
type IptEntry struct {
	Command               string
	Name                  string
	Chain                 string
	Flag                  string
	LockWaitTimeInSeconds string
	Specs                 []string
}

// IptablesManager stores iptables entries.
type IptablesManager struct {
	exec                 utilexec.Interface
	io                   ioshim
	OperationFlag        string
	placeAzureChainFirst bool
}

func isDropsChain(chainName string) bool {
	// Check if the chain name is one of the two DROP chains
	if (chainName == util.IptablesAzureIngressDropsChain) ||
		(chainName == util.IptablesAzureEgressDropsChain) {
		return true
	}
	return false
}

// NewIptablesManager creates a new instance for IptablesManager object.
func NewIptablesManager(exec utilexec.Interface, io ioshim, placeAzureChainFirst bool) *IptablesManager {
	iptMgr := &IptablesManager{
		exec:                 exec,
		io:                   io,
		OperationFlag:        "",
		placeAzureChainFirst: placeAzureChainFirst,
	}

	return iptMgr
}

// NewIptablesManager creates a new instance for IptablesManager object.

// InitNpmChains initializes Azure NPM chains in iptables.
func (iptMgr *IptablesManager) InitNpmChains() error {
	log.Logf("Initializing AZURE-NPM chains.")

	if err := iptMgr.addAllChains(); err != nil {
		return err
	}

	if err := iptMgr.checkAndAddForwardChain(); err != nil {
		metrics.SendErrorLogAndMetric(util.IptmID, "Error: failed to add AZURE-NPM chain to FORWARD chain. %s", err.Error())
	}

	return iptMgr.addAllRulesToChains()
}

// UninitNpmChains uninitializes Azure NPM chains in iptables.
func (iptMgr *IptablesManager) UninitNpmChains() error {
	// Remove AZURE-NPM chain from FORWARD chain.
	iptMgr.OperationFlag = util.IptablesDeletionFlag
	errCode, err := iptMgr.run(deprecatedJumpToAzureEntry)
	if errCode != iptablesErrDoesNotExist && err != nil {
		metrics.SendErrorLogAndMetric(util.IptmID, "Error: failed to delete deprecated jump from FORWARD chain to AZURE-NPM")
		return err
	}

	entry := &IptEntry{
		Chain: util.IptablesForwardChain,
		Specs: []string{
			util.IptablesJumpFlag,
			util.IptablesAzureChain,
			util.IptablesModuleFlag,
			util.IptablesCtstateModuleFlag,
			util.IptablesCtstateFlag,
			util.IptablesNewState,
		},
	}
	errCode, err = iptMgr.run(entry)
	if errCode != iptablesErrDoesNotExist && err != nil {
		metrics.SendErrorLogAndMetric(util.IptmID, "Error: failed to delete AZURE-NPM from Forward chain")
		return err
	}

	// For backward compatibility, we should be cleaning older chains.
	// TODO(jungukcho): need to check K8s or NPM version and do it selectively
	// to avoid unnecessary call.
	allAzureChains := append(IptablesAzureChainList,
		util.IptablesAzureTargetSetsChain,
		util.IptablesAzureIngressWrongDropsChain,
	)

	iptMgr.OperationFlag = util.IptablesFlushFlag
	for _, chain := range allAzureChains {
		entry := &IptEntry{
			Chain: chain,
		}
		errCode, err := iptMgr.run(entry)
		if errCode != iptablesErrDoesNotExist && err != nil {
			metrics.SendErrorLogAndMetric(util.IptmID, "Error: failed to flush iptables chain %s.", chain)
		}
	}

	for _, chain := range allAzureChains {
		if err := iptMgr.deleteChain(chain); err != nil {
			return err
		}
	}

	return nil
}

// Add adds a rule in iptables.
func (iptMgr *IptablesManager) Add(entry *IptEntry) error {
	prometheusTimer := metrics.StartNewTimer()
	defer metrics.RecordACLRuleExecTime(prometheusTimer) // record execution time regardless of failure

	log.Logf("Adding iptables entry: %+v.", entry)

	// Since there is a RETURN statement added to each DROP chain, we need to make sure
	// any new DROP rule added to ingress or egress DROPS chain is added at the BOTTOM
	if isDropsChain(entry.Chain) {
		iptMgr.OperationFlag = util.IptablesAppendFlag
	} else {
		iptMgr.OperationFlag = util.IptablesInsertionFlag
	}
	if _, err := iptMgr.run(entry); err != nil {
		metrics.SendErrorLogAndMetric(util.IptmID, "Error: failed to create iptables rules.")
		return err
	}

	metrics.IncNumACLRules()

	return nil
}

// Delete removes a rule in iptables.
func (iptMgr *IptablesManager) Delete(entry *IptEntry) error {
	log.Logf("Deleting iptables entry: %+v", entry)

	exists, err := iptMgr.exists(entry)
	if err != nil {
		return err
	}

	if !exists {
		return nil
	}

	iptMgr.OperationFlag = util.IptablesDeletionFlag
	if _, err := iptMgr.run(entry); err != nil {
		metrics.SendErrorLogAndMetric(util.IptmID, "Error: failed to delete iptables rules.")
		return err
	}

	metrics.DecNumACLRules()
	return nil
}

func (iptMgr *IptablesManager) ReconcileIPTables(stopCh <-chan struct{}) {
	// (TODO) Ideally, we only need this when network policy installs iptables
	// Control below two functions with InitNpmChains and UninitNpmChains functions together
	go iptMgr.reconcileChains(stopCh)
}

// checkAndAddForwardChain initializes and reconciles Azure-NPM chain in right order
func (iptMgr *IptablesManager) checkAndAddForwardChain() error {
	// TODO Adding this chain is printing error messages try to clean it up
	if err := iptMgr.addChain(util.IptablesAzureChain); err != nil {
		return err
	}

	// Insert AZURE-NPM chain to FORWARD chain.
	entry := &IptEntry{
		Chain: util.IptablesForwardChain,
		Specs: []string{
			util.IptablesJumpFlag,
			util.IptablesAzureChain,
			util.IptablesModuleFlag,
			util.IptablesCtstateModuleFlag,
			util.IptablesCtstateFlag,
			util.IptablesNewState,
		},
	}

	var index int
	var kubeServicesLine int
	if !iptMgr.placeAzureChainFirst {
		// retrieve KUBE-SERVICES index
		var err error
		kubeServicesLine, err = iptMgr.getChainLineNumber(util.IptablesKubeServicesChain, util.IptablesForwardChain)
		if err != nil {
			metrics.SendErrorLogAndMetric(util.IptmID, "Error: failed to get index of KUBE-SERVICES in FORWARD chain with error: %s", err.Error())
			return err
		}
		index = kubeServicesLine + 1 // insert the jump to AZURE-NPM after the jump to KUBE-SERVICES
	}

	exists, err := iptMgr.exists(entry)
	if err != nil {
		return err
	}

	if !exists {
		// position Azure-NPM chain after Kube-Forward and Kube-Service chains if it exists
		iptMgr.OperationFlag = util.IptablesInsertionFlag
		if !iptMgr.placeAzureChainFirst {
			entry.Specs = append([]string{strconv.Itoa(index)}, entry.Specs...)
		}
		if _, err = iptMgr.run(entry); err != nil {
			metrics.SendErrorLogAndMetric(util.IptmID, "Error: failed to add AZURE-NPM chain to FORWARD chain.")
			return err
		}

		return nil
	}

	npmChainLine, err := iptMgr.getChainLineNumber(util.IptablesAzureChain, util.IptablesForwardChain)
	if err != nil {
		metrics.SendErrorLogAndMetric(util.IptmID, "Error: failed to get index of AZURE-NPM in FORWARD chain with error: %s", err.Error())
		return err
	}

	if iptMgr.placeAzureChainFirst {
		if npmChainLine == 1 {
			return nil
		}
	} else {
		// Kube-services line number is less than npm chain line number then all good
		if kubeServicesLine < npmChainLine || kubeServicesLine <= 0 {
			return nil
		}
	}

	errCode := 0
	// NPM Chain number is less than KUBE-SERVICES then
	// delete existing NPM chain and add it in the right order
	iptMgr.OperationFlag = util.IptablesDeletionFlag
	metrics.SendErrorLogAndMetric(util.IptmID, "Info: Reconciler deleting and re-adding AZURE-NPM in FORWARD table.")
	if errCode, err = iptMgr.run(entry); err != nil {
		metrics.SendErrorLogAndMetric(util.IptmID, "Error: failed to delete AZURE-NPM chain from FORWARD chain with error code %d.", errCode)
		return err
	}
	iptMgr.OperationFlag = util.IptablesInsertionFlag
	if !iptMgr.placeAzureChainFirst {
		// Reduce index for deleted AZURE-NPM chain
		index--
		entry.Specs = append([]string{strconv.Itoa(index)}, entry.Specs...)
	}
	if errCode, err = iptMgr.run(entry); err != nil {
		metrics.SendErrorLogAndMetric(util.IptmID, "Error: failed to add AZURE-NPM chain to FORWARD chain with error code %d.", errCode)
		return err
	}

	return nil
}

// reconcileChains checks for ordering of AZURE-NPM chain in FORWARD chain periodically.
func (iptMgr *IptablesManager) reconcileChains(stopCh <-chan struct{}) {
	ticker := time.NewTicker(time.Minute * time.Duration(reconcileChainTimeInMinutes))
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			if err := iptMgr.checkAndAddForwardChain(); err != nil {
				metrics.SendErrorLogAndMetric(util.NpmID, "Error: failed to reconcileChains Azure-NPM due to %s", err.Error())
			}
		}
	}
}

// addAllRulesToChains checks and adds all the rules in NPM chains
func (iptMgr *IptablesManager) addAllRulesToChains() error {
	allDefaultRules := getAllDefaultRules()
	for _, rule := range allDefaultRules {
		entry := &IptEntry{
			Chain: rule[0],
			Specs: rule[1:],
		}
		exists, err := iptMgr.exists(entry)
		if err != nil {
			return err
		}

		if !exists {
			iptMgr.OperationFlag = util.IptablesAppendFlag
			if _, err = iptMgr.run(entry); err != nil {
				msg := "Error: failed to add %s to parent chain %s"
				switch {
				case len(rule) == 3:
					// 0th index is parent chain and 2nd is chain to be added
					msg = fmt.Sprintf(msg, rule[2], rule[0])
				case len(rule) > 3:
					// last element is comment
					msg = fmt.Sprintf(msg, rule[len(rule)-1], rule[0])
				default:
					msg = "Error: failed to add main chains with invalid rule length"
				}

				metrics.SendErrorLogAndMetric(util.IptmID, msg)
				return err
			}
		}

	}

	return nil
}

// Exists checks if a rule exists in iptables.
func (iptMgr *IptablesManager) exists(entry *IptEntry) (bool, error) {
	iptMgr.OperationFlag = util.IptablesCheckFlag
	returnCode, err := iptMgr.run(entry)
	if err == nil {
		return true, nil
	}

	if returnCode == iptablesErrDoesNotExist {
		return false, nil
	}

	return false, err
}

// AddAllChains adds all NPM chains
func (iptMgr *IptablesManager) addAllChains() error {
	// Add all secondary Chains
	for _, chainToAdd := range IptablesAzureChainList {
		if err := iptMgr.addChain(chainToAdd); err != nil {
			return err
		}
	}
	return nil
}

// AddChain adds a chain to iptables.
func (iptMgr *IptablesManager) addChain(chain string) error {
	entry := &IptEntry{
		Chain: chain,
	}
	iptMgr.OperationFlag = util.IptablesChainCreationFlag
	errCode, err := iptMgr.run(entry)
	if err != nil {
		if errCode == iptablesErrDoesNotExist {
			log.Logf("Chain already exists %s.", entry.Chain)
			return nil
		}

		metrics.SendErrorLogAndMetric(util.IptmID, "Error: failed to create iptables chain %s.", entry.Chain)
		return err
	}

	return nil
}

// GetChainLineNumber given a Chain and its parent chain returns line number
func (iptMgr *IptablesManager) getChainLineNumber(chain string, parentChain string) (int, error) {
	var (
		output []byte
		err    error
	)

	cmdName := util.Iptables
	cmdArgs := []string{"-t", "filter", "-n", "--list", parentChain, "--line-numbers"}

	iptFilterEntries := iptMgr.exec.Command(cmdName, cmdArgs...)
	grep := iptMgr.exec.Command("grep", chain)
	pipe, err := iptFilterEntries.StdoutPipe()
	if err != nil {
		return 0, err
	}
	defer pipe.Close()
	grep.SetStdin(pipe)

	if err = iptFilterEntries.Start(); err != nil {
		return 0, err
	}
	// Without this wait, defunct iptable child process are created
	defer iptFilterEntries.Wait()

	if output, err = grep.CombinedOutput(); err != nil {
		// grep returns err status 1 if not found
		return 0, nil
	}

	if len(output) > 2 {
		lineNum, _ := strconv.Atoi(string(output[0]))
		return lineNum, nil
	}
	return 0, nil
}

// DeleteChain deletes a chain from iptables.
func (iptMgr *IptablesManager) deleteChain(chain string) error {
	entry := &IptEntry{
		Chain: chain,
	}
	iptMgr.OperationFlag = util.IptablesDestroyFlag
	errCode, err := iptMgr.run(entry)
	if err != nil {
		if errCode == iptablesErrDoesNotExist {
			log.Logf("Chain doesn't exist %s.", entry.Chain)
			return nil
		}

		metrics.SendErrorLogAndMetric(util.IptmID, "Error: failed to delete iptables chain %s.", entry.Chain)
		return err
	}

	return nil
}

// Run execute an iptables command to update iptables.
func (iptMgr *IptablesManager) run(entry *IptEntry) (int, error) {
	cmdName := entry.Command
	if cmdName == "" {
		cmdName = util.Iptables
	}

	if entry.LockWaitTimeInSeconds == "" {
		entry.LockWaitTimeInSeconds = defaultlockWaitTimeInSeconds
	}

	cmdArgs := append([]string{util.IptablesWaitFlag, entry.LockWaitTimeInSeconds, iptMgr.OperationFlag, entry.Chain}, entry.Specs...)

	if iptMgr.OperationFlag != util.IptablesCheckFlag {
		log.Logf("Executing iptables command %s %v", cmdName, cmdArgs)
	}

	output, err := iptMgr.exec.Command(cmdName, cmdArgs...).CombinedOutput()
	if msg, failed := err.(utilexec.ExitError); failed {
		errCode := msg.ExitStatus()
		if errCode > 0 && iptMgr.OperationFlag != util.IptablesCheckFlag {
			msgStr := strings.TrimSuffix(string(output), "\n")
			if strings.Contains(msgStr, "Chain already exists") && iptMgr.OperationFlag == util.IptablesChainCreationFlag {
				return 0, nil
			}
			metrics.SendErrorLogAndMetric(util.IptmID, "Error: There was an error running command: [%s %v] Stderr: [%v, %s]", cmdName, strings.Join(cmdArgs, " "), err, msgStr)
		}

		return errCode, err
	}

	return 0, nil
}

// TO-DO :- Use iptables-restore to update iptables.
// func SyncIptables(entries []*IptEntry) error {
// 	// Ensure main chains and rules are installed.
// 	tablesNeedServicesChain := []utiliptables.Table{utiliptables.TableFilter, utiliptables.TableNAT}
// 	for _, table := range tablesNeedServicesChain {
// 		if _, err := proxier.iptables.EnsureChain(table, iptablesServicesChain); err != nil {
// 			glog.Errorf("Failed to ensure that %s chain %s exists: %v", table, iptablesServicesChain, err)
// 			return
// 		}
// 	}

// 	// Get iptables-save output so we can check for existing chains and rules.
// 	// This will be a map of chain name to chain with rules as stored in iptables-save/iptables-restore
// 	existingFilterChains := make(map[utiliptables.Chain]string)
// 	iptablesSaveRaw, err := proxier.iptables.Save(utiliptables.TableFilter)
// 	if err != nil { // if we failed to get any rules
// 		glog.Errorf("Failed to execute iptables-save, syncing all rules. %s", err.Error())
// 	} else { // otherwise parse the output
// 		existingFilterChains = getChainLines(utiliptables.TableFilter, iptablesSaveRaw)
// 	}

// 	// Write table headers.
// 	writeLine(filterChains, "*filter")

// }
