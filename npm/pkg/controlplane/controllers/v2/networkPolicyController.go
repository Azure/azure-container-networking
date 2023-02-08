// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package controllers

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
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

var (
	errNetPolKeyFormat          = errors.New("invalid network policy key format")
	errNetPolTranslationFailure = errors.New("failed to translate network policy")
)

type NetworkPolicyController struct {
	sync.RWMutex
	netPolLister netpollister.NetworkPolicyLister
	workqueue    workqueue.RateLimitingInterface
	rawNpSpecMap map[string]*networkingv1.NetworkPolicySpec // Key is <nsname>/<policyname>
	dp           dataplane.GenericDataplane
}

func (c *NetworkPolicyController) GetCache() map[string]*networkingv1.NetworkPolicySpec {
	c.RLock()
	defer c.RUnlock()
	return c.rawNpSpecMap
}

func NewNetworkPolicyController(npInformer networkinginformers.NetworkPolicyInformer, dp dataplane.GenericDataplane) *NetworkPolicyController {
	netPolController := &NetworkPolicyController{
		netPolLister: npInformer.Lister(),
		workqueue:    workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "NetworkPolicy"),
		rawNpSpecMap: make(map[string]*networkingv1.NetworkPolicySpec),
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
	return len(c.rawNpSpecMap)
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
	// timer for recording execution times
	timer := metrics.StartNewTimer()

	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s err: %w", key, errNetPolKeyFormat))
		return nil //nolint HandleError  is used instead of returning error to caller
	}

	// record exec time after syncing
	operationKind := metrics.NoOp
	defer func() {
		metrics.RecordControllerPolicyExecTime(timer, operationKind, err != nil)
	}()

	// Get the network policy resource with this namespace/name
	netPolObj, err := c.netPolLister.NetworkPolicies(namespace).Get(name)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			klog.Infof("Network Policy %s is not found, may be it is deleted", key)

			if _, ok := c.rawNpSpecMap[key]; ok {
				// record time to delete policy if it exists (can't call within cleanUpNetworkPolicy because this can be called by a pod update)
				operationKind = metrics.DeleteOp
			}

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
		if _, ok := c.rawNpSpecMap[key]; ok {
			// record time to delete policy if it exists (can't call within cleanUpNetworkPolicy because this can be called by a pod update)
			operationKind = metrics.DeleteOp
		}
		err = c.cleanUpNetworkPolicy(key)
		if err != nil {
			return fmt.Errorf("error: %w when ObjectMeta.DeletionTimestamp field is set", err)
		}
		return nil
	}

	cachedNetPolSpecObj, netPolExists := c.rawNpSpecMap[key]
	if netPolExists {
		// if network policy does not have different states against lastly applied states stored in cachedNetPolObj,
		// netPolController does not need to reconcile this update.
		// In this updateNetworkPolicy event,
		// newNetPol was updated with states which netPolController does not need to reconcile.
		if reflect.DeepEqual(cachedNetPolSpecObj, &netPolObj.Spec) {
			return nil
		}
	}

	operationKind, err = c.syncAddAndUpdateNetPol(netPolObj)
	if err != nil {
		return fmt.Errorf("[syncNetPol] error due to  %w", err)
	}

	return nil
}

// syncAddAndUpdateNetPol handles a new network policy or an updated network policy object triggered by add and update events
func (c *NetworkPolicyController) syncAddAndUpdateNetPol(netPolObj *networkingv1.NetworkPolicy) (metrics.OperationKind, error) {
	var err error
	netpolKey, err := cache.MetaNamespaceKeyFunc(netPolObj)
	if err != nil {
		// consider a no-op since we can't determine add vs. update. The exec time here isn't important either.
		return metrics.NoOp, fmt.Errorf("[syncAddAndUpdateNetPol] Error: while running MetaNamespaceKeyFunc err: %w", err)
	}

	// install translated rules into kernel
	npmNetPolObj, err := translation.TranslatePolicy(netPolObj)
	if err != nil {
		if isUnsupportedWindowsTranslationErr(err) {
			klog.Warningf("NetworkPolicy %s in namespace %s is not translated because it has unsupported translated features of Windows: %s",
				netPolObj.ObjectMeta.Name, netPolObj.ObjectMeta.Namespace, err.Error())

			// We can safely suppress unsupported network policy because re-Queuing will result in same error.
			// The exec time isn't relevant here, so consider a no-op.
			return metrics.NoOp, nil
		}

		klog.Errorf("Failed to translate podSelector in NetworkPolicy %s in namespace %s: %s", netPolObj.ObjectMeta.Name, netPolObj.ObjectMeta.Namespace, err.Error())
		// The exec time isn't relevant here, so consider a no-op. Returning nil to prevent re-queuing since this is not a transient error.
		return metrics.NoOp, nil
	}

	_, policyExisted := c.rawNpSpecMap[netpolKey]
	var operationKind metrics.OperationKind
	if policyExisted {
		operationKind = metrics.UpdateOp
	} else {
		operationKind = metrics.CreateOp
	}

	// install translated rules into Dataplane
	// DP update policy call will check if this policy already exists in kernel
	// if yes: then will delete old rules and program new rules
	// if no: then will program add new rules
	err = c.dp.UpdatePolicy(npmNetPolObj)
	if err != nil {
		// if error occurred the key is re-queued in workqueue and process this function again,
		// which eventually meets desired states of network policy
		return operationKind, fmt.Errorf("[syncAddAndUpdateNetPol] Error: failed to update translated NPMNetworkPolicy into Dataplane due to %w", err)
	}

	if !policyExisted {
		// inc metric for NumPolicies only if it a new network policy
		metrics.IncNumPolicies()
	}

	c.rawNpSpecMap[netpolKey] = &netPolObj.Spec
	return operationKind, nil
}

// DeleteNetworkPolicy handles deleting network policy based on netPolKey.
func (c *NetworkPolicyController) cleanUpNetworkPolicy(netPolKey string) error {
	_, cachedNetPolObjExists := c.rawNpSpecMap[netPolKey]
	// if there is no applied network policy with the netPolKey, do not need to clean up process.
	if !cachedNetPolObjExists {
		return nil
	}

	err := c.dp.RemovePolicy(netPolKey)
	if err != nil {
		return fmt.Errorf("[cleanUpNetworkPolicy] Error: failed to remove policy due to %w", err)
	}

	// Success to clean up ipset and iptables operations in kernel and delete the cached network policy from RawNpMap
	delete(c.rawNpSpecMap, netPolKey)
	metrics.DecNumPolicies()
	return nil
}

func isUnsupportedWindowsTranslationErr(err error) bool {
	return errors.Is(err, translation.ErrUnsupportedNamedPort) ||
		errors.Is(err, translation.ErrUnsupportedNegativeMatch) ||
		errors.Is(err, translation.ErrUnsupportedSCTP) ||
		errors.Is(err, translation.ErrUnsupportedExceptCIDR)
}
