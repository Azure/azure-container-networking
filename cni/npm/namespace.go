package npm

import (
	"fmt"

	"github.com/Azure/azure-container-networking/cni/npm/ipsm"
	"github.com/Azure/azure-container-networking/cni/npm/iptm"
	"github.com/Azure/azure-container-networking/cni/npm/util"
	"k8s.io/apimachinery/pkg/types"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
)

type namespace struct {
	name   string
	setMap map[string]string
	podMap map[types.UID]*corev1.Pod
	npMap  map[string]*networkingv1.NetworkPolicy // TODO: Optimize to ordered map.
	ipsMgr *ipsm.IpsetManager
	iptMgr *iptm.IptablesManager
}

// newNS constructs a new namespace object.
func newNs(name string) (*namespace, error) {
	ns := &namespace{
		name:   name,
		setMap: make(map[string]string),
		podMap: make(map[types.UID]*corev1.Pod),
		npMap:  make(map[string]*networkingv1.NetworkPolicy),
		ipsMgr: ipsm.NewIpsetManager(),
		iptMgr: iptm.NewIptablesManager(),
	}

	return ns, nil
}

func isSystemNs(nsObj *corev1.Namespace) bool {
	return nsObj.ObjectMeta.Name == util.KubeSystemFlag
}

func getNsIpsetName(k, v string) string {
	return "ns-" + k + ":" + v
}

// AddNamespace handles add namespace.
func (npMgr *NetworkPolicyManager) AddNamespace(nsObj *corev1.Namespace) error {
	npMgr.Lock()
	defer npMgr.Unlock()

	// Don't deal with kube-system namespace
	if isSystemNs(nsObj) {
		return nil
	}

	nsName, nsNs := nsObj.ObjectMeta.Name, nsObj.ObjectMeta.Namespace
	fmt.Printf("NAMESPACE CREATED: %s/%s\n", nsName, nsNs)

	ns, exists := npMgr.nsMap[nsName]
	if !exists {
		newns, err := newNs(nsName)
		if err != nil {
			return err
		}
		npMgr.nsMap[nsName] = newns
		ns = newns
	}

	// Create ipset for the namespace.
	ipsMgr := ns.ipsMgr
	if err := ipsMgr.CreateSet(nsName); err != nil {
		fmt.Printf("Error creating ipset for namespace %s.\n", nsName)
		return err
	}

	// Add the namespace to its label's ipset list.
	var labelKeys []string
	nsLabels := nsObj.ObjectMeta.Labels
	for nsLabelKey, nsLabelVal := range nsLabels {
		labelKey := getNsIpsetName(nsLabelKey, nsLabelVal)
		fmt.Printf("Adding namespace %s to ipset list %s\n", nsName, labelKey)
		if err := ipsMgr.AddToList(labelKey, nsName); err != nil {
			fmt.Printf("Error Adding namespace %s to ipset list %s\n", nsName, labelKey)
			return err
		}
		labelKeys = append(labelKeys, labelKey)
	}

	ns.setMap = nsObj.ObjectMeta.Labels

	return nil
}

// UpdateNamespace handles update namespace.
func (npMgr *NetworkPolicyManager) UpdateNamespace(oldNsObj *corev1.Namespace, newNsObj *corev1.Namespace) error {
	npMgr.Lock()

	oldNsName, newNsName := oldNsObj.ObjectMeta.Name, newNsObj.ObjectMeta.Name
	fmt.Printf("NAMESPACE UPDATED. %s/%s", oldNsName, newNsName)

	npMgr.Unlock()
	npMgr.DeleteNamespace(oldNsObj)

	if newNsObj.ObjectMeta.DeletionTimestamp == nil && newNsObj.ObjectMeta.DeletionGracePeriodSeconds == nil {
		npMgr.AddNamespace(newNsObj)
	}

	return nil
}

// DeleteNamespace handles delete namespace.
func (npMgr *NetworkPolicyManager) DeleteNamespace(nsObj *corev1.Namespace) error {
	npMgr.Lock()
	defer npMgr.Unlock()

	nsName, nsNs := nsObj.ObjectMeta.Name, nsObj.ObjectMeta.Namespace
	fmt.Printf("NAMESPACE DELETED: %s/%s\n", nsName, nsNs)

	ns, exists := npMgr.nsMap[nsName]
	if !exists {
		return nil
	}

	// Delete the namespace from its label's ipset list.
	ipsMgr := ns.ipsMgr
	var labelKeys []string
	nsLabels := nsObj.ObjectMeta.Labels
	for nsLabelKey, nsLabelVal := range nsLabels {
		labelKey := getNsIpsetName(nsLabelKey, nsLabelVal)
		fmt.Printf("Deleting namespace %s from ipset list %s\n", nsName, labelKey)
		if err := ipsMgr.DeleteFromList(labelKey, nsName); err != nil {
			fmt.Printf("Error deleting namespace %s from ipset list %s\n", nsName, labelKey)
			return err
		}
		labelKeys = append(labelKeys, labelKey)
	}

	// Delete ipset for the namespace.
	if err := ipsMgr.DeleteSet(nsName); err != nil {
		fmt.Printf("Error deleting ipset for namespace %s.\n", nsName)
		return err
	}

	ns.setMap = make(map[string]string)

	delete(npMgr.nsMap, nsName)

	return nil
}
