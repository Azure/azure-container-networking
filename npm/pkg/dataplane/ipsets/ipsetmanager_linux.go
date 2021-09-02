package ipsets

import (
	"fmt"

	"github.com/Azure/azure-container-networking/npm/util/errors"
)

func (iMgr *IPSetManager) applyIPSets(networkID string) error {
	for setName := range iMgr.dirtyCaches {
		set, exists := iMgr.setMap[setName] // check if the Set exists
		if !exists {
			return errors.Errorf(errors.AppendIPSet, false, fmt.Sprintf("member ipset %s does not exist", setName))
		}

		fmt.Printf(set.Name)

	}
	return nil
}

