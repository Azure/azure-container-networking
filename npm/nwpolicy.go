// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"github.com/Azure/azure-container-networking/log"
	networkingv1 "k8s.io/api/networking/v1"
)

// UpdateNetworkPolicy handles updating network policy.
func (npMgr *NetworkPolicyManager) UpdateNetworkPolicy(oldNpObj *networkingv1.NetworkPolicy, newNpObj *networkingv1.NetworkPolicy) error {
	var err error

	log.Printf("NETWORK POLICY UPDATING:\n old policy:[%v]\n new policy:[%v]", oldNpObj, newNpObj)

	if err = npMgr.DeleteNetworkPolicy(oldNpObj); err != nil {
		return err
	}

	if newNpObj.ObjectMeta.DeletionTimestamp == nil && newNpObj.ObjectMeta.DeletionGracePeriodSeconds == nil {
		if err = npMgr.AddNetworkPolicy(newNpObj); err != nil {
			return err
		}
	}

	return nil
}
