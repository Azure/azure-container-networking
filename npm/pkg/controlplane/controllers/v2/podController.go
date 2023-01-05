// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package controllers

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/pkg/controlplane/controllers/common"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/util"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	coreinformer "k8s.io/client-go/informers/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
)

// NamedPortOperation decides opeartion (e.g., delete or add) for named port ipset in manageNamedPortIpsets
type NamedPortOperation string

const (
	deleteNamedPort NamedPortOperation = "del"
	addNamedPort    NamedPortOperation = "add"
)

var kubeAllNamespaces = &ipsets.IPSetMetadata{Name: util.KubeAllNamespacesFlag, Type: ipsets.KeyLabelOfNamespace}

type PodController struct {
	podLister corelisters.PodLister
	workqueue workqueue.RateLimitingInterface
	dp        dataplane.GenericDataplane
	podMap    map[string]*common.NpmPod // Key is <nsname>/<podname>
	sync.RWMutex
	npmNamespaceCache *NpmNamespaceCache
}

func NewPodController(podInformer coreinformer.PodInformer, dp dataplane.GenericDataplane, npmNamespaceCache *NpmNamespaceCache) *PodController {
	podController := &PodController{
		podLister:         podInformer.Lister(),
		workqueue:         workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Pods"),
		dp:                dp,
		podMap:            make(map[string]*common.NpmPod),
		npmNamespaceCache: npmNamespaceCache,
	}

	podInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    podController.addPod,
			UpdateFunc: podController.updatePod,
			DeleteFunc: podController.deletePod,
		},
	)
	return podController
}

func (c *PodController) MarshalJSON() ([]byte, error) {
	c.Lock()
	defer c.Unlock()

	podMapRaw, err := json.Marshal(c.podMap)
	if err != nil {
		return nil, errors.Errorf("failed to marshal podMap due to %v", err)
	}

	return podMapRaw, nil
}

func (c *PodController) LengthOfPodMap() int {
	return len(c.podMap)
}

// needSync filters the event if the event is not required to handle
func (c *PodController) needSync(eventType string, obj interface{}) (string, bool) {
	needSync := false
	var key string

	podObj, ok := obj.(*corev1.Pod)
	if !ok {
		metrics.SendErrorLogAndMetric(util.PodID, "ADD Pod: Received unexpected object type: %v", obj)
		return key, needSync
	}

	if !hasValidPodIP(podObj) {
		return key, needSync
	}

	if isHostNetworkPod(podObj) {
		return key, needSync
	}

	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		metrics.SendErrorLogAndMetric(util.PodID, "[POD %s EVENT] Error: podKey is empty for %s pod in %s with UID %s",
			eventType, podObj.Name, util.GetNSNameWithPrefix(podObj.Namespace), podObj.UID)
		return key, needSync
	}

	needSync = true
	return key, needSync
}

func (c *PodController) addPod(obj interface{}) {
	key, needSync := c.needSync("ADD", obj)
	if !needSync {
		return
	}
	podObj, _ := obj.(*corev1.Pod)

	// To check whether this pod is needed to queue or not.
	// If the pod are in completely terminated states, the pod is not enqueued to avoid unnecessary computation.
	if isCompletePod(podObj) {
		return
	}

	c.workqueue.Add(key)
}

func (c *PodController) updatePod(old, newp interface{}) {
	key, needSync := c.needSync("UPDATE", newp)
	if !needSync {
		return
	}

	// needSync checked validation of casting newPod.
	newPod, _ := newp.(*corev1.Pod)
	oldPod, ok := old.(*corev1.Pod)
	if ok {
		if oldPod.ResourceVersion == newPod.ResourceVersion {
			// Periodic resync will send update events for all known pods.
			// Two different versions of the same pods will always have different RVs.
			return
		}
	}

	c.workqueue.Add(key)
}

func (c *PodController) deletePod(obj interface{}) {
	podObj, ok := obj.(*corev1.Pod)
	// DeleteFunc gets the final state of the resource (if it is known).
	// Otherwise, it gets an object of type DeletedFinalStateUnknown.
	// This can happen if the watch is closed and misses the delete event and
	// the controller doesn't notice the deletion until the subsequent re-list
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			metrics.SendErrorLogAndMetric(util.PodID, "[POD DELETE EVENT] Pod: Received unexpected object type: %v", obj)
			return
		}

		if podObj, ok = tombstone.Obj.(*corev1.Pod); !ok {
			metrics.SendErrorLogAndMetric(util.PodID, "[POD DELETE EVENT] Pod: Received unexpected object type (error decoding object tombstone, invalid type): %v", obj)
			return
		}
	}

	klog.Infof("[POD DELETE EVENT] for %s in %s", podObj.Name, podObj.Namespace)
	if isHostNetworkPod(podObj) {
		return
	}

	var err error
	var key string
	if key, err = cache.MetaNamespaceKeyFunc(podObj); err != nil {
		utilruntime.HandleError(err)
		metrics.SendErrorLogAndMetric(util.PodID, "[POD DELETE EVENT] Error: podKey is empty for %s pod in %s with UID %s",
			podObj.ObjectMeta.Name, util.GetNSNameWithPrefix(podObj.Namespace), podObj.UID)
		return
	}

	c.workqueue.Add(key)
}

func (c *PodController) Run(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()

	klog.Infof("Starting Pod worker")
	go wait.Until(c.runWorker, time.Second, stopCh)

	klog.Info("Started Pod workers")
	<-stopCh
	klog.Info("Shutting down Pod workers")
}

func (c *PodController) runWorker() {
	for c.processNextWorkItem() {
	}
}

func (c *PodController) processNextWorkItem() bool {
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
			utilruntime.HandleError(fmt.Errorf("expected string in workqueue got %#v, err %w", obj, errWorkqueueFormatting))
			return nil
		}
		// Run the syncPod, passing it the namespace/name string of the
		// Pod resource to be synced.
		if err := c.syncPod(key); err != nil {
			// Put the item back on the workqueue to handle any transient errors.
			c.workqueue.AddRateLimited(key)
			metrics.SendErrorLogAndMetric(util.PodID, "[podController processNextWorkItem] Error: failed to syncPod %s. Requeuing with err: %v", key, err)
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
		return true
	}

	return true
}

// syncPod compares the actual state with the desired, and attempts to converge the two.
func (c *PodController) syncPod(key string) error {
	// timer for recording execution times
	timer := metrics.StartNewTimer()

	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("failed to split meta namespace key %s with err %w", key, err))
		return nil //nolint HandleError  is used instead of returning error to caller
	}

	// Get the Pod resource with this namespace/name
	pod, err := c.podLister.Pods(namespace).Get(name)

	// apply dataplane and record exec time after syncing
	operationKind := metrics.NoOp
	defer func() {
		if err != nil {
			klog.Infof("[syncPod] failed to sync pod, but will apply any changes to the dataplane. err: %s", err.Error())
		}

		dperr := c.dp.ApplyDataPlane()

		// can't record this in another deferred func since deferred funcs are processed in LIFO order
		metrics.RecordControllerPodExecTime(timer, operationKind, err != nil && dperr != nil)

		if dperr != nil {
			if err == nil {
				err = fmt.Errorf("failed to apply dataplane changes while syncing pod. err: %w", dperr)
			} else {
				err = fmt.Errorf("failed to sync pod and apply dataplane changes. sync err: [%w], apply err: [%v]", err, dperr)
			}
		}
	}()

	c.Lock()
	defer c.Unlock()

	if err != nil {
		if apierrors.IsNotFound(err) {
			klog.Infof("pod %s not found, may be it is deleted", key)

			if _, ok := c.podMap[key]; ok {
				// record time to delete pod if it exists (can't call within cleanUpDeletedPod because this can be called by a pod update)
				operationKind = metrics.DeleteOp
			}

			// cleanUpDeletedPod will check if the pod exists in cache, if it does then proceeds with deletion
			// if it does not exists, then event will be no-op
			err = c.cleanUpDeletedPod(key)
			if err != nil {
				// need to retry this cleaning-up process
				return fmt.Errorf("error: %w when pod is not found", err)
			}
			return err
		}

		return err
	}

	// If this pod is completely in terminated states (which means pod is gracefully shutdown),
	// NPM starts clean-up the lastly applied states even in update events.
	// This proactive clean-up helps to miss stale pod object in case delete event is missed.
	if isCompletePod(pod) {
		if _, ok := c.podMap[key]; ok {
			// record time to delete pod if it exists (can't call within cleanUpDeletedPod because this can be called by a pod update)
			operationKind = metrics.DeleteOp
		}
		if err = c.cleanUpDeletedPod(key); err != nil {
			return fmt.Errorf("error: %w when when pod is in completed state", err)
		}
		return nil
	}

	cachedNpmPod, npmPodExists := c.podMap[key]
	if npmPodExists {
		// if pod does not have different states against lastly applied states stored in cachedNpmPod,
		// podController does not need to reconcile this update.
		// in this updatePod event, newPod was updated with states which PodController does not need to reconcile.
		if cachedNpmPod.NoUpdate(pod) {
			return nil
		}
	}

	operationKind, err = c.syncAddAndUpdatePod(pod)
	if err != nil {
		return fmt.Errorf("failed to sync pod due to %w", err)
	}

	return nil
}

func (c *PodController) syncAddedPod(podObj *corev1.Pod) error {
	klog.Infof("POD CREATING: [%s/%s/%s/%s/%+v/%s]", string(podObj.GetUID()), podObj.Namespace,
		podObj.Name, podObj.Spec.NodeName, podObj.Labels, podObj.Status.PodIP)

	if !util.IsIPV4(podObj.Status.PodIP) {
		msg := fmt.Sprintf("[syncAddedPod] warning: ADD POD  [%s/%s/%s/%+v] ignored as the PodIP is not valid ipv4 address. ip: [%s]", podObj.Namespace,
			podObj.Name, podObj.Spec.NodeName, podObj.Labels, podObj.Status.PodIP)
		metrics.SendLog(util.PodID, msg, metrics.PrintLog)
		// return nil so that we don't requeue.
		// Wait until an update event comes from API Server where the IP is valid e.g. if the IP is empty.
		// There may be latency in receiving the update event versus retrying on our own,
		// but this prevents us from retrying indefinitely for pods stuck in Running state with no IP as seen in AKS Windows Server '22.
		return nil
	}

	var err error
	podKey, _ := cache.MetaNamespaceKeyFunc(podObj)

	podMetadata := dataplane.NewPodMetadata(podKey, podObj.Status.PodIP, podObj.Spec.NodeName)

	namespaceSet := []*ipsets.IPSetMetadata{ipsets.NewIPSetMetadata(podObj.Namespace, ipsets.Namespace)}

	// Add the pod ip information into namespace's ipset.
	klog.Infof("Adding pod %s (ip : %s) to ipset %s", podKey, podObj.Status.PodIP, podObj.Namespace)
	if err = c.dp.AddToSets(namespaceSet, podMetadata); err != nil {
		return fmt.Errorf("[syncAddedPod] Error: failed to add pod to namespace ipset with err: %w", err)
	}

	// Create npmPod and add it to the podMap
	npmPodObj := common.NewNpmPod(podObj)
	c.podMap[podKey] = npmPodObj

	// Get lists of podLabelKey and podLabelKey + podLavelValue ,and then start adding them to ipsets.
	for labelKey, labelVal := range podObj.Labels {
		labelKeyValue := util.GetIpSetFromLabelKV(labelKey, labelVal)

		targetSetKey := ipsets.NewIPSetMetadata(labelKey, ipsets.KeyLabelOfPod)
		targetSetKeyValue := ipsets.NewIPSetMetadata(labelKeyValue, ipsets.KeyValueLabelOfPod)
		allSets := []*ipsets.IPSetMetadata{targetSetKey, targetSetKeyValue}

		klog.Infof("Creating ipsets %+v and %+v if they do not exist", targetSetKey, targetSetKeyValue)
		klog.Infof("Adding pod %s (ip : %s) to ipset %s and %s", podKey, npmPodObj.PodIP, labelKey, labelKeyValue)
		if err = c.dp.AddToSets(allSets, podMetadata); err != nil {
			return fmt.Errorf("[syncAddedPod] Error: failed to add pod to label ipset with err: %w", err)
		}
		npmPodObj.AppendLabels(map[string]string{labelKey: labelVal}, common.AppendToExistingLabels)
	}

	// Add pod's named ports from its ipset.
	klog.Infof("Adding named port ipsets")
	containerPorts := common.GetContainerPortList(podObj)
	if err = c.manageNamedPortIpsets(containerPorts, podKey, npmPodObj.PodIP, podObj.Spec.NodeName, addNamedPort); err != nil {
		return fmt.Errorf("[syncAddedPod] Error: failed to add pod to named port ipset with err: %w", err)
	}
	npmPodObj.AppendContainerPorts(podObj)

	return nil
}

// syncAddAndUpdatePod handles updating pod ip in its label's ipset.
func (c *PodController) syncAddAndUpdatePod(newPodObj *corev1.Pod) (metrics.OperationKind, error) {
	var err error
	podKey, _ := cache.MetaNamespaceKeyFunc(newPodObj)

	// lock before using nsMap since nsMap is shared with namespace controller
	c.npmNamespaceCache.Lock()
	if _, exists := c.npmNamespaceCache.NsMap[newPodObj.Namespace]; !exists {
		// Create ipset related to namespace which this pod belong to if it does not exist.

		toBeAdded := []*ipsets.IPSetMetadata{ipsets.NewIPSetMetadata(newPodObj.Namespace, ipsets.Namespace)}
		if err = c.dp.AddToLists([]*ipsets.IPSetMetadata{kubeAllNamespaces}, toBeAdded); err != nil {
			c.npmNamespaceCache.Unlock()
			// since the namespace doesn't exist, this must be a pod create event, so we'll return metrics.CreateOp
			return metrics.CreateOp, fmt.Errorf("[syncAddAndUpdatePod] Error: failed to add %s to all-namespace ipset list with err: %w", newPodObj.Namespace, err)
		}

		// Add namespace object into NsMap cache only when two ipset operations are successful.
		npmNs := common.NewNs(newPodObj.Namespace)
		c.npmNamespaceCache.NsMap[newPodObj.Namespace] = npmNs
	}
	c.npmNamespaceCache.Unlock()

	cachedNpmPod, exists := c.podMap[podKey]
	klog.Infof("[syncAddAndUpdatePod] updating Pod with key %s", podKey)
	// No cached npmPod exists. start adding the pod in a cache
	if !exists {
		return metrics.CreateOp, c.syncAddedPod(newPodObj)
	}
	// now we know this is an update event, and we'll return metrics.UpdateOp

	// Dealing with "updatePod" event - Compare last applied states against current Pod states
	// There are two possibilities for npmPodObj and newPodObj
	// #1 case The same object with the same UID and the same key (namespace + name)
	// #2 case Different objects with different UID, but the same key (namespace + name) due to missing some events for the old object

	// Dealing with #2 pod update event, the IP addresses of cached npmPod and newPodObj are different
	// NPM should clean up existing references of cached pod obj and its IP.
	// then, re-add new pod obj.
	if cachedNpmPod.PodIP != newPodObj.Status.PodIP {
		klog.Infof("Pod (Namespace:%s, Name:%s, newUid:%s), has cachedPodIp:%s which is different from PodIp:%s",
			newPodObj.Namespace, newPodObj.Name, string(newPodObj.UID), cachedNpmPod.PodIP, newPodObj.Status.PodIP)

		klog.Infof("Deleting cached Pod with key:%s first due to IP Mistmatch", podKey)
		if er := c.cleanUpDeletedPod(podKey); er != nil {
			return metrics.UpdateOp, er
		}

		klog.Infof("Adding back Pod with key:%s after IP Mistmatch", podKey)
		return metrics.UpdateOp, c.syncAddedPod(newPodObj)
	}

	// Dealing with #1 pod update event, the IP addresses of cached npmPod and newPodObj are same
	// If no change in labels, then GetIPSetListCompareLabels will return empty list.
	// Otherwise it returns list of deleted PodIP from cached pod's labels and list of added PodIp from new pod's labels
	addToIPSets, deleteFromIPSets := util.GetIPSetListCompareLabels(cachedNpmPod.Labels, newPodObj.Labels)

	newPodMetadata := dataplane.NewPodMetadata(podKey, newPodObj.Status.PodIP, newPodObj.Spec.NodeName)
	// todo: verify pulling nodename from newpod,
	// if a pod is getting deleted, we do not have to cleanup policies, so it is okay to pass in wrong nodename
	cachedPodMetadata := dataplane.NewPodMetadata(podKey, cachedNpmPod.PodIP, newPodMetadata.NodeName)
	// Delete the pod from its label's ipset.
	for _, removeIPSetName := range deleteFromIPSets {
		klog.Infof("Deleting pod %s (ip : %s) from ipset %s", podKey, cachedNpmPod.PodIP, removeIPSetName)

		var toRemoveSet *ipsets.IPSetMetadata
		if util.IsKeyValueLabelSetName(removeIPSetName) {
			toRemoveSet = ipsets.NewIPSetMetadata(removeIPSetName, ipsets.KeyValueLabelOfPod)
		} else {
			toRemoveSet = ipsets.NewIPSetMetadata(removeIPSetName, ipsets.KeyLabelOfPod)
		}
		if err = c.dp.RemoveFromSets([]*ipsets.IPSetMetadata{toRemoveSet}, cachedPodMetadata); err != nil {
			return metrics.UpdateOp, fmt.Errorf("[syncAddAndUpdatePod] Error: failed to delete pod from label ipset with err: %w", err)
		}
		// {IMPORTANT} The order of compared list will be key and then key+val. NPM should only append after both key
		// key + val ipsets are worked on. 0th index will be key and 1st index will be value of the label
		removedLabelKey, removedLabelValue := util.GetLabelKVFromSet(removeIPSetName)
		if removedLabelValue != "" {
			cachedNpmPod.RemoveLabelsWithKey(removedLabelKey)
		}
	}

	// Add the pod to its label's ipset.
	for _, addIPSetName := range addToIPSets {

		klog.Infof("Creating ipset %s if it doesn't already exist", addIPSetName)

		var toAddSet *ipsets.IPSetMetadata
		if util.IsKeyValueLabelSetName(addIPSetName) {
			toAddSet = ipsets.NewIPSetMetadata(addIPSetName, ipsets.KeyValueLabelOfPod)
		} else {
			toAddSet = ipsets.NewIPSetMetadata(addIPSetName, ipsets.KeyLabelOfPod)
		}

		klog.Infof("Adding pod %s (ip : %s) to ipset %s", podKey, newPodObj.Status.PodIP, addIPSetName)
		if err = c.dp.AddToSets([]*ipsets.IPSetMetadata{toAddSet}, newPodMetadata); err != nil {
			return metrics.UpdateOp, fmt.Errorf("[syncAddAndUpdatePod] Error: failed to add pod to label ipset with err: %w", err)
		}
		// {IMPORTANT} Same as above order is assumed to be key and then key+val. NPM should only append to existing labels
		// only after both ipsets for a given label's key value pair are added successfully
		// (TODO) will need to remove this ordering dependency
		addedLabelKey, addedLabelValue := util.GetLabelKVFromSet(addIPSetName)
		if addedLabelValue != "" {
			cachedNpmPod.AppendLabels(map[string]string{addedLabelKey: addedLabelValue}, common.AppendToExistingLabels)
		}
	}
	// This will ensure after all labels are worked on to overwrite. This way will reduce any bugs introduced above
	// If due to ordering issue the above deleted and added labels are not correct,
	// this below appendLabels will help ensure correct state in cache for all successful ops.
	cachedNpmPod.AppendLabels(newPodObj.Labels, common.ClearExistingLabels)

	// (TODO): optimize named port addition and deletions.
	// named ports are mostly static once configured in todays usage pattern
	// so keeping this simple by deleting all and re-adding
	newPodPorts := common.GetContainerPortList(newPodObj)
	if !reflect.DeepEqual(cachedNpmPod.ContainerPorts, newPodPorts) {
		// Delete cached pod's named ports from its ipset.
		if err = c.manageNamedPortIpsets(
			cachedNpmPod.ContainerPorts, podKey, cachedNpmPod.PodIP, "", deleteNamedPort); err != nil {
			return metrics.UpdateOp, fmt.Errorf("[syncAddAndUpdatePod] Error: failed to delete pod from named port ipset with err: %w", err)
		}
		// Since portList ipset deletion is successful, NPM can remove cachedContainerPorts
		cachedNpmPod.RemoveContainerPorts()

		// Add new pod's named ports from its ipset.
		if err = c.manageNamedPortIpsets(newPodPorts, podKey, newPodObj.Status.PodIP, newPodObj.Spec.NodeName, addNamedPort); err != nil {
			return metrics.UpdateOp, fmt.Errorf("[syncAddAndUpdatePod] Error: failed to add pod to named port ipset with err: %w", err)
		}
		cachedNpmPod.AppendContainerPorts(newPodObj)
	}
	cachedNpmPod.UpdateNpmPodAttributes(newPodObj)

	return metrics.UpdateOp, nil
}

// cleanUpDeletedPod cleans up all ipset associated with this pod
func (c *PodController) cleanUpDeletedPod(cachedNpmPodKey string) error {
	klog.Infof("[cleanUpDeletedPod] deleting Pod with key %s", cachedNpmPodKey)
	// If cached npmPod does not exist, return nil
	cachedNpmPod, exist := c.podMap[cachedNpmPodKey]
	if !exist {
		return nil
	}

	var err error
	cachedPodMetadata := dataplane.NewPodMetadata(cachedNpmPodKey, cachedNpmPod.PodIP, "")
	// Delete the pod from its namespace's ipset.
	// note: NodeName empty is not going to call update pod
	if err = c.dp.RemoveFromSets(
		[]*ipsets.IPSetMetadata{ipsets.NewIPSetMetadata(cachedNpmPod.Namespace, ipsets.Namespace)},
		cachedPodMetadata); err != nil {
		return fmt.Errorf("[cleanUpDeletedPod] Error: failed to delete pod from namespace ipset with err: %w", err)
	}

	// Get lists of podLabelKey and podLabelKey + podLavelValue ,and then start deleting them from ipsets
	for labelKey, labelVal := range cachedNpmPod.Labels {
		labelKeyValue := util.GetIpSetFromLabelKV(labelKey, labelVal)
		klog.Infof("Deleting pod %s (ip : %s) from ipsets %s and %s", cachedNpmPodKey, cachedNpmPod.PodIP, labelKey, labelKeyValue)
		if err = c.dp.RemoveFromSets(
			[]*ipsets.IPSetMetadata{
				ipsets.NewIPSetMetadata(labelKey, ipsets.KeyLabelOfPod),
				ipsets.NewIPSetMetadata(labelKeyValue, ipsets.KeyValueLabelOfPod),
			},
			cachedPodMetadata); err != nil {
			return fmt.Errorf("[cleanUpDeletedPod] Error: failed to delete pod from label ipset with err: %w", err)
		}
		cachedNpmPod.RemoveLabelsWithKey(labelKey)
	}

	// Delete pod's named ports from its ipset. Need to pass true in the manageNamedPortIpsets function call
	if err = c.manageNamedPortIpsets(
		cachedNpmPod.ContainerPorts, cachedNpmPodKey, cachedNpmPod.PodIP, "", deleteNamedPort); err != nil {
		return fmt.Errorf("[cleanUpDeletedPod] Error: failed to delete pod from named port ipset with err: %w", err)
	}

	delete(c.podMap, cachedNpmPodKey)
	return nil
}

// manageNamedPortIpsets helps with adding or deleting Pod namedPort IPsets.
func (c *PodController) manageNamedPortIpsets(portList []corev1.ContainerPort, podKey,
	podIP, nodeName string, namedPortOperation NamedPortOperation) error {
	if util.IsWindowsDP() {
		// NOTE: if we support namedport operations, need to be careful of implications of including the node name in the pod metadata below
		// since we say the node name is "" in cleanUpDeletedPod
		klog.Warningf("Windows Dataplane does not support NamedPort operations. Operation: %s portList is %+v", namedPortOperation, portList)
		return nil
	}
	for _, port := range portList {
		klog.Infof("port is %+v", port)
		if port.Name == "" {
			continue
		}

		// K8s guarantees port.Protocol has "TCP", "UDP", or "SCTP" if the field exists.
		var protocol string
		if len(port.Protocol) != 0 {
			// without adding ":" after protocol, ipset complains.
			protocol = fmt.Sprintf("%s:", port.Protocol)
		}

		namedPortIpsetEntry := fmt.Sprintf("%s,%s%d", podIP, protocol, port.ContainerPort)

		// nodename in NewPodMetadata is nil so UpdatePod is ignored
		podMetadata := dataplane.NewPodMetadata(podKey, namedPortIpsetEntry, nodeName)
		switch namedPortOperation {
		case deleteNamedPort:
			if err := c.dp.RemoveFromSets([]*ipsets.IPSetMetadata{ipsets.NewIPSetMetadata(port.Name, ipsets.NamedPorts)}, podMetadata); err != nil {
				return fmt.Errorf("failed to remove from set when deleting named port with err %w", err)
			}
		case addNamedPort:
			if err := c.dp.AddToSets([]*ipsets.IPSetMetadata{ipsets.NewIPSetMetadata(port.Name, ipsets.NamedPorts)}, podMetadata); err != nil {
				return fmt.Errorf("failed to add to set when deleting named port with err %w", err)
			}
		}
	}

	return nil
}

// isCompletePod evaluates whether this pod is completely in terminated states,
// which means pod is gracefully shutdown.
func isCompletePod(podObj *corev1.Pod) bool {
	// DeletionTimestamp and DeletionGracePeriodSeconds in pod are not nil,
	// which means pod is expected to be deleted and
	// DeletionGracePeriodSeconds value is zero, which means the pod is gracefully terminated.
	if podObj.DeletionTimestamp != nil && podObj.DeletionGracePeriodSeconds != nil && *podObj.DeletionGracePeriodSeconds == 0 {
		return true
	}

	// K8s categorizes Succeeded and Failed pods as a terminated pod and will not restart them.
	// So NPM will ignorer adding these pods
	// TODO(jungukcho): what are the values of DeletionTimestamp and podObj.DeletionGracePeriodSeconds
	// in either below status?
	if podObj.Status.Phase == corev1.PodSucceeded || podObj.Status.Phase == corev1.PodFailed {
		return true
	}
	return false
}

func hasValidPodIP(podObj *corev1.Pod) bool {
	return len(podObj.Status.PodIP) > 0
}

func isHostNetworkPod(podObj *corev1.Pod) bool {
	return podObj.Spec.HostNetwork
}
