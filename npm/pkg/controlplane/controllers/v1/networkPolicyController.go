// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package controllers

import (
	"fmt"
	"strconv"
	"time"

	"github.com/Azure/azure-container-networking/npm/ipsm"
	"github.com/Azure/azure-container-networking/npm/iptm"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/util"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	networkinginformers "k8s.io/client-go/informers/networking/v1"
	netpollister "k8s.io/client-go/listers/networking/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
	"k8s.io/utils/exec"
)

// IsSafeCleanUpAzureNpmChain is used to indicate whether default Azure NPM chain can be safely deleted or not.
type IsSafeCleanUpAzureNpmChain bool

const (
	safeToCleanUpAzureNpmChain   IsSafeCleanUpAzureNpmChain = true
	unSafeToCleanUpAzureNpmChain IsSafeCleanUpAzureNpmChain = false
)

type NetworkPolicyController struct {
	netPolLister netpollister.NetworkPolicyLister
	workqueue    workqueue.RateLimitingInterface
	rawNpMap     map[string]*networkingv1.NetworkPolicy // Key is <nsname>/<policyname>
	// (TODO): will leverage this strucute to manage network policy more efficiently
	// ProcessedNpMap map[string]*networkingv1.NetworkPolicy // Key is <nsname>/<podSelectorHash>
	// flag to indicate default Azure NPM chain is created or not
	isAzureNpmChainCreated bool
	ipsMgr                 *ipsm.IpsetManager
	iptMgr                 *iptm.IptablesManager
}

func NewNetworkPolicyController(npInformer networkinginformers.NetworkPolicyInformer, ipsMgr *ipsm.IpsetManager, placeAzureChainFirst bool) *NetworkPolicyController {
	netPolController := &NetworkPolicyController{
		netPolLister: npInformer.Lister(),
		workqueue:    workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "NetworkPolicy"),
		rawNpMap:     make(map[string]*networkingv1.NetworkPolicy),
		// ProcessedNpMap:         make(map[string]*networkingv1.NetworkPolicy),
		isAzureNpmChainCreated: false,
		ipsMgr:                 ipsMgr,
		iptMgr:                 iptm.NewIptablesManager(exec.New(), iptm.NewIptOperationShim(), placeAzureChainFirst),
	}

	npInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    netPolController.addNetworkPolicy,
			UpdateFunc: netPolController.updateNetworkPolicy,
			DeleteFunc: netPolController.deleteNetworkPolicy,
		},
	)
	return netPolController
}

// initializeDataPlane do all initialization tasks for data plane
// TODO(jungukcho) Need to refactor UninitNpmChains since it assumes it has already AZURE-NPM chains
func (c *NetworkPolicyController) ResetDataPlane() error {
	klog.Infof("Initiailize data plane. Clean up Azure-NPM chains and start reconcile iptables")

	// TODO(jungukcho): will clean-up error handling codes to initialize iptables and ipset in a separate PR
	// It is important to keep order to clean-up iptables and ipset.
	// IPtables should be cleaned first to avoid failures to clean-up iptables due to "ipset is using in kernel" error
	// 1. clean-up NPM-related iptables information and then running periodic processes to keep iptables correct
	if err := c.iptMgr.UninitNpmChains(); err != nil {
		utilruntime.HandleError(fmt.Errorf("Failed to UninitNpmChains with err: %w", err))
	}

	// 2. then clean-up all NPM ipsets states
	if err := c.ipsMgr.DestroyNpmIpsets(); err != nil {
		utilruntime.HandleError(fmt.Errorf("Failed to DestroyNpmIpsets with err: %w", err))
	}

	return nil
}

func (c *NetworkPolicyController) RunPeriodicTasks(stopCh <-chan struct{}) {
	// (TODO): Check any side effects
	c.iptMgr.ReconcileIPTables(stopCh)
}

func (c *NetworkPolicyController) LengthOfRawNpMap() int {
	return len(c.rawNpMap)
}

// getNetworkPolicyKey returns namespace/name of network policy object if it is valid network policy object and has valid namespace/name.
// If not, it returns error.
func (c *NetworkPolicyController) getNetworkPolicyKey(obj interface{}) (string, error) {
	var key string
	_, ok := obj.(*networkingv1.NetworkPolicy)
	if !ok {
		return key, fmt.Errorf("cannot cast obj (%v) to network policy obj", obj)
	}

	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		return key, fmt.Errorf("error due to %s", err)
	}

	return key, nil
}

func (c *NetworkPolicyController) addNetworkPolicy(obj interface{}) {
	netPolkey, err := c.getNetworkPolicyKey(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}

	c.workqueue.Add(netPolkey)
}

func (c *NetworkPolicyController) updateNetworkPolicy(old, newnetpol interface{}) {
	netPolkey, err := c.getNetworkPolicyKey(newnetpol)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}

	// new network policy object is already checked validation by calling getNetworkPolicyKey function.
	newNetPol, _ := newnetpol.(*networkingv1.NetworkPolicy)
	oldNetPol, ok := old.(*networkingv1.NetworkPolicy)
	if ok {
		if oldNetPol.ResourceVersion == newNetPol.ResourceVersion {
			// Periodic resync will send update events for all known network plicies.
			// Two different versions of the same network policy will always have different RVs.
			return
		}
	}

	c.workqueue.Add(netPolkey)
}

func (c *NetworkPolicyController) deleteNetworkPolicy(obj interface{}) {
	netPolObj, ok := obj.(*networkingv1.NetworkPolicy)
	// DeleteFunc gets the final state of the resource (if it is known).
	// Otherwise, it gets an object of type DeletedFinalStateUnknown.
	// This can happen if the watch is closed and misses the delete event and
	// the controller doesn't notice the deletion until the subsequent re-list
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			metrics.SendErrorLogAndMetric(util.NetpolID, "[NETPOL DELETE EVENT] Received unexpected object type: %v", obj)
			utilruntime.HandleError(fmt.Errorf("error decoding object, invalid type"))
			return
		}

		if netPolObj, ok = tombstone.Obj.(*networkingv1.NetworkPolicy); !ok {
			metrics.SendErrorLogAndMetric(util.NetpolID, "[NETPOL DELETE EVENT] Received unexpected object type (error decoding object tombstone, invalid type): %v", obj)
			utilruntime.HandleError(fmt.Errorf("error decoding object tombstone, invalid type"))
			return
		}
	}

	var netPolkey string
	var err error
	if netPolkey, err = cache.MetaNamespaceKeyFunc(netPolObj); err != nil {
		utilruntime.HandleError(err)
		return
	}

	c.workqueue.Add(netPolkey)
}

func (c *NetworkPolicyController) Run(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()

	klog.Infof("Starting Network Policy worker")
	go wait.Until(c.runWorker, time.Second, stopCh)

	klog.Infof("Started Network Policy worker")
	<-stopCh
	klog.Info("Shutting down Network Policy workers")
}

func (c *NetworkPolicyController) runWorker() {
	for c.processNextWorkItem() {
	}
}

func (c *NetworkPolicyController) processNextWorkItem() bool {
	obj, shutdown := c.workqueue.Get()

	if shutdown {
		return false
	}

	err := func(obj interface{}) error {
		defer c.workqueue.Done(obj)
		var key string
		var ok bool
		if key, ok = obj.(string); !ok {
			// As the item in the workqueue is actually invalid, we call
			// Forget here else we'd go into a loop of attempting to
			// process a work item that is invalid.
			c.workqueue.Forget(obj)
			utilruntime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}
		// Run the syncNetPol, passing it the namespace/name string of the
		// network policy resource to be synced.
		if err := c.syncNetPol(key); err != nil {
			// Put the item back on the workqueue to handle any transient errors.
			c.workqueue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.workqueue.Forget(obj)
		klog.Infof("Successfully synced '%s'", key)
		return nil
	}(obj)
	if err != nil {
		utilruntime.HandleError(err)
		metrics.SendErrorLogAndMetric(util.NetpolID, "syncNetPol error due to %v", err)
		return true
	}

	return true
}

// syncNetPol compares the actual state with the desired, and attempts to converge the two.
func (c *NetworkPolicyController) syncNetPol(key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	// Get the network policy resource with this namespace/name
	netPolObj, err := c.netPolLister.NetworkPolicies(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			klog.Infof("Network Policy %s is not found, may be it is deleted", key)
			// netPolObj is not found, but should need to check the RawNpMap cache with key.
			// cleanUpNetworkPolicy method will take care of the deletion of a cached network policy if the cached network policy exists with key in our RawNpMap cache.
			err = c.cleanUpNetworkPolicy(key, safeToCleanUpAzureNpmChain)
			if err != nil {
				return fmt.Errorf("[syncNetPol] Error: %v when network policy is not found\n", err)
			}
			return err
		}
		return err
	}

	// If DeletionTimestamp of the netPolObj is set, start cleaning up lastly applied states.
	// This is early cleaning up process from updateNetPol event
	if netPolObj.ObjectMeta.DeletionTimestamp != nil || netPolObj.ObjectMeta.DeletionGracePeriodSeconds != nil {
		err = c.cleanUpNetworkPolicy(key, safeToCleanUpAzureNpmChain)
		if err != nil {
			return fmt.Errorf("Error: %v when ObjectMeta.DeletionTimestamp field is set\n", err)
		}
		return nil
	}

	cachedNetPolObj, netPolExists := c.rawNpMap[key]
	if netPolExists {
		// if network policy does not have different states against lastly applied states stored in cachedNetPolObj,
		// netPolController does not need to reconcile this update.
		// In this updateNetworkPolicy event,
		// newNetPol was updated with states which netPolController does not need to reconcile.
		if isSameNetworkPolicy(cachedNetPolObj, netPolObj) {
			return nil
		}
	}

	err = c.syncAddAndUpdateNetPol(netPolObj)
	if err != nil {
		return fmt.Errorf("[syncNetPol] Error due to  %s\n", err.Error())
	}

	return nil
}

// initializeDefaultAzureNpmChain install default rules for kube-system and iptables
func (c *NetworkPolicyController) initializeDefaultAzureNpmChain() error {
	if c.isAzureNpmChainCreated {
		return nil
	}

	if err := c.ipsMgr.CreateSet(util.KubeSystemFlag, []string{util.IpsetNetHashFlag}); err != nil {
		return fmt.Errorf("[initializeDefaultAzureNpmChain] Error: failed to initialize kube-system ipset with err %s", err)
	}
	if err := c.iptMgr.InitNpmChains(); err != nil {
		return fmt.Errorf("[initializeDefaultAzureNpmChain] Error: failed to initialize azure-npm chains with err %s", err)
	}

	c.isAzureNpmChainCreated = true
	return nil
}

// syncAddAndUpdateNetPol handles a new network policy or an updated network policy object triggered by add and update events
func (c *NetworkPolicyController) syncAddAndUpdateNetPol(netPolObj *networkingv1.NetworkPolicy) error {
	prometheusTimer := metrics.StartNewTimer()
	defer metrics.RecordPolicyExecTime(prometheusTimer) // record execution time regardless of failure

	var err error
	netpolKey, err := cache.MetaNamespaceKeyFunc(netPolObj)
	if err != nil {
		return fmt.Errorf("[syncAddAndUpdateNetPol] Error: while running MetaNamespaceKeyFunc err: %s", err)
	}

	// Start reconciling loop to eventually meet cached states against the desired states from network policy.
	// #1 If a new network policy is created, the network policy is not in RawNPMap.
	// start translating policy and install translated ipset and iptables rules into kernel
	// #2 If a network policy with <ns>-<netpol namespace>-<netpol name> is applied before and two network policy are the same object (same UID),
	// first delete the applied network policy, then start translating policy and install translated ipset and iptables rules into kernel
	// #3 If a network policy with <ns>-<netpol namespace>-<netpol name> is applied before and two network policy are the different object (different UID) due to missing some events for the old object
	// first delete the applied network policy, then start translating policy and install translated ipset and iptables rules into kernel
	// To deal with all three cases, we first delete network policy if possible, then install translated rules into kernel.
	// (TODO): can optimize logic more to reduce computations. For example, apply only difference if possible like podController

	// Do not need to clean up default Azure NPM chain in deleteNetworkPolicy function, if network policy object is applied soon.
	// So, avoid extra overhead to install default Azure NPM chain in initializeDefaultAzureNpmChain function.
	// To achieve it, use flag unSafeToCleanUpAzureNpmChain to indicate that the default Azure NPM chain cannot be deleted.
	// delete existing network policy
	err = c.cleanUpNetworkPolicy(netpolKey, unSafeToCleanUpAzureNpmChain)
	if err != nil {
		return fmt.Errorf("[syncAddAndUpdateNetPol] Error: failed to deleteNetworkPolicy due to %s", err)
	}

	// Install this default rules for kube-system and azure-npm chains if they are not initilized.
	// Execute initializeDefaultAzureNpmChain function first before actually starting processing network policy object.
	if err = c.initializeDefaultAzureNpmChain(); err != nil {
		return fmt.Errorf("[syncNetPol] Error: due to %v", err)
	}

	// Cache network object first before applying ipsets and iptables.
	// If error happens while applying ipsets and iptables,
	// the key is re-queued in workqueue and process this function again, which eventually meets desired states of network policy
	c.rawNpMap[netpolKey] = netPolObj
	metrics.IncNumPolicies()

	sets, namedPorts, lists, ingressIPCidrs, egressIPCidrs, iptEntries := translatePolicy(netPolObj)
	for _, set := range sets {
		klog.Infof("Creating set: %v, hashedSet: %v", set, util.GetHashedName(set))
		if err = c.ipsMgr.CreateSet(set, []string{util.IpsetNetHashFlag}); err != nil {
			return fmt.Errorf("[syncAddAndUpdateNetPol] Error: creating ipset %s with err: %v", set, err)
		}
	}
	for _, set := range namedPorts {
		klog.Infof("Creating set: %v, hashedSet: %v", set, util.GetHashedName(set))
		if err = c.ipsMgr.CreateSet(set, []string{util.IpsetIPPortHashFlag}); err != nil {
			return fmt.Errorf("[syncAddAndUpdateNetPol] Error: creating ipset named port %s with err: %v", set, err)
		}
	}

	// lists is a map with list name and members as value
	// NPM will create the list first and increments the refer count
	for listKey := range lists {
		if err = c.ipsMgr.CreateList(listKey); err != nil {
			return fmt.Errorf("[syncAddAndUpdateNetPol] Error: creating ipset list %s with err: %v", listKey, err)
		}
		c.ipsMgr.IpSetReferIncOrDec(listKey, util.IpsetSetListFlag, ipsm.IncrementOp)
	}
	// Then NPM will add members to the above list, this is to avoid members being added
	// to lists before they are created.
	for listKey, listLabelsMembers := range lists {
		for _, listMember := range listLabelsMembers {
			if err = c.ipsMgr.AddToList(listKey, listMember); err != nil {
				return fmt.Errorf("[syncAddAndUpdateNetPol] Error: Adding ipset member %s to ipset list %s with err: %v", listMember, listKey, err)
			}
		}
		c.ipsMgr.IpSetReferIncOrDec(listKey, util.IpsetSetListFlag, ipsm.IncrementOp)
	}

	if err = c.createCidrsRule("in", netPolObj.Name, netPolObj.Namespace, ingressIPCidrs); err != nil {
		return fmt.Errorf("[syncAddAndUpdateNetPol] Error: createCidrsRule in due to %v", err)
	}

	if err = c.createCidrsRule("out", netPolObj.Name, netPolObj.Namespace, egressIPCidrs); err != nil {
		return fmt.Errorf("[syncAddAndUpdateNetPol] Error: createCidrsRule out due to %v", err)
	}

	for _, iptEntry := range iptEntries {
		if err = c.iptMgr.Add(iptEntry); err != nil {
			return fmt.Errorf("[syncAddAndUpdateNetPol] Error: failed to apply iptables rule. Rule: %+v with err: %v", iptEntry, err)
		}
	}

	return nil
}

// DeleteNetworkPolicy handles deleting network policy based on netPolKey.
func (c *NetworkPolicyController) cleanUpNetworkPolicy(netPolKey string, isSafeCleanUpAzureNpmChain IsSafeCleanUpAzureNpmChain) error {
	cachedNetPolObj, cachedNetPolObjExists := c.rawNpMap[netPolKey]
	// if there is no applied network policy with the netPolKey, do not need to clean up process.
	if !cachedNetPolObjExists {
		return nil
	}

	// translate policy from "cachedNetPolObj"
	_, _, lists, ingressIPCidrs, egressIPCidrs, iptEntries := translatePolicy(cachedNetPolObj)

	var err error
	// delete iptables entries
	for _, iptEntry := range iptEntries {
		if err = c.iptMgr.Delete(iptEntry); err != nil {
			return fmt.Errorf("[cleanUpNetworkPolicy] Error: failed to apply iptables rule. Rule: %+v with err: %v", iptEntry, err)
		}
	}

	// lists is a map with list name and members as value
	for listKey := range lists {
		// We do not have delete the members before deleting set as,
		// 1. ipset allows deleting a ipset list with members
		// 2. if the refer count is more than one we should not remove members
		// 3. for reduced datapath operations
		if err = c.ipsMgr.DeleteList(listKey); err != nil {
			return fmt.Errorf("[cleanUpNetworkPolicy] Error: failed to delete ipset list %s with err: %w", listKey, err)
		}
	}

	// delete ipset list related to ingress CIDRs
	if err = c.removeCidrsRule("in", cachedNetPolObj.Name, cachedNetPolObj.Namespace, ingressIPCidrs); err != nil {
		return fmt.Errorf("[cleanUpNetworkPolicy] Error: removeCidrsRule in due to %v", err)
	}

	// delete ipset list related to egress CIDRs
	if err = c.removeCidrsRule("out", cachedNetPolObj.Name, cachedNetPolObj.Namespace, egressIPCidrs); err != nil {
		return fmt.Errorf("[cleanUpNetworkPolicy] Error: removeCidrsRule out due to %v", err)
	}

	// Sucess to clean up ipset and iptables operations in kernel and delete the cached network policy from RawNpMap
	delete(c.rawNpMap, netPolKey)
	metrics.DecNumPolicies()

	// If there is no cached network policy in RawNPMap anymore and no immediate network policy to process, start cleaning up default azure npm chains
	// However, UninitNpmChains function is failed which left failed states and will not retry, but functionally it is ok.
	// (TODO): Ideally, need to decouple cleaning-up default azure npm chains from "network policy deletion" event.
	if isSafeCleanUpAzureNpmChain && len(c.rawNpMap) == 0 {
		// Even though UninitNpmChains function returns error, isAzureNpmChainCreated sets up false.
		// So, when a new network policy is added, the "default Azure NPM chain" can be installed.
		c.isAzureNpmChainCreated = false
		if err = c.iptMgr.UninitNpmChains(); err != nil {
			utilruntime.HandleError(fmt.Errorf("Error: failed to uninitialize azure-npm chains with err: %s", err))
			return nil
		}
	}

	return nil
}

// (TODO) do not need to ipsMgr parameter
func (c *NetworkPolicyController) createCidrsRule(direction, policyName, ns string, ipsets [][]string) error {
	spec := []string{util.IpsetNetHashFlag, util.IpsetMaxelemName, util.IpsetMaxelemNum}

	for i, ipCidrSet := range ipsets {
		if len(ipCidrSet) == 0 {
			continue
		}
		setName := policyName + "-in-ns-" + ns + "-" + strconv.Itoa(i) + direction
		klog.Infof("Creating set: %v, hashedSet: %v", setName, util.GetHashedName(setName))
		if err := c.ipsMgr.CreateSet(setName, spec); err != nil {
			return fmt.Errorf("[createCidrsRule] Error: creating ipset %s with err: %v", ipCidrSet, err)
		}
		for _, ipCidrEntry := range util.DropEmptyFields(ipCidrSet) {
			// Ipset doesn't allow 0.0.0.0/0 to be added.
			// A solution is split 0.0.0.0/0 in half which convert to 0.0.0.0/1 and 128.0.0.0/1.
			if ipCidrEntry == "0.0.0.0/0" {
				splitEntry := [2]string{"0.0.0.0/1", "128.0.0.0/1"}
				for _, entry := range splitEntry {
					if err := c.ipsMgr.AddToSet(setName, entry, util.IpsetNetHashFlag, ""); err != nil {
						return fmt.Errorf("[createCidrsRule] adding ip cidrs %s into ipset %s with err: %v", entry, ipCidrSet, err)
					}
				}
			} else {
				if err := c.ipsMgr.AddToSet(setName, ipCidrEntry, util.IpsetNetHashFlag, ""); err != nil {
					return fmt.Errorf("[createCidrsRule] adding ip cidrs %s into ipset %s with err: %v", ipCidrEntry, ipCidrSet, err)
				}
			}
		}
	}

	return nil
}

func (c *NetworkPolicyController) removeCidrsRule(direction, policyName, ns string, ipsets [][]string) error {
	for i, ipCidrSet := range ipsets {
		if len(ipCidrSet) == 0 {
			continue
		}
		setName := policyName + "-in-ns-" + ns + "-" + strconv.Itoa(i) + direction
		klog.Infof("Delete set: %v, hashedSet: %v", setName, util.GetHashedName(setName))
		if err := c.ipsMgr.DeleteSet(setName); err != nil {
			return fmt.Errorf("[removeCidrsRule] deleting ipset %s with err: %v", ipCidrSet, err)
		}
	}

	return nil
}

// GetProcessedNPKey will return netpolKey
// (TODO): will use this function when optimizing management of multiple network policies with merging and deducting multiple network policies.
// func (c *networkPolicyController) getProcessedNPKey(netPolObj *networkingv1.NetworkPolicy) string {
// 	// hashSelector will never be empty
// 	// (TODO): what if PodSelector is [] or nothing? - make the Unit test for this
// 	hashedPodSelector := HashSelector(&netPolObj.Spec.PodSelector)

// 	// (TODO): any chance to have namespace has zero length?
// 	if len(netPolObj.GetNamespace()) > 0 {
// 		hashedPodSelector = netPolObj.GetNamespace() + "/" + hashedPodSelector
// 	}
// 	return util.GetNSNameWithPrefix(hashedPodSelector)
// }
