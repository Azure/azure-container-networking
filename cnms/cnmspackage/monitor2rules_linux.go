package cnms

import (
	"fmt"

	"github.com/Azure/azure-container-networking/ebtables"
	"github.com/Azure/azure-container-networking/log"
)

// deleteRulesNotExistInMap deletes rules from nat Ebtable if rule was not in stateRules after a certain number of iterations.
func (networkMonitor *NetworkMonitor) deleteRulesNotExistInMap(chainRules map[string]string, stateRules map[string]string) {

	table := ebtables.Nat
	action := ebtables.Delete

	for rule, chain := range chainRules {
		if _, ok := stateRules[rule]; !ok {
			if itr, ok := networkMonitor.DeleteRulesToBeValidated[rule]; ok && itr > 0 {
				buf := fmt.Sprintf("[monitor] Deleting Ebtable rule as it didn't exist in state for %d iterations chain %v rule %v", itr, chain, rule)
				if err := ebtables.SetEbRule(table, action, chain, rule); err != nil {
					buf = fmt.Sprintf("[monitor] Error while deleting ebtable rule %v", err)
				}

				log.Printf(buf)
				networkMonitor.CNIReport.ErrorMessage = buf
				networkMonitor.CNIReport.OperationType = "EBTableDelete"
				delete(networkMonitor.DeleteRulesToBeValidated, rule)
			} else {
				log.Printf("[DELETE] Found unmatched rule chain %v rule %v itr %d. Giving one more iteration.", chain, rule, itr)
				networkMonitor.DeleteRulesToBeValidated[rule] = itr + 1
			}
		}
	}
}

// addRulesNotExistInMap adds rules to nat Ebtable if rule was in stateRules and not in current chain rules after a certain number of iterations.
func (networkMonitor *NetworkMonitor) addRulesNotExistInMap(
	stateRules map[string]string,
	chainRules map[string]string) {

	table := ebtables.Nat
	action := ebtables.Append

	for rule, chain := range stateRules {
		if _, ok := chainRules[rule]; !ok {
			if itr, ok := networkMonitor.AddRulesToBeValidated[rule]; ok && itr > 0 {
				buf := fmt.Sprintf("[monitor] Adding Ebtable rule as it existed in state rules but not in current chain rules for %d iterations chain %v rule %v", itr, chain, rule)
				if err := ebtables.SetEbRule(table, action, chain, rule); err != nil {
					buf = fmt.Sprintf("[monitor] Error while adding ebtable rule %v", err)
				}

				log.Printf(buf)
				networkMonitor.CNIReport.ErrorMessage = buf
				networkMonitor.CNIReport.OperationType = "EBTableAdd"
				delete(networkMonitor.AddRulesToBeValidated, rule)
			} else {
				log.Printf("[ADD] Found unmatched rule chain %v rule %v itr %d. Giving one more iteration.", chain, rule, itr)
				networkMonitor.AddRulesToBeValidated[rule] = itr + 1
			}
		}
	}
}

// CreateRequiredL2Rules finds the rules that should be in nat ebtable based on state.
func (networkMonitor *NetworkMonitor) CreateRequiredL2Rules(
	currentEbtableRulesMap map[string]string,
	currentStateRulesMap map[string]string) error {

	for rule := range networkMonitor.AddRulesToBeValidated {
		if _, ok := currentStateRulesMap[rule]; !ok {
			delete(networkMonitor.AddRulesToBeValidated, rule)
		}
	}

	networkMonitor.addRulesNotExistInMap(currentStateRulesMap, currentEbtableRulesMap)

	return nil
}

// RemoveInvalidL2Rules removes rules that should not be in nat ebtable based on state.
func (networkMonitor *NetworkMonitor) RemoveInvalidL2Rules(
	currentEbtableRulesMap map[string]string,
	currentStateRulesMap map[string]string) error {

	for rule := range networkMonitor.DeleteRulesToBeValidated {
		if _, ok := currentEbtableRulesMap[rule]; !ok {
			delete(networkMonitor.DeleteRulesToBeValidated, rule)
		}
	}

	networkMonitor.deleteRulesNotExistInMap(currentEbtableRulesMap, currentStateRulesMap)

	return nil
}

// generateL2RulesMap gets rules from chainName and puts them in currentEbtableRulesMap.
func generateL2RulesMap(currentEbtableRulesMap map[string]string, chainName string) error {
	table := ebtables.Nat
	rules, err := ebtables.GetEbtableRules(table, chainName)
	if err != nil {
		log.Printf("[monitor] Error while getting rules list from table %v chain %v. Error: %v",
			table, chainName, err)
		return err
	}

	for _, rule := range rules {
		currentEbtableRulesMap[rule] = chainName
	}

	return nil
}

// GetEbTableRulesInMap gathers prerouting and postrouting rules into a map.
func GetEbTableRulesInMap() (map[string]string, error) {
	currentEbtableRulesMap := make(map[string]string)
	if err := generateL2RulesMap(currentEbtableRulesMap, ebtables.PreRouting); err != nil {
		return nil, err
	}

	if err := generateL2RulesMap(currentEbtableRulesMap, ebtables.PostRouting); err != nil {
		return nil, err
	}

	return currentEbtableRulesMap, nil
}
