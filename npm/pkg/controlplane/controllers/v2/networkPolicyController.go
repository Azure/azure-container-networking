// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package controllers

import (
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/pkg/controlplane/translation"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane"
	"github.com/Azure/azure-container-networking/npm/util"
	networkingv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	networkinginformers "k8s.io/client-go/informers/networking/v1"
	netpollister "k8s.io/client-go/listers/networking/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
)

var errNetPolKeyFormat = errors.New("invalid network policy key format")

type NetworkPolicyController struct {
	netPolLister netpollister.NetworkPolicyLister
	workqueue    workqueue.RateLimitingInterface
	rawNpMap     map[string]*networkingv1.NetworkPolicy // Key is <nsname>/<policyname>
	dp           dataplane.GenericDataplane
}

func NewNetworkPolicyController(npInformer networkinginformers.NetworkPolicyInformer, dp dataplane.GenericDataplane) *NetworkPolicyController {
	netPolController := &NetworkPolicyController{
		netPolLister: npInformer.Lister(),
		workqueue:    workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "NetworkPolicy"),
		rawNpMap:     make(map[string]*networkingv1.NetworkPolicy),
		dp:           dp,
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

func (c *NetworkPolicyController) LengthOfRawNpMap() int {
	return len(c.rawNpMap)
}

// getNetworkPolicyKey returns namespace/name of network policy object if it is valid network policy object and has valid namespace/name.
// If not, it returns error.
func (c *NetworkPolicyController) getNetworkPolicyKey(obj interface{}) (string, error) {
	var key string
	_, ok := obj.(*networkingv1.NetworkPolicy)
	if !ok {
		return key, fmt.Errorf("cannot cast obj (%v) to network policy obj err: %w", obj, errNetPolKeyFormat)
	}

	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		return key, fmt.Errorf("error due to %w", err)
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
			return
		}

		if netPolObj, ok = tombstone.Obj.(*networkingv1.NetworkPolicy); !ok {
			metrics.SendErrorLogAndMetric(util.NetpolID, "[NETPOL DELETE EVENT] Received unexpected object type (error decoding object tombstone, invalid type): %v", obj)
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
			utilruntime.HandleError(fmt.Errorf("expected string in workqueue but got %#v, err %w", obj, errWorkqueueFormatting))
			return nil
		}
		// Run the syncNetPol, passing it the namespace/name string of the
		// network policy resource to be synced.
		if err := c.syncNetPol(key); err != nil {
			// Put the item back on the workqueue to handle any transient errors.
			c.workqueue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s': %w, requeuing", key, err)
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
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s err: %w", key, errNetPolKeyFormat))
		return nil //nolint HandleError  is used instead of returning error to caller
	}

	// Get the network policy resource with this namespace/name
	netPolObj, err := c.netPolLister.NetworkPolicies(namespace).Get(name)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			klog.Infof("Network Policy %s is not found, may be it is deleted", key)
			// netPolObj is not found, but should need to check the RawNpMap cache with key.
			// cleanUpNetworkPolicy method will take care of the deletion of a cached network policy if the cached network policy exists with key in our RawNpMap cache.
			err = c.cleanUpNetworkPolicy(key)
			if err != nil {
				return fmt.Errorf("[syncNetPol] error: %w when network policy is not found", err)
			}
			return err
		}
		return err
	}

	// If DeletionTimestamp of the netPolObj is set, start cleaning up lastly applied states.
	// This is early cleaning up process from updateNetPol event
	if netPolObj.ObjectMeta.DeletionTimestamp != nil || netPolObj.ObjectMeta.DeletionGracePeriodSeconds != nil {
		err = c.cleanUpNetworkPolicy(key)
		if err != nil {
			return fmt.Errorf("error: %w when ObjectMeta.DeletionTimestamp field is set", err)
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
		return fmt.Errorf("[syncNetPol] error due to  %w", err)
	}

	return nil
}

// syncAddAndUpdateNetPol handles a new network policy or an updated network policy object triggered by add and update events
func (c *NetworkPolicyController) syncAddAndUpdateNetPol(netPolObj *networkingv1.NetworkPolicy) error {
	prometheusTimer := metrics.StartNewTimer()
	defer metrics.RecordPolicyExecTime(prometheusTimer) // record execution time regardless of failure

	var err error
	netpolKey, err := cache.MetaNamespaceKeyFunc(netPolObj)
	if err != nil {
		return fmt.Errorf("[syncAddAndUpdateNetPol] Error: while running MetaNamespaceKeyFunc err: %w", err)
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
	err = c.cleanUpNetworkPolicy(netpolKey)
	if err != nil {
		return fmt.Errorf("[syncAddAndUpdateNetPol] Error: failed to deleteNetworkPolicy due to %w", err)
	}

	// install translated rules into kernel
	npmNetPolObj := translation.TranslatePolicy(netPolObj)

	fmt.Printf("%+v", npmNetPolObj)

	// install translated rules into Dataplane
	err = c.dp.AddPolicy(npmNetPolObj)
	if err != nil {
		return fmt.Errorf("[syncAddAndUpdateNetPol] Error: failed to install translated NPMNetworkPolicy into Dataplane due to %w", err)
	}

	// Cache network object first before applying ipsets and iptables.
	// If error happens while applying ipsets and iptables,
	// the key is re-queued in workqueue and process this function again, which eventually meets desired states of network policy
	c.rawNpMap[netpolKey] = netPolObj
	metrics.IncNumPolicies()

	// TODO

	return nil
}

// DeleteNetworkPolicy handles deleting network policy based on netPolKey.
func (c *NetworkPolicyController) cleanUpNetworkPolicy(netPolKey string) error {
	_, cachedNetPolObjExists := c.rawNpMap[netPolKey]
	// if there is no applied network policy with the netPolKey, do not need to clean up process.
	if !cachedNetPolObjExists {
		return nil
	}

	err := c.dp.RemovePolicy(netPolKey)
	if err != nil {
		return fmt.Errorf("[cleanUpNetworkPolicy] Error: failed to remove policy due to %w", err)
	}

	// Success to clean up ipset and iptables operations in kernel and delete the cached network policy from RawNpMap
	delete(c.rawNpMap, netPolKey)
	metrics.DecNumPolicies()
	return nil
}

// compare all fields including name of two network policies, which network policy controller need to care about.
func isSameNetworkPolicy(old, newnetpol *networkingv1.NetworkPolicy) bool {
	if old.ObjectMeta.Name != newnetpol.ObjectMeta.Name {
		return false
	}
	return isSamePolicy(old, newnetpol)
}

func isSamePolicy(old, newnetpol *networkingv1.NetworkPolicy) bool {
	if !reflect.DeepEqual(old.TypeMeta, newnetpol.TypeMeta) {
		return false
	}

	if old.ObjectMeta.Namespace != newnetpol.ObjectMeta.Namespace {
		return false
	}

	if !reflect.DeepEqual(old.Spec, newnetpol.Spec) {
		return false
	}

	return true
}
