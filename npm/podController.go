// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/Azure/azure-container-networking/npm/ipsm"
	"github.com/Azure/azure-container-networking/npm/metrics"
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

type NpmPod struct {
	Name           string
	Namespace      string
	PodIP          string
	Labels         map[string]string
	ContainerPorts []corev1.ContainerPort
	Phase          corev1.PodPhase
}

func newNpmPod(podObj *corev1.Pod) *NpmPod {
	return &NpmPod{
		Name:           podObj.ObjectMeta.Name,
		Namespace:      podObj.ObjectMeta.Namespace,
		PodIP:          podObj.Status.PodIP,
		Labels:         make(map[string]string),
		ContainerPorts: []corev1.ContainerPort{},
		Phase:          podObj.Status.Phase,
	}
}

func (nPod *NpmPod) appendLabels(new map[string]string, clear LabelAppendOperation) {
	if clear {
		nPod.Labels = make(map[string]string)
	}
	for k, v := range new {
		nPod.Labels[k] = v
	}
}

func (nPod *NpmPod) removeLabelsWithKey(key string) {
	delete(nPod.Labels, key)
}

func (nPod *NpmPod) appendContainerPorts(podObj *corev1.Pod) {
	nPod.ContainerPorts = getContainerPortList(podObj)
}

func (nPod *NpmPod) removeContainerPorts() {
	nPod.ContainerPorts = []corev1.ContainerPort{}
}

// This function can be expanded to other attribs if needed
func (nPod *NpmPod) updateNpmPodAttributes(podObj *corev1.Pod) {
	if nPod.Phase != podObj.Status.Phase {
		nPod.Phase = podObj.Status.Phase
	}
}

type podController struct {
	podLister corelisters.PodLister
	workqueue workqueue.RateLimitingInterface
	ipsMgr    *ipsm.IpsetManager
	podMap    map[string]*NpmPod // Key is <nsname>/<podname>
	sync.Mutex
	npmNamespaceCache *npmNamespaceCache
}

func NewPodController(podInformer coreinformer.PodInformer, ipsMgr *ipsm.IpsetManager, npmNamespaceCache *npmNamespaceCache) *podController {
	podController := &podController{
		podLister:         podInformer.Lister(),
		workqueue:         workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Pods"),
		ipsMgr:            ipsMgr,
		podMap:            make(map[string]*NpmPod),
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

func (c *podController) MarshalJSON() ([]byte, error) {
	c.Lock()
	defer c.Unlock()

	podMapRaw, err := json.Marshal(c.podMap)
	if err != nil {
		return nil, errors.Errorf("failed to marshal podMap due to %v", err)
	}

	return podMapRaw, nil
}

func (c *podController) lengthOfPodMap() int {
	return len(c.podMap)
}

// needSync filters the event if the event is not required to handle
func (c *podController) needSync(eventType string, obj interface{}) (string, bool) {
	needSync := false
	var key string

	podObj, ok := obj.(*corev1.Pod)
	if !ok {
		metrics.SendErrorLogAndMetric(util.PodID, "ADD Pod: Received unexpected object type: %v", obj)
		return key, needSync
	}

	klog.Infof("[POD %s EVENT] for %s in %s", eventType, podObj.Name, podObj.Namespace)

	if !hasValidPodIP(podObj) {
		return key, needSync
	}

	if isHostNetworkPod(podObj) {
		klog.Infof("[POD %s EVENT] HostNetwork POD IGNORED: [%s/%s/%s/%+v%s]",
			eventType, podObj.GetObjectMeta().GetUID(), podObj.Namespace, podObj.Name, podObj.Labels, podObj.Status.PodIP)
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

func (c *podController) addPod(obj interface{}) {
	key, needSync := c.needSync("ADD", obj)
	if !needSync {
		return
	}
	podObj, _ := obj.(*corev1.Pod)

	// If newPodObj status is either corev1.PodSucceeded or corev1.PodFailed or DeletionTimestamp is set, do not need to add it into workqueue.
	if isCompletePod(podObj) {
		return
	}

	c.workqueue.Add(key)
}

func (c *podController) updatePod(old, new interface{}) {
	key, needSync := c.needSync("UPDATE", new)
	if !needSync {
		klog.Infof("[POD UPDATE EVENT] No need to sync this pod")
		return
	}

	// needSync checked validation of casting newPod.
	newPod, _ := new.(*corev1.Pod)
	oldPod, ok := old.(*corev1.Pod)
	if ok {
		if oldPod.ResourceVersion == newPod.ResourceVersion {
			// Periodic resync will send update events for all known pods.
			// Two different versions of the same pods will always have different RVs.
			klog.Infof("[POD UPDATE EVENT] Two pods have the same RVs")
			return
		}
	}

	c.workqueue.Add(key)
}

func (c *podController) deletePod(obj interface{}) {
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
		klog.Infof("[POD DELETE EVENT] HostNetwork POD IGNORED: [%s/%s/%s/%+v%s]", podObj.UID, podObj.Namespace, podObj.Name, podObj.Labels, podObj.Status.PodIP)
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

func (c *podController) Run(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()

	klog.Infof("Starting Pod worker")
	go wait.Until(c.runWorker, time.Second, stopCh)

	klog.Info("Started Pod workers")
	<-stopCh
	klog.Info("Shutting down Pod workers")
}

func (c *podController) runWorker() {
	for c.processNextWorkItem() {
	}
}

func (c *podController) processNextWorkItem() bool {
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
		// Run the syncPod, passing it the namespace/name string of the
		// Pod resource to be synced.
		if err := c.syncPod(key); err != nil {
			// Put the item back on the workqueue to handle any transient errors.
			c.workqueue.AddRateLimited(key)
			metrics.SendErrorLogAndMetric(util.PodID, "[podController processNextWorkItem] Error: failed to syncPod %s. Requeuing with err: %v", key, err)
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
		return true
	}

	return true
}

// syncPod compares the actual state with the desired, and attempts to converge the two.
func (c *podController) syncPod(key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	// Get the Pod resource with this namespace/name
	pod, err := c.podLister.Pods(namespace).Get(name)

	c.Lock()
	defer c.Unlock()

	if err != nil {
		if apierrors.IsNotFound(err) {
			klog.Infof("pod %s not found, may be it is deleted", key)
			// cleanUpDeletedPod will check if the pod exists in cache, if it does then proceeds with deletion
			// if it does not exists, then event will be no-op
			err = c.cleanUpDeletedPod(key)
			if err != nil {
				// need to retry this cleaning-up process
				return fmt.Errorf("Error: %v when pod is not found\n", err)
			}
			return err
		}

		return err
	}

	// If newPodObj status is either corev1.PodSucceeded or corev1.PodFailed or DeletionTimestamp is set, start clean-up the lastly applied states.
	if isCompletePod(pod) {
		if err = c.cleanUpDeletedPod(key); err != nil {
			return fmt.Errorf("Error: %v when when pod is in completed state.\n", err)
		}
		return nil
	}

	cachedNpmPod, npmPodExists := c.podMap[key]
	if npmPodExists {
		// if pod does not have different states against lastly applied states stored in cachedNpmPod,
		// podController does not need to reconcile this update.
		// in this updatePod event, newPod was updated with states which PodController does not need to reconcile.
		if isInvalidPodUpdate(cachedNpmPod, pod) {
			return nil
		}
	}

	err = c.syncAddAndUpdatePod(pod)
	if err != nil {
		return fmt.Errorf("Failed to sync pod due to %v\n", err)
	}

	return nil
}

func (c *podController) syncAddedPod(podObj *corev1.Pod) error {
	klog.Infof("POD CREATING: [%s%s/%s/%s%+v%s]", string(podObj.GetUID()), podObj.Namespace,
		podObj.Name, podObj.Spec.NodeName, podObj.Labels, podObj.Status.PodIP)

	var err error
	podNs := util.GetNSNameWithPrefix(podObj.Namespace)
	podKey, _ := cache.MetaNamespaceKeyFunc(podObj)
	// Add the pod ip information into namespace's ipset.
	klog.Infof("Adding pod %s to ipset %s", podObj.Status.PodIP, podNs)
	if err = c.ipsMgr.AddToSet(podNs, podObj.Status.PodIP, util.IpsetNetHashFlag, podKey); err != nil {
		return fmt.Errorf("[syncAddedPod] Error: failed to add pod to namespace ipset with err: %v", err)
	}

	// Create npmPod and add it to the podMap
	npmPodObj := newNpmPod(podObj)
	c.podMap[podKey] = npmPodObj

	// Get lists of podLabelKey and podLabelKey + podLavelValue ,and then start adding them to ipsets.
	for labelKey, labelVal := range podObj.Labels {
		klog.Infof("Adding pod %s to ipset %s", npmPodObj.PodIP, labelKey)
		if err = c.ipsMgr.AddToSet(labelKey, npmPodObj.PodIP, util.IpsetNetHashFlag, podKey); err != nil {
			return fmt.Errorf("[syncAddedPod] Error: failed to add pod to label ipset with err: %v", err)
		}

		podIPSetName := util.GetIpSetFromLabelKV(labelKey, labelVal)
		klog.Infof("Adding pod %s to ipset %s", npmPodObj.PodIP, podIPSetName)
		if err = c.ipsMgr.AddToSet(podIPSetName, npmPodObj.PodIP, util.IpsetNetHashFlag, podKey); err != nil {
			return fmt.Errorf("[syncAddedPod] Error: failed to add pod to label ipset with err: %v", err)
		}
		npmPodObj.appendLabels(map[string]string{labelKey: labelVal}, AppendToExistingLabels)
	}

	// Add pod's named ports from its ipset.
	klog.Infof("Adding named port ipsets")
	containerPorts := getContainerPortList(podObj)
	if err = c.manageNamedPortIpsets(containerPorts, podKey, npmPodObj.PodIP, addNamedPort); err != nil {
		return fmt.Errorf("[syncAddedPod] Error: failed to add pod to named port ipset with err: %v", err)
	}
	npmPodObj.appendContainerPorts(podObj)

	return nil
}

// syncAddAndUpdatePod handles updating pod ip in its label's ipset.
func (c *podController) syncAddAndUpdatePod(newPodObj *corev1.Pod) error {
	var err error
	newPodObjNs := util.GetNSNameWithPrefix(newPodObj.Namespace)

	// lock before using nsMap since nsMap is shared with namespace controller
	c.npmNamespaceCache.Lock()
	if _, exists := c.npmNamespaceCache.nsMap[newPodObjNs]; !exists {
		// Create ipset related to namespace which this pod belong to if it does not exist.
		if err = c.ipsMgr.CreateSet(newPodObjNs, []string{util.IpsetNetHashFlag}); err != nil {
			c.npmNamespaceCache.Unlock()
			return fmt.Errorf("[syncAddAndUpdatePod] Error: failed to create ipset for namespace %s with err: %v", newPodObjNs, err)
		}

		if err = c.ipsMgr.AddToList(util.KubeAllNamespacesFlag, newPodObjNs); err != nil {
			c.npmNamespaceCache.Unlock()
			return fmt.Errorf("[syncAddAndUpdatePod] Error: failed to add %s to all-namespace ipset list with err: %v", newPodObjNs, err)
		}

		// Add namespace object into NsMap cache only when two ipset operations are successful.
		npmNs := newNs(newPodObjNs)
		c.npmNamespaceCache.nsMap[newPodObjNs] = npmNs
	}
	c.npmNamespaceCache.Unlock()

	podKey, _ := cache.MetaNamespaceKeyFunc(newPodObj)
	cachedNpmPod, exists := c.podMap[podKey]
	klog.Infof("[syncAddAndUpdatePod] updating Pod with key %s", podKey)
	// No cached npmPod exists. start adding the pod in a cache
	if !exists {
		if err = c.syncAddedPod(newPodObj); err != nil {
			return err
		}
		return nil
	}

	// Dealing with "updatePod" event - Compare last applied states against current Pod states
	// There are two possiblities for npmPodObj and newPodObj
	// #1 case The same object with the same UID and the same key (namespace + name)
	// #2 case Different objects with different UID, but the same key (namespace + name) due to missing some events for the old object

	// Dealing with #2 pod update event, the IP addresses of cached npmPod and newPodObj are different
	// NPM should clean up existing references of cached pod obj and its IP.
	// then, re-add new pod obj.
	if cachedNpmPod.PodIP != newPodObj.Status.PodIP {
		klog.Infof("Pod (Namespace:%s, Name:%s, newUid:%s), has cachedPodIp:%s which is different from PodIp:%s",
			newPodObj.Namespace, newPodObj.Name, string(newPodObj.UID), cachedNpmPod.PodIP, newPodObj.Status.PodIP)

		klog.Infof("Deleting cached Pod with key:%s first due to IP Mistmatch", podKey)
		if err = c.cleanUpDeletedPod(podKey); err != nil {
			return err
		}

		klog.Infof("Adding back Pod with key:%s after IP Mistmatch", podKey)
		if err = c.syncAddedPod(newPodObj); err != nil {
			return err
		}

		return nil
	}

	// Dealing with #1 pod update event, the IP addresses of cached npmPod and newPodObj are same
	// If no change in labels, then GetIPSetListCompareLabels will return empty list.
	// Otherwise it returns list of deleted PodIP from cached pod's labels and list of added PodIp from new pod's labels
	addToIPSets, deleteFromIPSets := util.GetIPSetListCompareLabels(cachedNpmPod.Labels, newPodObj.Labels)

	// Delete the pod from its label's ipset.
	for _, podIPSetName := range deleteFromIPSets {
		klog.Infof("Deleting pod %s from ipset %s", cachedNpmPod.PodIP, podIPSetName)
		if err = c.ipsMgr.DeleteFromSet(podIPSetName, cachedNpmPod.PodIP, podKey); err != nil {
			return fmt.Errorf("[syncAddAndUpdatePod] Error: failed to delete pod from label ipset with err: %v", err)
		}
		// {IMPORTANT} The order of compared list will be key and then key+val. NPM should only append after both key
		// key + val ipsets are worked on. 0th index will be key and 1st index will be value of the label
		removedLabelKey, removedLabelValue := util.GetLabelKVFromSet(podIPSetName)
		if removedLabelValue != "" {
			cachedNpmPod.removeLabelsWithKey(removedLabelKey)
		}
	}

	// Add the pod to its label's ipset.
	for _, addIPSetName := range addToIPSets {
		klog.Infof("Adding pod %s to ipset %s", newPodObj.Status.PodIP, addIPSetName)
		if err = c.ipsMgr.AddToSet(addIPSetName, newPodObj.Status.PodIP, util.IpsetNetHashFlag, podKey); err != nil {
			return fmt.Errorf("[syncAddAndUpdatePod] Error: failed to add pod to label ipset with err: %v", err)
		}
		// {IMPORTANT} Same as above order is assumed to be key and then key+val. NPM should only append to existing labels
		// only after both ipsets for a given label's key value pair are added successfully
		// (TODO) will need to remove this ordering dependency
		addedLabelKey, addedLabelValue := util.GetLabelKVFromSet(addIPSetName)
		if addedLabelValue != "" {
			cachedNpmPod.appendLabels(map[string]string{addedLabelKey: addedLabelValue}, AppendToExistingLabels)
		}
	}
	// This will ensure after all labels are worked on to overwrite. This way will reduce any bugs introduced above
	// If due to ordering issue the above deleted and added labels are not correct,
	// this below appendLabels will help ensure correct state in cache for all successful ops.
	cachedNpmPod.appendLabels(newPodObj.Labels, ClearExistingLabels)

	// (TODO): optimize named port addition and deletions.
	// named ports are mostly static once configured in todays usage pattern
	// so keeping this simple by deleting all and re-adding
	newPodPorts := getContainerPortList(newPodObj)
	if !reflect.DeepEqual(cachedNpmPod.ContainerPorts, newPodPorts) {
		// Delete cached pod's named ports from its ipset.
		if err = c.manageNamedPortIpsets(
			cachedNpmPod.ContainerPorts, podKey, cachedNpmPod.PodIP, deleteNamedPort); err != nil {
			return fmt.Errorf("[syncAddAndUpdatePod] Error: failed to delete pod from named port ipset with err: %v", err)
		}
		// Since portList ipset deletion is successful, NPM can remove cachedContainerPorts
		cachedNpmPod.removeContainerPorts()

		// Add new pod's named ports from its ipset.
		if err = c.manageNamedPortIpsets(newPodPorts, podKey, newPodObj.Status.PodIP, addNamedPort); err != nil {
			return fmt.Errorf("[syncAddAndUpdatePod] Error: failed to add pod to named port ipset with err: %v", err)
		}
		cachedNpmPod.appendContainerPorts(newPodObj)
	}
	cachedNpmPod.updateNpmPodAttributes(newPodObj)

	return nil
}

// cleanUpDeletedPod cleans up all ipset associated with this pod
func (c *podController) cleanUpDeletedPod(cachedNpmPodKey string) error {
	klog.Infof("[cleanUpDeletedPod] deleting Pod with key %s", cachedNpmPodKey)
	// If cached npmPod does not exist, return nil
	cachedNpmPod, exist := c.podMap[cachedNpmPodKey]
	if !exist {
		return nil
	}

	podNs := util.GetNSNameWithPrefix(cachedNpmPod.Namespace)
	var err error
	// Delete the pod from its namespace's ipset.
	if err = c.ipsMgr.DeleteFromSet(podNs, cachedNpmPod.PodIP, cachedNpmPodKey); err != nil {
		return fmt.Errorf("[cleanUpDeletedPod] Error: failed to delete pod from namespace ipset with err: %v", err)
	}

	// Get lists of podLabelKey and podLabelKey + podLavelValue ,and then start deleting them from ipsets
	for labelKey, labelVal := range cachedNpmPod.Labels {
		klog.Infof("Deleting pod %s from ipset %s", cachedNpmPod.PodIP, labelKey)
		if err = c.ipsMgr.DeleteFromSet(labelKey, cachedNpmPod.PodIP, cachedNpmPodKey); err != nil {
			return fmt.Errorf("[cleanUpDeletedPod] Error: failed to delete pod from label ipset with err: %v", err)
		}

		podIPSetName := util.GetIpSetFromLabelKV(labelKey, labelVal)
		klog.Infof("Deleting pod %s from ipset %s", cachedNpmPod.PodIP, podIPSetName)
		if err = c.ipsMgr.DeleteFromSet(podIPSetName, cachedNpmPod.PodIP, cachedNpmPodKey); err != nil {
			return fmt.Errorf("[cleanUpDeletedPod] Error: failed to delete pod from label ipset with err: %v", err)
		}
		cachedNpmPod.removeLabelsWithKey(labelKey)
	}

	// Delete pod's named ports from its ipset. Need to pass true in the manageNamedPortIpsets function call
	if err = c.manageNamedPortIpsets(
		cachedNpmPod.ContainerPorts, cachedNpmPodKey, cachedNpmPod.PodIP, deleteNamedPort); err != nil {
		return fmt.Errorf("[cleanUpDeletedPod] Error: failed to delete pod from named port ipset with err: %v", err)
	}

	delete(c.podMap, cachedNpmPodKey)
	return nil
}

// manageNamedPortIpsets helps with adding or deleting Pod namedPort IPsets.
func (c *podController) manageNamedPortIpsets(portList []corev1.ContainerPort, podKey string,
	podIP string, namedPortOperation NamedPortOperation) error {
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

		namedPort := util.NamedPortIPSetPrefix + port.Name
		namedPortIpsetEntry := fmt.Sprintf("%s,%s%d", podIP, protocol, port.ContainerPort)
		switch namedPortOperation {
		case deleteNamedPort:
			if err := c.ipsMgr.DeleteFromSet(namedPort, namedPortIpsetEntry, podKey); err != nil {
				return err
			}
		case addNamedPort:
			if err := c.ipsMgr.AddToSet(namedPort, namedPortIpsetEntry, util.IpsetIPPortHashFlag, podKey); err != nil {
				return err
			}
		}
	}

	return nil
}

func isCompletePod(podObj *corev1.Pod) bool {
	if podObj.DeletionTimestamp != nil {
		return true
	}

	// K8s categorizes Succeeded and Failed pods as a terminated pod and will not restart them
	// So NPM will ignorer adding these pods
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

func getContainerPortList(podObj *corev1.Pod) []corev1.ContainerPort {
	portList := []corev1.ContainerPort{}
	for _, container := range podObj.Spec.Containers {
		portList = append(portList, container.Ports...)
	}
	return portList
}

// (TODO): better naming?
func isInvalidPodUpdate(npmPod *NpmPod, newPodObj *corev1.Pod) bool {
	return npmPod.Namespace == newPodObj.ObjectMeta.Namespace &&
		npmPod.Name == newPodObj.ObjectMeta.Name &&
		npmPod.Phase == newPodObj.Status.Phase &&
		npmPod.PodIP == newPodObj.Status.PodIP &&
		newPodObj.ObjectMeta.DeletionTimestamp == nil &&
		newPodObj.ObjectMeta.DeletionGracePeriodSeconds == nil &&
		reflect.DeepEqual(npmPod.Labels, newPodObj.ObjectMeta.Labels) &&
		reflect.DeepEqual(npmPod.ContainerPorts, getContainerPortList(newPodObj))
}
