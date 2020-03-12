package cnms

import (
	"github.com/Azure/azure-container-networking/ebtables"
	"github.com/Azure/azure-container-networking/log"
)

func (networkMonitor *NetworkMonitor) deleteRulesNotExistInMap(chainRules map[string]string, stateRules map[string]string) {

	table := ebtables.Nat
	action := ebtables.Delete

	for rule, chain := range chainRules {
		if _, ok := stateRules[rule]; !ok {
			if itr, ok := networkMonitor.DeleteRulesToBeValidated[rule]; ok && itr > 0 {
				log.Printf("[monitor] Deleting Ebtable rule as it didn't exist in state for %d iterations chain %v rule %v", itr, chain, rule)
				if err := ebtables.SetEbRule(table, action, chain, rule); err != nil {
					log.Printf("[monitor] Error while deleting ebtable rule %v", err)
				}

				delete(networkMonitor.DeleteRulesToBeValidated, rule)
			} else {
				log.Printf("[DELETE] Found unmatched rule chain %v rule %v itr %d. Giving one more iteration", chain, rule, itr)
				networkMonitor.DeleteRulesToBeValidated[rule] = itr + 1
			}
		}
	}
}

func deleteRulesExistInMap(originalChainRules map[string]string, stateRules map[string]string) {

	table := ebtables.Nat
	action := ebtables.Delete

	for rule, chain := range originalChainRules {
		if _, ok := stateRules[rule]; ok {
			log.Printf("[monitor] Deleting Ebtable rule which existed in map %v", rule)
			if err := ebtables.SetEbRule(table, action, chain, rule); err != nil {
				log.Printf("[monitor] Error while deleting ebtable rule %v", err)
			}
		}
	}
}

func (networkMonitor *NetworkMonitor) addRulesNotExistInMap(
	stateRules map[string]string,
	chainRules map[string]string) {

	table := ebtables.Nat
	action := ebtables.Append

	for rule, chain := range stateRules {
		if _, ok := chainRules[rule]; !ok {
			if itr, ok := networkMonitor.AddRulesToBeValidated[rule]; ok && itr > 0 {
				log.Printf("[monitor] Adding Ebtable rule as it didn't exist in state for %d iterations chain %v rule %v", itr, chain, rule)
				if err := ebtables.SetEbRule(table, action, chain, rule); err != nil {
					log.Printf("[monitor] Error while adding ebtable rule %v", err)
				}

				delete(networkMonitor.AddRulesToBeValidated, rule)
			} else {
				log.Printf("[ADD] Found unmatched rule chain %v rule %v itr %d. Giving one more iteration", chain, rule, itr)
				networkMonitor.AddRulesToBeValidated[rule] = itr + 1
			}
		}
	}
}

func (networkMonitor *NetworkMonitor) CreateRequiredL2Rules(
	currentEbtableRulesMap map[string]string,
	currentStateRulesMap map[string]string) error {

	for rule := range networkMonitor.AddRulesToBeValidated {
		log.Printf("[monitor] Rule in AddRulesToBeValidated %v", rule)
		if _, ok := currentStateRulesMap[rule]; !ok {
			log.Printf("[monitor] Deleting Rule from AddRulesToBeValidated %v", rule)
			delete(networkMonitor.AddRulesToBeValidated, rule)
		}
	}

	networkMonitor.addRulesNotExistInMap(currentStateRulesMap, currentEbtableRulesMap)

	return nil
}

func (networkMonitor *NetworkMonitor) RemoveInvalidL2Rules(
	currentEbtableRulesMap map[string]string,
	currentStateRulesMap map[string]string) error {

	for rule := range networkMonitor.DeleteRulesToBeValidated {
		log.Printf("[monitor] Checking DeleteRulesToBeValidated rule: %v", rule)
		if _, ok := currentEbtableRulesMap[rule]; !ok {
			log.Printf("[monitor] DeleteRulesToBeValidated deleting rule: %v", rule)
			delete(networkMonitor.DeleteRulesToBeValidated, rule)
		}
	}

	networkMonitor.deleteRulesNotExistInMap(currentEbtableRulesMap, currentStateRulesMap)

	return nil
}

func generateL2RulesMap(currentEbtableRulesMap map[string]string, chainName string) error {

	table := ebtables.Nat
	rules, err := ebtables.GetEbtableRules(table, chainName)

	if err != nil {
		log.Printf("[monitor] Error while getting rules list from table %v chain %v. Error: %v",
			table, chainName, err)
		return err
	}

	for _, rule := range rules {
		log.Printf("[monitor] Adding rule %s mapped to chainName %s.", rule, chainName)
		currentEbtableRulesMap[rule] = chainName
	}

	return nil
}

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
