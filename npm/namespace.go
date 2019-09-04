// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/util"

	corev1 "k8s.io/api/core/v1"
)

func isSystemNs(nsObj *corev1.Namespace) bool {
	return nsObj.ObjectMeta.Name == util.KubeSystemFlag
}

// UpdateNamespace handles updating namespace in ipset.
func (npMgr *NetworkPolicyManager) UpdateNamespace(oldNsObj *corev1.Namespace, newNsObj *corev1.Namespace) error {
	var err error

	oldNsName, oldNsNs, oldNsLabel := oldNsObj.ObjectMeta.Name, oldNsObj.ObjectMeta.Namespace, oldNsObj.ObjectMeta.Labels
	newNsName, newNsNs, newNsLabel := newNsObj.ObjectMeta.Name, newNsObj.ObjectMeta.Namespace, newNsObj.ObjectMeta.Labels
	log.Printf(
		"NAMESPACE UPDATING:\n old namespace: [%s/%s/%+v]\n new namespace: [%s/%s/%+v]",
		oldNsName, oldNsNs, oldNsLabel, newNsName, newNsNs, newNsLabel,
	)

	if err = npMgr.DeleteNamespace(oldNsObj); err != nil {
		return err
	}

	if newNsObj.ObjectMeta.DeletionTimestamp == nil && newNsObj.ObjectMeta.DeletionGracePeriodSeconds == nil {
		if err = npMgr.AddNamespace(newNsObj); err != nil {
			return err
		}
	}

	return nil
}
