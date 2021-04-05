// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"fmt"
	"reflect"
	"time"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/ipsm"
	"github.com/Azure/azure-container-networking/npm/iptm"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/util"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	coreinformer "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type Namespace struct {
	name            string
	LabelsMap       map[string]string // NameSpace labels
	SetMap          map[string]string
	IpsMgr          *ipsm.IpsetManager
	iptMgr          *iptm.IptablesManager
	resourceVersion uint64 // NameSpace ResourceVersion
}

// newNS constructs a new namespace object.
func newNs(name string) (*Namespace, error) {
	ns := &Namespace{
		name:      name,
		LabelsMap: make(map[string]string),
		SetMap:    make(map[string]string),
		IpsMgr:    ipsm.NewIpsetManager(),
		iptMgr:    iptm.NewIptablesManager(),
		// resource version is converted to uint64
		// so make sure it is initialized to "0"
		resourceVersion: 0,
	}

	return ns, nil
}

func (nsObj *Namespace) getNamespaceObjFromNsObj() *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   nsObj.name,
			Labels: nsObj.LabelsMap,
		},
	}
}

// setResourceVersion setter func for RV
func (nsObj *Namespace) setResourceVersion(rv string) {
	nsObj.resourceVersion = util.ParseResourceVersion(rv)
}

func isSystemNs(nsObj *corev1.Namespace) bool {
	return nsObj.ObjectMeta.Name == util.KubeSystemFlag
}

type nameSpaceController struct {
	clientset             kubernetes.Interface
	nameSpaceLister       corelisters.NamespaceLister
	nameSpaceListerSynced cache.InformerSynced
	workqueue             workqueue.RateLimitingInterface
	// TODO does not need to have whole NetworkPolicyManager pointer. Need to improve it
	npMgr *NetworkPolicyManager
}

func NewNameSpaceController(nameSpaceInformer coreinformer.NamespaceInformer, clientset kubernetes.Interface, npMgr *NetworkPolicyManager) *nameSpaceController {
	nameSpaceController := &nameSpaceController{
		clientset:             clientset,
		nameSpaceLister:       nameSpaceInformer.Lister(),
		nameSpaceListerSynced: nameSpaceInformer.Informer().HasSynced,
		workqueue:             workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Namespaces"),
		npMgr:                 npMgr,
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
func (nsc *nameSpaceController) needSync(obj interface{}, event string) (string, bool) {
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
		metrics.SendErrorLogAndMetric(util.NSID, "[NAMESPACE %s EVENT] Error: NameSpaceKey is empty for %s namespace", event, util.GetNSNameWithPrefix(nsObj.Name))
		return key, needSync
	}

	log.Logf("[NAMESPACE %s EVENT] for namespace [%s]", event, key)

	needSync = true
	return key, needSync
}

func (nsc *nameSpaceController) addNamespace(obj interface{}) {
	key, needSync := nsc.needSync(obj, "ADD")
	if !needSync {
		log.Logf("[NAMESPACE ADD EVENT] No need to sync this namespace [%s]", key)
		return
	}
	nsc.workqueue.Add(key)
}

func (nsc *nameSpaceController) updateNamespace(old, new interface{}) {
	key, needSync := nsc.needSync(new, "UPDATE")
	if !needSync {
		log.Logf("[NAMESPACE UPDATE EVENT] No need to sync this namespace [%s]", key)
		return
	}

	nsObj, _ := new.(*corev1.Namespace)
	oldNsObj, ok := old.(*corev1.Namespace)
	if ok {
		if oldNsObj.ResourceVersion == nsObj.ResourceVersion {
			log.Logf("[NAMESPACE UPDATE EVENT] Resourceversion is same for this namespace [%s]", key)
			return
		}
	}

	nsKey := util.GetNSNameWithPrefix(key)

	nsc.npMgr.Lock()
	defer nsc.npMgr.Unlock()
	cachedNsObj, nsExists := nsc.npMgr.NsMap[nsKey]
	if nsExists {
		if reflect.DeepEqual(cachedNsObj.LabelsMap, nsObj.ObjectMeta.Labels) {
			log.Logf("[NAMESPACE UPDATE EVENT] Namespace [%s] labels did not change", key)
			return
		}
	}

	nsc.workqueue.Add(key)
}

func (nsc *nameSpaceController) deleteNamespace(obj interface{}) {
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
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		metrics.SendErrorLogAndMetric(util.NSID, "[NAMESPACE DELETE EVENT] Error: nameSpaceKey is empty for %s namespace", util.GetNSNameWithPrefix(nsObj.Name))
		return
	}

	nsc.npMgr.Lock()
	defer nsc.npMgr.Unlock()

	nsKey := util.GetNSNameWithPrefix(key)
	_, nsExists := nsc.npMgr.NsMap[nsKey]
	if !nsExists {
		log.Logf("[NAMESPACE DELETE EVENT] Namespace [%s] does not exist in case, so returning", key)
		return
	}

	nsc.workqueue.Add(key)
}

func (nsc *nameSpaceController) Run(threadiness int, stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer nsc.workqueue.ShutDown()

	log.Logf("Starting Namespace controller\n")
	log.Logf("Starting workers")
	// Launch workers to process namespace resources
	for i := 0; i < threadiness; i++ {
		go wait.Until(nsc.runWorker, time.Second, stopCh)
	}

	log.Logf("Started workers")
	<-stopCh
	log.Logf("Shutting down workers")

	return nil
}

func (nsc *nameSpaceController) runWorker() {
	for nsc.processNextWorkItem() {
	}
}

func (nsc *nameSpaceController) processNextWorkItem() bool {
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
			utilruntime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}
		// Run the syncNameSpace, passing it the namespace string of the
		// resource to be synced.
		// TODO : may consider using "c.queue.AddAfter(key, *requeueAfter)" according to error type later
		if err := nsc.syncNameSpace(key); err != nil {
			// Put the item back on the workqueue to handle any transient errors.
			nsc.workqueue.AddRateLimited(key)
			metrics.SendErrorLogAndMetric(util.NSID, "[processNextWorkItem] Error: failed to syncNameSpace %s. Requeuing with err: %v", key, err)
			return err
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		nsc.workqueue.Forget(obj)
		log.Logf("Successfully synced '%s'", key)
		return nil
	}(obj)

	if err != nil {
		utilruntime.HandleError(err)
		return true
	}

	return true
}

// syncNameSpace compares the actual state with the desired, and attempts to converge the two.
func (nsc *nameSpaceController) syncNameSpace(key string) error {
	// Get the NameSpace resource with this key
	nsObj, err := nsc.nameSpaceLister.Get(key)
	// lock to complete events
	// TODO: Reduce scope of lock later
	nsc.npMgr.Lock()
	defer nsc.npMgr.Unlock()
	if err != nil {
		if errors.IsNotFound(err) {
			utilruntime.HandleError(fmt.Errorf("NameSpace '%s' in work queue no longer exists", key))
			// find the namespace object from a local cache and start cleaning up process (calling cleanDeletedNamespace function)
			nsKey := util.GetNSNameWithPrefix(key)
			cachedNs, found := nsc.npMgr.NsMap[nsKey]
			// if the namespace does not exists, we do not need to clean up process and retry it
			if !found {
				return nil
			}

			// Found the namespace object from NsMap local cache and start cleaning up processes
			err = nsc.cleanDeletedNamespace(cachedNs.name, cachedNs.LabelsMap)
			if err != nil {
				// need to retry this cleaning-up process
				metrics.SendErrorLogAndMetric(util.NSID, "Error: %v when namespace is not found", err)
				return fmt.Errorf("Error: %v when namespace is not found", err)
			}
		}
		return err
	}

	if nsObj.DeletionTimestamp != nil || nsObj.DeletionGracePeriodSeconds != nil {
		return nsc.cleanDeletedNamespace(util.GetNSNameWithPrefix(nsObj.Name), nsObj.Labels)

	}

	err = nsc.syncUpdateNameSpace(nsObj)
	// 1. deal with error code and retry this
	if err != nil {
		metrics.SendErrorLogAndMetric(util.NSID, "[syncNameSpace] failed to sync namespace due to  %s", err.Error())
		return err
	}

	return nil
}

// syncAddNameSpace handles adding namespace to ipset.
func (nsc *nameSpaceController) syncAddNameSpace(nsObj *corev1.Namespace) error {
	var err error

	nsName, nsLabel := util.GetNSNameWithPrefix(nsObj.ObjectMeta.Name), nsObj.ObjectMeta.Labels
	log.Logf("NAMESPACE CREATING: [%s/%v]", nsName, nsLabel)

	ipsMgr := nsc.npMgr.NsMap[util.KubeAllNamespacesFlag].IpsMgr
	// Create ipset for the namespace.
	if err = ipsMgr.CreateSet(nsName, []string{util.IpsetNetHashFlag}); err != nil {
		metrics.SendErrorLogAndMetric(util.NSID, "[AddNamespace] Error: failed to create ipset for namespace %s with err: %v", nsName, err)
		return err
	}

	if err = ipsMgr.AddToList(util.KubeAllNamespacesFlag, nsName); err != nil {
		metrics.SendErrorLogAndMetric(util.NSID, "[AddNamespace] Error: failed to add %s to all-namespace ipset list with err: %v", nsName, err)
		return err
	}

	// Add the namespace to its label's ipset list.
	nsLabels := util.GetSetsFromLabels(nsObj.ObjectMeta.Labels)
	for _, nsLabel := range nsLabels {
		labelKey := util.GetNSNameWithPrefix(nsLabel)
		log.Logf("Adding namespace %s to ipset list %s", nsName, labelKey)
		if err = ipsMgr.AddToList(labelKey, nsName); err != nil {
			metrics.SendErrorLogAndMetric(util.NSID, "[AddNamespace] Error: failed to add namespace %s to ipset list %s with err: %v", nsName, labelKey, err)
			return err
		}
	}

	ns, _ := newNs(nsName)
	ns.setResourceVersion(nsObj.GetObjectMeta().GetResourceVersion())

	// Append all labels to the cache NS obj
	ns.LabelsMap = util.AppendMap(ns.LabelsMap, nsLabel)
	nsc.npMgr.NsMap[nsName] = ns

	return nil
}

// syncUpdateNameSpace handles updating namespace in ipset.
func (nsc *nameSpaceController) syncUpdateNameSpace(newNsObj *corev1.Namespace) error {
	var err error
	newNsName, newNsLabel := util.GetNSNameWithPrefix(newNsObj.ObjectMeta.Name), newNsObj.ObjectMeta.Labels
	log.Logf(
		"NAMESPACE UPDATING:\n namespace: [%s/%v]",
		newNsName, newNsLabel,
	)

	// If orignal AddNamespace failed for some reason, then NS will not be found
	// in nsMap, resulting in retry of ADD.
	curNsObj, exists := nsc.npMgr.NsMap[newNsName]
	if !exists {
		if newNsObj.ObjectMeta.DeletionTimestamp == nil && newNsObj.ObjectMeta.DeletionGracePeriodSeconds == nil {
			if err = nsc.syncAddNameSpace(newNsObj); err != nil {
				return err
			}
		}

		return nil
	}

	//If the Namespace is not deleted, delete removed labels and create new labels
	addToIPSets, deleteFromIPSets := util.GetIPSetListCompareLabels(curNsObj.LabelsMap, newNsLabel)

	// Delete the namespace from its label's ipset list.
	ipsMgr := nsc.npMgr.NsMap[util.KubeAllNamespacesFlag].IpsMgr
	for _, nsLabelVal := range deleteFromIPSets {
		labelKey := util.GetNSNameWithPrefix(nsLabelVal)
		log.Logf("Deleting namespace %s from ipset list %s", newNsName, labelKey)
		if err = ipsMgr.DeleteFromList(labelKey, newNsName); err != nil {
			metrics.SendErrorLogAndMetric(util.NSID, "[UpdateNamespace] Error: failed to delete namespace %s from ipset list %s with err: %v", newNsName, labelKey, err)
			return err
		}
	}

	// Add the namespace to its label's ipset list.
	for _, nsLabelVal := range addToIPSets {
		labelKey := util.GetNSNameWithPrefix(nsLabelVal)
		log.Logf("Adding namespace %s to ipset list %s", newNsName, labelKey)
		if err = ipsMgr.AddToList(labelKey, newNsName); err != nil {
			metrics.SendErrorLogAndMetric(util.NSID, "[UpdateNamespace] Error: failed to add namespace %s to ipset list %s with err: %v", newNsName, labelKey, err)
			return err
		}
	}

	// Append all labels to the cache NS obj
	curNsObj.LabelsMap = util.ClearAndAppendMap(curNsObj.LabelsMap, newNsLabel)
	curNsObj.setResourceVersion(newNsObj.GetObjectMeta().GetResourceVersion())
	nsc.npMgr.NsMap[newNsName] = curNsObj

	return nil
}

// cleanDeletedNamespace handles deleting namespace from ipset.
func (nsc *nameSpaceController) cleanDeletedNamespace(nsName string, nsLabel map[string]string) error {
	log.Logf("NAMESPACE DELETING: [%s/%v]", nsName, nsLabel)

	cachedNsObj, exists := nsc.npMgr.NsMap[nsName]
	if !exists {
		return nil
	}

	log.Logf("NAMESPACE DELETING cached labels: [%s/%v]", nsName, cachedNsObj.LabelsMap)

	var err error
	ipsMgr := nsc.npMgr.NsMap[util.KubeAllNamespacesFlag].IpsMgr
	nsLabels := util.GetIPSetListFromLabels(cachedNsObj.LabelsMap)
	// Delete the namespace from its label's ipset list.
	for _, nsLabelKey := range nsLabels {
		labelKey := util.GetNSNameWithPrefix(nsLabelKey)
		log.Logf("Deleting namespace %s from ipset list %s", nsName, labelKey)
		if err = ipsMgr.DeleteFromList(labelKey, nsName); err != nil {
			metrics.SendErrorLogAndMetric(util.NSID, "[DeleteNamespace] Error: failed to delete namespace %s from ipset list %s with err: %v", nsName, labelKey, err)
			return err
		}
	}

	// Delete the namespace from all-namespace ipset list.
	if err = ipsMgr.DeleteFromList(util.KubeAllNamespacesFlag, nsName); err != nil {
		metrics.SendErrorLogAndMetric(util.NSID, "[DeleteNamespace] Error: failed to delete namespace %s from ipset list %s with err: %v", nsName, util.KubeAllNamespacesFlag, err)
		return err
	}

	// Delete ipset for the namespace.
	if err = ipsMgr.DeleteSet(nsName); err != nil {
		metrics.SendErrorLogAndMetric(util.NSID, "[DeleteNamespace] Error: failed to delete ipset for namespace %s with err: %v", nsName, err)
		return err
	}

	delete(nsc.npMgr.NsMap, nsName)

	return nil
}
