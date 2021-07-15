// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"fmt"
	"reflect"
	"time"

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
	"k8s.io/klog"
	utilexec "k8s.io/utils/exec"
)

type LabelAppendOperation bool

const (
	ClearExistingLabels    LabelAppendOperation = true
	AppendToExistingLabels LabelAppendOperation = false
)

type Namespace struct {
	name      string
	LabelsMap map[string]string // NameSpace labels
	SetMap    map[string]string
	IpsMgr    *ipsm.IpsetManager
	iptMgr    *iptm.IptablesManager
}

// newNS constructs a new namespace object.
func newNs(name string, exec utilexec.Interface) (*Namespace, error) {
	ns := &Namespace{
		name:      name,
		LabelsMap: make(map[string]string),
		SetMap:    make(map[string]string),
		IpsMgr:    ipsm.NewIpsetManager(exec),
		iptMgr:    iptm.NewIptablesManager(exec, iptm.NewIptOperationShim()),
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

func (nsObj *Namespace) appendLabels(new map[string]string, clear LabelAppendOperation) {
	if clear {
		nsObj.LabelsMap = make(map[string]string)
	}
	for k, v := range new {
		nsObj.LabelsMap[k] = v
	}
}

func (nsObj *Namespace) removeLabelsWithKey(key string) {
	delete(nsObj.LabelsMap, key)
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

	klog.Infof("[NAMESPACE %s EVENT] for namespace [%s]", event, key)

	needSync = true
	return key, needSync
}

func (nsc *nameSpaceController) addNamespace(obj interface{}) {
	key, needSync := nsc.needSync(obj, "ADD")
	if !needSync {
		klog.Infof("[NAMESPACE ADD EVENT] No need to sync this namespace [%s]", key)
		return
	}
	nsc.workqueue.Add(key)
}

func (nsc *nameSpaceController) updateNamespace(old, new interface{}) {
	key, needSync := nsc.needSync(new, "UPDATE")
	if !needSync {
		klog.Infof("[NAMESPACE UPDATE EVENT] No need to sync this namespace [%s]", key)
		return
	}

	nsObj, _ := new.(*corev1.Namespace)
	oldNsObj, ok := old.(*corev1.Namespace)
	if ok {
		if oldNsObj.ResourceVersion == nsObj.ResourceVersion {
			klog.Infof("[NAMESPACE UPDATE EVENT] Resourceversion is same for this namespace [%s]", key)
			return
		}
	}

	nsKey := util.GetNSNameWithPrefix(key)

	nsc.npMgr.Lock()
	defer nsc.npMgr.Unlock()
	cachedNsObj, nsExists := nsc.npMgr.NsMap[nsKey]
	if nsExists {
		if reflect.DeepEqual(cachedNsObj.LabelsMap, nsObj.ObjectMeta.Labels) {
			klog.Infof("[NAMESPACE UPDATE EVENT] Namespace [%s] labels did not change", key)
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
	if key, err = cache.MetaNamespaceKeyFunc(nsObj); err != nil {
		utilruntime.HandleError(err)
		metrics.SendErrorLogAndMetric(util.NSID, "[NAMESPACE DELETE EVENT] Error: nameSpaceKey is empty for %s namespace", util.GetNSNameWithPrefix(nsObj.Name))
		return
	}

	nsc.npMgr.Lock()
	defer nsc.npMgr.Unlock()

	nsKey := util.GetNSNameWithPrefix(key)
	_, nsExists := nsc.npMgr.NsMap[nsKey]
	if !nsExists {
		klog.Infof("[NAMESPACE DELETE EVENT] Namespace [%s] does not exist in case, so returning", key)
		return
	}

	nsc.workqueue.Add(key)
}

func (nsc *nameSpaceController) Run(threadiness int, stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer nsc.workqueue.ShutDown()

	klog.Info("Starting Namespace controller\n")
	klog.Info("Starting workers")
	// Launch workers to process namespace resources
	for i := 0; i < threadiness; i++ {
		go wait.Until(nsc.runWorker, time.Second, stopCh)
	}

	klog.Info("Started workers")
	<-stopCh
	klog.Info("Shutting down workers")

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
		klog.Infof("Successfully synced '%s'", key)
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
			klog.Infof("NameSpace %s not found, may be it is deleted", key)
			// find the nsMap key and start cleaning up process (calling cleanDeletedNamespace function)
			cachedNsKey := util.GetNSNameWithPrefix(key)
			// cleanDeletedNamespace will check if the NS exists in cache, if it does, then proceeds with deletion
			// if it does not exists, then event will be no-op
			err = nsc.cleanDeletedNamespace(cachedNsKey)
			if err != nil {
				// need to retry this cleaning-up process
				metrics.SendErrorLogAndMetric(util.NSID, "Error: %v when namespace is not found", err)
				return fmt.Errorf("Error: %v when namespace is not found", err)
			}
		}
		return err
	}

	if nsObj.DeletionTimestamp != nil || nsObj.DeletionGracePeriodSeconds != nil {
		return nsc.cleanDeletedNamespace(util.GetNSNameWithPrefix(nsObj.Name))

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

	corev1NsName, corev1NsLabels := util.GetNSNameWithPrefix(nsObj.ObjectMeta.Name), nsObj.ObjectMeta.Labels
	klog.Infof("NAMESPACE CREATING: [%s/%v]", corev1NsName, corev1NsLabels)

	ipsMgr := nsc.npMgr.NsMap[util.KubeAllNamespacesFlag].IpsMgr
	// Create ipset for the namespace.
	if err = ipsMgr.CreateSet(corev1NsName, []string{util.IpsetNetHashFlag}); err != nil {
		metrics.SendErrorLogAndMetric(util.NSID, "[AddNamespace] Error: failed to create ipset for namespace %s with err: %v", corev1NsName, err)
		return err
	}

	if err = ipsMgr.AddToList(util.KubeAllNamespacesFlag, corev1NsName); err != nil {
		metrics.SendErrorLogAndMetric(util.NSID, "[AddNamespace] Error: failed to add %s to all-namespace ipset list with err: %v", corev1NsName, err)
		return err
	}

	npmNs, _ := newNs(corev1NsName, nsc.npMgr.Exec)
	nsc.npMgr.NsMap[corev1NsName] = npmNs

	// Add the namespace to its label's ipset list.
	for nsLabelKey, nsLabelVal := range corev1NsLabels {
		labelIpsetName := util.GetNSNameWithPrefix(nsLabelKey)
		klog.Infof("Adding namespace %s to ipset list %s", corev1NsName, labelIpsetName)
		if err = ipsMgr.AddToList(labelIpsetName, corev1NsName); err != nil {
			metrics.SendErrorLogAndMetric(util.NSID, "[AddNamespace] Error: failed to add namespace %s to ipset list %s with err: %v", corev1NsName, labelIpsetName, err)
			return err
		}

		labelIpsetName = util.GetNSNameWithPrefix(util.GetIpSetFromLabelKV(nsLabelKey, nsLabelVal))
		klog.Infof("Adding namespace %s to ipset list %s", corev1NsName, labelIpsetName)
		if err = ipsMgr.AddToList(labelIpsetName, corev1NsName); err != nil {
			metrics.SendErrorLogAndMetric(util.NSID, "[AddNamespace] Error: failed to add namespace %s to ipset list %s with err: %v", corev1NsName, labelIpsetName, err)
			return err
		}

		// Append succeeded labels to the cache NS obj
		npmNs.appendLabels(map[string]string{nsLabelKey: nsLabelVal}, AppendToExistingLabels)
	}

	return nil
}

// syncUpdateNameSpace handles updating namespace in ipset.
func (nsc *nameSpaceController) syncUpdateNameSpace(newNsObj *corev1.Namespace) error {
	var err error
	newNsName, newNsLabel := util.GetNSNameWithPrefix(newNsObj.ObjectMeta.Name), newNsObj.ObjectMeta.Labels
	klog.Infof(
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
		klog.Infof("Deleting namespace %s from ipset list %s", newNsName, labelKey)
		if err = ipsMgr.DeleteFromList(labelKey, newNsName); err != nil {
			metrics.SendErrorLogAndMetric(util.NSID, "[UpdateNamespace] Error: failed to delete namespace %s from ipset list %s with err: %v", newNsName, labelKey, err)
			return err
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
		labelKey := util.GetNSNameWithPrefix(nsLabelVal)
		klog.Infof("Adding namespace %s to ipset list %s", newNsName, labelKey)
		if err = ipsMgr.AddToList(labelKey, newNsName); err != nil {
			metrics.SendErrorLogAndMetric(util.NSID, "[UpdateNamespace] Error: failed to add namespace %s to ipset list %s with err: %v", newNsName, labelKey, err)
			return err
		}
		// {IMPORTANT} Same as above order is assumed to be key and then key+val. NPM should only append to existing labels
		// only after both ipsets for a given label's key value pair are added successfully
		addedLabelKey, addedLabelValue := util.GetLabelKVFromSet(nsLabelVal)
		if addedLabelValue != "" {
			curNsObj.appendLabels(map[string]string{addedLabelKey: addedLabelValue}, AppendToExistingLabels)
		}
	}

	// Append all labels to the cache NS obj
	// If due to ordering issue the above deleted and added labels are not correct,
	// this below appendLabels will help ensure correct state in cache for all successful ops.
	curNsObj.appendLabels(newNsLabel, ClearExistingLabels)
	nsc.npMgr.NsMap[newNsName] = curNsObj

	return nil
}

// cleanDeletedNamespace handles deleting namespace from ipset.
func (nsc *nameSpaceController) cleanDeletedNamespace(cachedNsKey string) error {
	klog.Infof("NAMESPACE DELETING: [%s]", cachedNsKey)

	cachedNsObj, exists := nsc.npMgr.NsMap[cachedNsKey]
	if !exists {
		return nil
	}

	klog.Infof("NAMESPACE DELETING cached labels: [%s/%v]", cachedNsKey, cachedNsObj.LabelsMap)

	var err error
	ipsMgr := nsc.npMgr.NsMap[util.KubeAllNamespacesFlag].IpsMgr
	// Delete the namespace from its label's ipset list.
	for nsLabelKey, nsLabelVal := range cachedNsObj.LabelsMap {
		labelIpsetName := util.GetNSNameWithPrefix(nsLabelKey)
		klog.Infof("Deleting namespace %s from ipset list %s", cachedNsKey, labelIpsetName)
		if err = ipsMgr.DeleteFromList(labelIpsetName, cachedNsKey); err != nil {
			metrics.SendErrorLogAndMetric(util.NSID, "[DeleteNamespace] Error: failed to delete namespace %s from ipset list %s with err: %v", cachedNsKey, labelIpsetName, err)
			return err
		}

		labelIpsetName = util.GetNSNameWithPrefix(util.GetIpSetFromLabelKV(nsLabelKey, nsLabelVal))
		klog.Infof("Deleting namespace %s from ipset list %s", cachedNsKey, labelIpsetName)
		if err = ipsMgr.DeleteFromList(labelIpsetName, cachedNsKey); err != nil {
			metrics.SendErrorLogAndMetric(util.NSID, "[DeleteNamespace] Error: failed to delete namespace %s from ipset list %s with err: %v", cachedNsKey, labelIpsetName, err)
			return err
		}

		// remove labels from the cache NS obj
		cachedNsObj.removeLabelsWithKey(nsLabelKey)
	}

	// Delete the namespace from all-namespace ipset list.
	if err = ipsMgr.DeleteFromList(util.KubeAllNamespacesFlag, cachedNsKey); err != nil {
		metrics.SendErrorLogAndMetric(util.NSID, "[DeleteNamespace] Error: failed to delete namespace %s from ipset list %s with err: %v", cachedNsKey, util.KubeAllNamespacesFlag, err)
		return err
	}

	// Delete ipset for the namespace.
	if err = ipsMgr.DeleteSet(cachedNsKey); err != nil {
		metrics.SendErrorLogAndMetric(util.NSID, "[DeleteNamespace] Error: failed to delete ipset for namespace %s with err: %v", cachedNsKey, err)
		return err
	}

	delete(nsc.npMgr.NsMap, cachedNsKey)

	return nil
}
