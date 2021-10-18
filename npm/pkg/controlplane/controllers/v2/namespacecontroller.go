// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/util"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	coreinformer "k8s.io/client-go/informers/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
)

type LabelAppendOperation bool

const (
	clearExistingLabels    LabelAppendOperation = true
	appendToExistingLabels LabelAppendOperation = false
)

var errWorkqueueFormatting = errors.New("error in formatting")

// NpmNamespaceCache to store namespace struct in nameSpaceController.go.
// Since this cache is shared between podController and NamespaceController,
// it has mutex for avoiding racing condition between them.
type NpmNamespaceCache struct {
	sync.Mutex
	NsMap map[string]*Namespace // Key is ns-<nsname>
}

func (n *NpmNamespaceCache) MarshalJSON() ([]byte, error) {
	n.Lock()
	defer n.Unlock()

	nsMapRaw, err := json.Marshal(n.NsMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal nsMap due to %w", err)
	}

	return nsMapRaw, nil
}

type Namespace struct {
	name      string
	LabelsMap map[string]string // Namespace labels
}

// newNS constructs a new namespace object.
func newNs(name string) *Namespace {
	ns := &Namespace{
		name:      name,
		LabelsMap: make(map[string]string),
	}
	return ns
}

func (nsObj *Namespace) appendLabels(newm map[string]string, clear LabelAppendOperation) {
	if clear {
		nsObj.LabelsMap = make(map[string]string)
	}
	for k, v := range newm {
		nsObj.LabelsMap[k] = v
	}
}

func (nsObj *Namespace) removeLabelsWithKey(key string) {
	delete(nsObj.LabelsMap, key)
}

type NamespaceController struct {
	dp                dataplane.GenericDataplane
	nameSpaceLister   corelisters.NamespaceLister
	workqueue         workqueue.RateLimitingInterface
	npmNamespaceCache *NpmNamespaceCache
}

func NewNamespaceController(nameSpaceInformer coreinformer.NamespaceInformer, dp dataplane.GenericDataplane, npmNamespaceCache *NpmNamespaceCache) *NamespaceController {
	nameSpaceController := &NamespaceController{
		dp:                dp,
		nameSpaceLister:   nameSpaceInformer.Lister(),
		workqueue:         workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Namespaces"),
		npmNamespaceCache: npmNamespaceCache,
	}

	nameSpaceInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    nameSpaceController.addNamespace,
			UpdateFunc: nameSpaceController.updateNamespace,
			DeleteFunc: nameSpaceController.deleteNamespace,
		},
	)
	return nameSpaceController
}

// filter this event if we do not need to handle this event
func (nsc *NamespaceController) needSync(obj interface{}, event string) (string, bool) {
	needSync := false
	var key string

	nsObj, ok := obj.(*corev1.Namespace)
	if !ok {
		metrics.SendErrorLogAndMetric(util.NSID, "[NAMESPACE %s EVENT] Received unexpected object type: %v", event, obj)
		return key, needSync
	}

	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		metrics.SendErrorLogAndMetric(util.NSID, "[NAMESPACE %s EVENT] Error: NamespaceKey is empty for %s namespace", event, util.GetNSNameWithPrefix(nsObj.Name))
		return key, needSync
	}

	klog.Infof("[NAMESPACE %s EVENT] for namespace [%s]", event, key)

	needSync = true
	return key, needSync
}

func (nsc *NamespaceController) addNamespace(obj interface{}) {
	key, needSync := nsc.needSync(obj, "ADD")
	if !needSync {
		klog.Infof("[NAMESPACE ADD EVENT] No need to sync this namespace [%s]", key)
		return
	}
	nsc.workqueue.Add(key)
}

func (nsc *NamespaceController) updateNamespace(old, newns interface{}) {
	key, needSync := nsc.needSync(newns, "UPDATE")
	if !needSync {
		klog.Infof("[NAMESPACE UPDATE EVENT] No need to sync this namespace [%s]", key)
		return
	}

	nsObj, _ := newns.(*corev1.Namespace)
	oldNsObj, ok := old.(*corev1.Namespace)
	if ok {
		if oldNsObj.ResourceVersion == nsObj.ResourceVersion {
			klog.Infof("[NAMESPACE UPDATE EVENT] Resourceversion is same for this namespace [%s]", key)
			return
		}
	}

	nsc.workqueue.Add(key)
}

func (nsc *NamespaceController) deleteNamespace(obj interface{}) {
	nsObj, ok := obj.(*corev1.Namespace)
	// DeleteFunc gets the final state of the resource (if it is known).
	// Otherwise, it gets an object of type DeletedFinalStateUnknown.
	// This can happen if the watch is closed and misses the delete event and
	// the controller doesn't notice the deletion until the subsequent re-list
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			metrics.SendErrorLogAndMetric(util.NSID, "[NAMESPACE DELETE EVENT]: Received unexpected object type: %v", obj)
			return
		}

		if nsObj, ok = tombstone.Obj.(*corev1.Namespace); !ok {
			metrics.SendErrorLogAndMetric(util.NSID, "[NAMESPACE DELETE EVENT]: Received unexpected object type (error decoding object tombstone, invalid type): %v", obj)
			return
		}
	}

	var err error
	var key string
	if key, err = cache.MetaNamespaceKeyFunc(nsObj); err != nil {
		utilruntime.HandleError(err)
		metrics.SendErrorLogAndMetric(util.NSID, "[NAMESPACE DELETE EVENT] Error: nameSpaceKey is empty for %s namespace", util.GetNSNameWithPrefix(nsObj.Name))
		return
	}

	nsc.workqueue.Add(key)
}

func (nsc *NamespaceController) Run(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer nsc.workqueue.ShutDown()

	klog.Info("Starting Namespace controller\n")
	klog.Info("Starting workers")
	// Launch workers to process namespace resources
	go wait.Until(nsc.runWorker, time.Second, stopCh)

	klog.Info("Started workers")
	<-stopCh
	klog.Info("Shutting down workers")
}

func (nsc *NamespaceController) runWorker() {
	for nsc.processNextWorkItem() {
	}
}

func (nsc *NamespaceController) processNextWorkItem() bool {
	obj, shutdown := nsc.workqueue.Get()

	if shutdown {
		return false
	}

	err := func(obj interface{}) error {
		defer nsc.workqueue.Done(obj)
		var key string
		var ok bool
		if key, ok = obj.(string); !ok {
			// As the item in the workqueue is actually invalid, we call
			// Forget here else we'd go into a loop of attempting to
			// process a work item that is invalid.
			nsc.workqueue.Forget(obj)
			utilruntime.HandleError(fmt.Errorf("expected string in workqueue but got %#v, err %w", obj, errWorkqueueFormatting))
			return nil
		}
		// Run the syncNamespace, passing it the namespace string of the
		// resource to be synced.
		if err := nsc.syncNamespace(key); err != nil {
			// Put the item back on the workqueue to handle any transient errors.
			nsc.workqueue.AddRateLimited(key)
			metrics.SendErrorLogAndMetric(util.NSID, "[processNextWorkItem] Error: failed to syncNamespace %s. Requeuing with err: %v", key, err)
			return err
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		nsc.workqueue.Forget(obj)
		klog.Infof("Successfully synced '%s'", key)
		return nil
	}(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return true
	}

	return true
}

// syncNamespace compares the actual state with the desired, and attempts to converge the two.
func (nsc *NamespaceController) syncNamespace(key string) error {
	// Get the Namespace resource with this key
	nsObj, err := nsc.nameSpaceLister.Get(key)
	cachedNsKey := util.GetNSNameWithPrefix(key)

	// apply dataplane after syncing
	defer func() {
		dperr := nsc.dp.ApplyDataPlane()
		if dperr != nil {
			err = fmt.Errorf("failed with error %w, apply failed with %v", err, dperr)
		}
	}()

	// hold lock to avoid racing condition with PodController
	nsc.npmNamespaceCache.Lock()
	defer nsc.npmNamespaceCache.Unlock()
	if err != nil {
		if k8serrors.IsNotFound(err) {
			klog.Infof("Namespace %s not found, may be it is deleted", key)
			// cleanDeletedNamespace will check if the NS exists in cache, if it does, then proceeds with deletion
			// if it does not exists, then event will be no-op
			err = nsc.cleanDeletedNamespace(cachedNsKey)
			if err != nil {
				// need to retry this cleaning-up process
				metrics.SendErrorLogAndMetric(util.NSID, "Error: %v when namespace is not found", err)
				return fmt.Errorf("Error: %w when namespace is not found", err)
			}
		}
		return err
	}

	if nsObj.DeletionTimestamp != nil || nsObj.DeletionGracePeriodSeconds != nil {
		return nsc.cleanDeletedNamespace(cachedNsKey)
	}

	cachedNsObj, nsExists := nsc.npmNamespaceCache.NsMap[cachedNsKey]
	if nsExists {
		if reflect.DeepEqual(cachedNsObj.LabelsMap, nsObj.ObjectMeta.Labels) {
			klog.Infof("[NAMESPACE UPDATE EVENT] Namespace [%s] labels did not change", key)
			return nil
		}
	}

	err = nsc.syncUpdateNamespace(nsObj)
	if err != nil {
		metrics.SendErrorLogAndMetric(util.NSID, "[syncNamespace] failed to sync namespace due to  %s", err.Error())
		return err
	}

	return nil
}

// syncAddNamespace handles adding namespace to ipset.
func (nsc *NamespaceController) syncAddNamespace(nsObj *corev1.Namespace) error {
	namespaceSets := []*ipsets.IPSetMetadata{ipsets.NewIPSetMetadata(nsObj.ObjectMeta.Name, ipsets.Namespace)}
	setsToAddNamespaceTo := []*ipsets.IPSetMetadata{}

	namespaceSets = append(namespaceSets, &ipsets.IPSetMetadata{Name: nsObj.ObjectMeta.Name, Type: ipsets.Namespace})
	setsToAddNamespaceTo = append(setsToAddNamespaceTo, kubeAllNamespaces)

	npmNs := newNs(nsObj.ObjectMeta.Name)
	nsc.npmNamespaceCache.NsMap[nsObj.ObjectMeta.Name] = npmNs

	// Add the namespace to its label's ipset list.
	for nsLabelKey, nsLabelVal := range nsObj.ObjectMeta.Labels {
		klog.Infof("Adding namespace %s to ipset list %s", nsObj.ObjectMeta.Name, nsLabelKey)
		labelIPSets := []*ipsets.IPSetMetadata{
			{Name: nsLabelKey, Type: ipsets.Namespace},
			{Name: util.GetIpSetFromLabelKV(nsLabelKey, nsLabelVal), Type: ipsets.Namespace},
		}

		setsToAddNamespaceTo = append(setsToAddNamespaceTo, labelIPSets...)

		// Append succeeded labels to the cache NS obj
		npmNs.appendLabels(map[string]string{nsLabelKey: nsLabelVal}, appendToExistingLabels)
	}

	nsc.dp.CreateIPSet(append(namespaceSets, setsToAddNamespaceTo...))

	if err := nsc.dp.AddToLists(setsToAddNamespaceTo, namespaceSets, nil); err != nil {
		return fmt.Errorf("failed to sync add namespace with error %w", err)
	}

	return nil
}

// syncUpdateNamespace handles updating namespace in ipset.
func (nsc *NamespaceController) syncUpdateNamespace(newNsObj *corev1.Namespace) error {
	var err error
	newNsName, newNsLabel := util.GetNSNameWithPrefix(newNsObj.ObjectMeta.Name), newNsObj.ObjectMeta.Labels
	klog.Infof("NAMESPACE UPDATING:\n namespace: [%s/%v]", newNsName, newNsLabel)

	// If previous syncAddNamespace failed for some reasons
	// before caching npm namespace object or syncUpdateNamespace is called due to namespace creation event,
	// then there is no cached object in nsMap.
	curNsObj, exists := nsc.npmNamespaceCache.NsMap[newNsName]
	if !exists {
		if newNsObj.ObjectMeta.DeletionTimestamp == nil && newNsObj.ObjectMeta.DeletionGracePeriodSeconds == nil {
			if er := nsc.syncAddNamespace(newNsObj); er != nil {
				return fmt.Errorf("failed to sync add namespace with err %w", err)
			}
		}

		return nil
	}

	// If the Namespace is not deleted, delete removed labels and create new labels
	addToIPSets, deleteFromIPSets := util.GetIPSetListCompareLabels(curNsObj.LabelsMap, newNsLabel)
	// Delete the namespace from its label's ipset list.
	for _, nsLabelVal := range deleteFromIPSets {
		labelKey := util.GetNSNameWithPrefix(nsLabelVal)

		labelKeySet := &ipsets.IPSetMetadata{Name: nsLabelVal, Type: ipsets.KeyLabelOfNamespace}
		toBeAdded := []*ipsets.IPSetMetadata{ipsets.NewIPSetMetadata(newNsName, ipsets.Namespace)}

		klog.Infof("Deleting namespace %s from ipset list %s", newNsName, labelKey)
		if err = nsc.dp.RemoveFromList(labelKeySet, toBeAdded, nil); err != nil {
			metrics.SendErrorLogAndMetric(util.NSID, "[UpdateNamespace] Error: failed to delete namespace %s from ipset list %s with err: %v", newNsName, labelKey, err)
			return fmt.Errorf("failed to remove from list during sync update namespace with err %w", err)
		}
		// {IMPORTANT} The order of compared list will be key and then key+val. NPM should only append after both key
		// key + val ipsets are worked on.
		// (TODO) need to remove this ordering dependency
		removedLabelKey, removedLabelValue := util.GetLabelKVFromSet(nsLabelVal)
		if removedLabelValue != "" {
			curNsObj.removeLabelsWithKey(removedLabelKey)
		}
	}

	// Add the namespace to its label's ipset list.
	for _, nsLabelVal := range addToIPSets {
		klog.Infof("Adding namespace %s to ipset list %s", newNsName, nsLabelVal)

		labelKeySet := []*ipsets.IPSetMetadata{ipsets.NewIPSetMetadata(nsLabelVal, ipsets.KeyLabelOfNamespace)}
		toBeAdded := []*ipsets.IPSetMetadata{ipsets.NewIPSetMetadata(newNsName, ipsets.Namespace)}

		if err = nsc.dp.AddToLists(labelKeySet, toBeAdded, nil); err != nil {
			metrics.SendErrorLogAndMetric(util.NSID, "[UpdateNamespace] Error: failed to add namespace %s to ipset list %s with err: %v", newNsName, nsLabelVal, err)
			return fmt.Errorf("failed to add %v sets to %v lists during addtolists in sync update namespace with err %w", toBeAdded, labelKeySet, err)
		}
		// {IMPORTANT} Same as above order is assumed to be key and then key+val. NPM should only append to existing labels
		// only after both ipsets for a given label's key value pair are added successfully
		addedLabelKey, addedLabelValue := util.GetLabelKVFromSet(nsLabelVal)
		if addedLabelValue != "" {
			curNsObj.appendLabels(map[string]string{addedLabelKey: addedLabelValue}, appendToExistingLabels)
		}
	}

	// Append all labels to the cache NS obj
	// If due to ordering issue the above deleted and added labels are not correct,
	// this below appendLabels will help ensure correct state in cache for all successful ops.
	curNsObj.appendLabels(newNsLabel, clearExistingLabels)
	nsc.npmNamespaceCache.NsMap[newNsName] = curNsObj

	return nil
}

// cleanDeletedNamespace handles deleting namespace from ipset.
func (nsc *NamespaceController) cleanDeletedNamespace(cachedNsKey string) error {
	klog.Infof("NAMESPACE DELETING: [%s]", cachedNsKey)
	cachedNsObj, exists := nsc.npmNamespaceCache.NsMap[cachedNsKey]
	if !exists {
		return nil
	}

	klog.Infof("NAMESPACE DELETING cached labels: [%s/%v]", cachedNsKey, cachedNsObj.LabelsMap)

	var err error
	// Delete the namespace from its label's ipset list.
	for nsLabelKey, nsLabelVal := range cachedNsObj.LabelsMap {

		labelKey := &ipsets.IPSetMetadata{Name: nsLabelKey, Type: ipsets.KeyLabelOfNamespace}
		toBeDeletedKey := []*ipsets.IPSetMetadata{ipsets.NewIPSetMetadata(cachedNsKey, ipsets.KeyLabelOfNamespace)}

		labelIpsetName := util.GetNSNameWithPrefix(nsLabelKey)
		klog.Infof("Deleting namespace %s from ipset list %s", cachedNsKey, labelIpsetName)
		if err = nsc.dp.RemoveFromList(labelKey, toBeDeletedKey, nil); err != nil {
			metrics.SendErrorLogAndMetric(util.NSID, "[DeleteNamespace] Error: failed to delete namespace %s from ipset list %s with err: %v", cachedNsKey, labelIpsetName, err)
			return fmt.Errorf("failed to clean deleted namespace when deleting key with err %w", err)
		}

		labelKeyValue := &ipsets.IPSetMetadata{Name: nsLabelKey, Type: ipsets.KeyValueLabelOfNamespace}
		toBeDeletedKeyValue := []*ipsets.IPSetMetadata{ipsets.NewIPSetMetadata(cachedNsKey, ipsets.KeyValueLabelOfNamespace)}

		labelIpsetName = util.GetNSNameWithPrefix(util.GetIpSetFromLabelKV(nsLabelKey, nsLabelVal))
		klog.Infof("Deleting namespace %s from ipset list %s", cachedNsKey, labelIpsetName)
		if err = nsc.dp.RemoveFromList(labelKeyValue, toBeDeletedKeyValue, nil); err != nil {
			metrics.SendErrorLogAndMetric(util.NSID, "[DeleteNamespace] Error: failed to delete namespace %s from ipset list %s with err: %v", cachedNsKey, labelIpsetName, err)
			return fmt.Errorf("failed to clean deleted namespace when deleting key value with err %w", err)
		}

		// remove labels from the cache NS obj
		cachedNsObj.removeLabelsWithKey(nsLabelKey)
	}

	allNamespacesSet := &ipsets.IPSetMetadata{Name: util.KubeAllNamespacesFlag, Type: ipsets.Namespace}
	toBeDeletedCachedKey := []*ipsets.IPSetMetadata{ipsets.NewIPSetMetadata(cachedNsKey, ipsets.Namespace)}

	// Delete the namespace from all-namespace ipset list.
	if err = nsc.dp.RemoveFromList(allNamespacesSet, toBeDeletedCachedKey, nil); err != nil {
		metrics.SendErrorLogAndMetric(util.NSID, "[DeleteNamespace] Error: failed to delete namespace %s from ipset list %s with err: %v", cachedNsKey, util.KubeAllNamespacesFlag, err)
		return fmt.Errorf("failed to remove from list during clean deleted namespace %w", err)
	}

	// Delete ipset for the namespace.
	nsc.dp.DeleteIPSet(ipsets.NewIPSetMetadata(cachedNsKey, ipsets.Namespace))

	delete(nsc.npmNamespaceCache.NsMap, cachedNsKey)

	return nil
}
