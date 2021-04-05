// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"fmt"
	"reflect"
	"time"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/ipsm"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/util"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	coreinformer "k8s.io/client-go/informers/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

// NamedPortOperation decides opeartion (e.g., delete or add) for named port ipset in manageNamedPortIpsets
type NamedPortOperation string

const (
	DeleteNamedPortIpsets NamedPortOperation = "del"
	AddNamedPortIpsets    NamedPortOperation = "add"
)

type NpmPod struct {
	Name            string
	Namespace       string
	NodeName        string
	PodUID          string
	PodIP           string
	IsHostNetwork   bool
	PodIPs          []corev1.PodIP
	Labels          map[string]string
	ContainerPorts  []corev1.ContainerPort
	ResourceVersion uint64 // Pod Resource Version
	Phase           corev1.PodPhase
}

func newNpmPod(podObj *corev1.Pod) *NpmPod {
	// (TODO): handle error?
	rv := util.ParseResourceVersion(podObj.GetObjectMeta().GetResourceVersion())
	return &NpmPod{
		Name:            podObj.ObjectMeta.Name,
		Namespace:       podObj.ObjectMeta.Namespace,
		NodeName:        podObj.Spec.NodeName,
		PodUID:          string(podObj.ObjectMeta.UID),
		PodIP:           podObj.Status.PodIP,
		PodIPs:          podObj.Status.PodIPs,
		IsHostNetwork:   podObj.Spec.HostNetwork,
		Labels:          podObj.Labels,
		ContainerPorts:  getContainerPortList(podObj),
		ResourceVersion: rv,
		Phase:           podObj.Status.Phase,
	}
}

// getPodObjFromNpmObj returns a new pod object based on NpmPod
func (nPod *NpmPod) getPodObjFromNpmPodObj() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nPod.Name,
			Namespace: nPod.Namespace,
			Labels:    nPod.Labels,
			UID:       types.UID(nPod.PodUID),
		},
		Status: corev1.PodStatus{
			Phase:  nPod.Phase,
			PodIP:  nPod.PodIP,
			PodIPs: nPod.PodIPs,
		},
		Spec: corev1.PodSpec{
			HostNetwork: nPod.IsHostNetwork,
			NodeName:    nPod.NodeName,
			Containers: []corev1.Container{
				corev1.Container{
					Ports: nPod.ContainerPorts,
				},
			},
		},
	}
}

type podController struct {
	clientset       kubernetes.Interface
	podLister       corelisters.PodLister
	podListerSynced cache.InformerSynced
	workqueue       workqueue.RateLimitingInterface
	//podCache        map[string]*corev1.Pod
	// (TODO): podController does not need to have whole NetworkPolicyManager pointer. Need to improve it
	npMgr *NetworkPolicyManager
}

func NewPodController(podInformer coreinformer.PodInformer, clientset kubernetes.Interface, npMgr *NetworkPolicyManager) *podController {
	podController := &podController{
		clientset:       clientset,
		podLister:       podInformer.Lister(),
		podListerSynced: podInformer.Informer().HasSynced,
		workqueue:       workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Pods"),
		npMgr:           npMgr,
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

// needSync filters the event if the event is not required to handle
func (c *podController) needSync(eventType string, obj interface{}) (string, bool) {
	needSync := false
	var key string

	podObj, ok := obj.(*corev1.Pod)
	if !ok {
		metrics.SendErrorLogAndMetric(util.NpmID, "ADD Pod: Received unexpected object type: %v", obj)
		return key, needSync
	}

	log.Logf("[POD %s EVENT] for %s in %s", eventType, podObj.Name, podObj.Namespace)

	if !hasValidPodIP(podObj) {
		return key, needSync
	}

	if isHostNetworkPod(podObj) {
		log.Logf("[POD %s EVENT] HostNetwork POD IGNORED: [%s/%s/%s/%+v%s]",
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
		log.Logf("[POD ADD EVENT] No need to sync this pod")
		return
	}
	// K8s categorizes Succeeded and Failed pods as a terminated pod and will not restart them
	// So NPM will ignorer adding these pods
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
		log.Logf("[POD UPDATE EVENT] No need to sync this pod")
		return
	}

	// needSync checked validation of casting newPod.
	newPod, _ := new.(*corev1.Pod)
	oldPod, ok := old.(*corev1.Pod)
	if ok {
		if oldPod.ResourceVersion == newPod.ResourceVersion {
			// Periodic resync will send update events for all known pods.
			// Two different versions of the same pods will always have different RVs.
			log.Logf("[POD UPDATE EVENT] Two pods have the same RVs")
			return
		}
	}

	c.npMgr.Lock()
	defer c.npMgr.Unlock()
	cachedNpmPodObj, npmPodExists := c.npMgr.PodMap[key]
	if npmPodExists {
		// if newPod does not have different states against lastly applied states stored in cachedNpmPodObj,
		// podController does not need to reconcile this update.
		// in this updatePod event, newPod was updated with states which PodController does not need to reconcile.
		if isInvalidPodUpdate(cachedNpmPodObj, newPod) {
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
			metrics.SendErrorLogAndMetric(util.NpmID, "[POD DELETE EVENT] Pod: Received unexpected object type: %v", obj)
			return
		}

		if podObj, ok = tombstone.Obj.(*corev1.Pod); !ok {
			metrics.SendErrorLogAndMetric(util.NpmID, "[POD DELETE EVENT] Pod: Received unexpected object type (error decoding object tombstone, invalid type): %v", obj)
			return
		}
	}

	log.Logf("[POD DELETE EVENT] for %s in %s", podObj.Name, podObj.Namespace)
	if isHostNetworkPod(podObj) {
		log.Logf("[POD DELETE EVENT] HostNetwork POD IGNORED: [%s/%s/%s/%+v%s]", podObj.UID, podObj.Namespace, podObj.Name, podObj.Labels, podObj.Status.PodIP)
		return
	}

	var err error
	var key string
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		metrics.SendErrorLogAndMetric(util.PodID, "[POD DELETE EVENT] Error: podKey is empty for %s pod in %s with UID %s",
			podObj.ObjectMeta.Name, util.GetNSNameWithPrefix(podObj.Namespace), podObj.UID)
		return
	}

	// (TODO): Reduce scope of lock later
	c.npMgr.Lock()
	defer c.npMgr.Unlock()

	// If this pod object is not in the PodMap, we do not need to clean-up states for this pod
	// since podController did not apply for any states for this pod
	_, npmPodExists := c.npMgr.PodMap[key]
	if !npmPodExists {
		return
	}

	c.workqueue.Add(key)
}

func (c *podController) Run(threadiness int, stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()

	log.Logf("Starting Pod %d worker(s)", threadiness)
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	log.Logf("Started Pod workers")
	<-stopCh
	log.Logf("Shutting down Pod workers")

	return nil
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
		// (TODO): may consider using "c.queue.AddAfter(key, *requeueAfter)" according to error type later
		if err := c.syncPod(key); err != nil {
			// Put the item back on the workqueue to handle any transient errors.
			c.workqueue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.workqueue.Forget(obj)
		log.Logf("Successfully synced '%s'", key)
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
	// (TODO): Reduce scope of lock later
	c.npMgr.Lock()
	defer c.npMgr.Unlock()

	if err != nil {
		if errors.IsNotFound(err) {
			log.Logf("pod %s not found, may be it is deleted", key)
			cachedNpmPodObj, exist := c.npMgr.PodMap[key]
			// if the npmPod does not exists, we do not need to clean up process and retry it
			if !exist {
				return nil
			}

			// Found the npmPod object from PodMap local cache and start cleaning up processes
			err = c.cleanUpDeletedPod(cachedNpmPodObj.getPodObjFromNpmPodObj())
			if err != nil {
				// need to retry this cleaning-up process
				metrics.SendErrorLogAndMetric(util.PodID, "Error: %v when pod is not found", err)
				return fmt.Errorf("Error: %v when pod is not found\n", err)
			}
			return err
		}

		return err
	}

	// If newPodObj status is either corev1.PodSucceeded or corev1.PodFailed or DeletionTimestamp is set, start clean-up the lastly applied states.
	if isCompletePod(pod) {
		if err = c.cleanUpDeletedPod(pod); err != nil {
			metrics.SendErrorLogAndMetric(util.PodID, "Error: %v when pod is in completed state.", err)
			return fmt.Errorf("Error: %v when when pod is in completed state.\n", err)
		}
		return nil
	}

	err = c.syncAddAndUpdatePod(pod)
	if err != nil {
		metrics.SendErrorLogAndMetric(util.PodID, "Error: Failed to sync pod due to %v", err)
		return fmt.Errorf("Failed to sync pod due to %v\n", err)
	}

	return nil
}

func (c *podController) syncAddedPod(podObj *corev1.Pod) error {
	npmPodObj := newNpmPod(podObj)
	podNs := util.GetNSNameWithPrefix(podObj.Namespace)
	podKey, _ := cache.MetaNamespaceKeyFunc(podObj)
	ipsMgr := c.npMgr.NsMap[util.KubeAllNamespacesFlag].IpsMgr
	log.Logf("POD CREATING: [%s%s/%s/%s%+v%s]", npmPodObj.PodUID, podNs, npmPodObj.Name, npmPodObj.NodeName, npmPodObj.Labels, npmPodObj.PodIP)

	// Add pod namespace if it doesn't exist
	var err error
	if _, exists := c.npMgr.NsMap[podNs]; !exists {
		// (TODO): need to change newNS function. It always returns "nil"
		c.npMgr.NsMap[podNs], _ = newNs(podNs)
		log.Logf("Creating set: %v, hashedSet: %v", podNs, util.GetHashedName(podNs))
		if err = ipsMgr.CreateSet(podNs, append([]string{util.IpsetNetHashFlag})); err != nil {
			return fmt.Errorf("[syncAddedPod] Error: creating ipset %s with err: %v", podNs, err)
		}
	}

	// Add the pod to its namespace's ipset.
	log.Logf("Adding pod %s to ipset %s", npmPodObj.PodIP, podNs)
	if err = ipsMgr.AddToSet(podNs, npmPodObj.PodIP, util.IpsetNetHashFlag, podKey); err != nil {
		return fmt.Errorf("[syncAddedPod] Error: failed to add pod to namespace ipset with err: %v", err)
	}

	// Get lists of podLabelKey and podLabelKey + podLavelValue ,and then start adding them to ipsets.
	addToIPSets := util.GetIPSetListFromLabels(npmPodObj.Labels)
	for _, addIPSetName := range addToIPSets {
		log.Logf("Adding pod %s to ipset %s", npmPodObj.PodIP, addIPSetName)
		if err = ipsMgr.AddToSet(addIPSetName, npmPodObj.PodIP, util.IpsetNetHashFlag, podKey); err != nil {
			return fmt.Errorf("[syncAddedPod] Error: failed to add pod to label ipset with err: %v", err)
		}
	}

	// Add pod's named ports from its ipset.
	if err = manageNamedPortIpsets(ipsMgr, npmPodObj.ContainerPorts, podKey, npmPodObj.PodIP, AddNamedPortIpsets); err != nil {
		return fmt.Errorf("[syncAddedPod] Error: failed to add pod to named port ipset with err: %v", err)
	}

	// add the Pod info to the podMap
	c.npMgr.PodMap[podKey] = npmPodObj
	return nil
}

// syncAddAndUpdatePod handles updating pod ip in its label's ipset.
func (c *podController) syncAddAndUpdatePod(newPodObj *corev1.Pod) error {
	log.Logf("[syncAddAndUpdatePod]")

	podKey, _ := cache.MetaNamespaceKeyFunc(newPodObj)
	newPodObjNs := util.GetNSNameWithPrefix(newPodObj.ObjectMeta.Namespace)
	ipsMgr := c.npMgr.NsMap[util.KubeAllNamespacesFlag].IpsMgr

	// Add pod namespace if it doesn't exist
	var err error
	if _, exists := c.npMgr.NsMap[newPodObjNs]; !exists {
		// (TODO): need to change newNS function. It always returns "nil"
		c.npMgr.NsMap[newPodObjNs], _ = newNs(newPodObjNs)
		log.Logf("Creating set: %v, hashedSet: %v", newPodObjNs, util.GetHashedName(newPodObjNs))
		if err = ipsMgr.CreateSet(newPodObjNs, append([]string{util.IpsetNetHashFlag})); err != nil {
			return fmt.Errorf("[syncAddAndUpdatePod] Error: creating ipset %s with err: %v", newPodObjNs, err)
		}
	}

	cachedNpmPodObj, exists := c.npMgr.PodMap[podKey]
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
	// Start processing three update cases - "ip address", label", and "portlist" change by comparing last applied states against current Pod states

	deleteFromIPSets := []string{}
	addToIPSets := []string{}
	// compare cached NPMPod's IP address against newPod's IP address
	if cachedNpmPodObj.PodIP != newPodObj.Status.PodIP {
		metrics.SendErrorLogAndMetric(util.PodID, "[syncAddAndUpdatePod] Info: Unexpected state. Pod (Namespace:%s, Name:%s, uid:%s, has cachedPodIp:%s which is different from PodIp:%s",
			newPodObjNs, newPodObj.Name, cachedNpmPodObj.PodUID, cachedNpmPodObj.PodIP, newPodObj.Status.PodIP)
		// cached PodIP needs to be cleaned up from all the cached labels
		deleteFromIPSets = util.GetIPSetListFromLabels(cachedNpmPodObj.Labels)

		// Assume that the pod IP will be released when pod moves to succeeded or failed state.
		// If the pod transitions back to an active state, then add operation will re-establish the updated pod info.
		// new PodIP needs to be added to all newLabels
		addToIPSets = util.GetIPSetListFromLabels(newPodObj.Labels)

		log.Logf("Deleting pod %s %s from ipset %s and adding pod %s to ipset %s",
			cachedNpmPodObj.PodUID, cachedNpmPodObj.PodIP, cachedNpmPodObj.Namespace, newPodObj.Status.PodIP, newPodObjNs,
		)

		// Delete the pod from its namespace's ipset.
		if err = ipsMgr.DeleteFromSet(cachedNpmPodObj.Namespace, cachedNpmPodObj.PodIP, podKey); err != nil {
			return fmt.Errorf("[syncAddAndUpdatePod] Error: failed to delete pod from namespace ipset with err: %v", err)
		}
		// Add the pod to its namespace's ipset.
		if err = ipsMgr.AddToSet(newPodObjNs, newPodObj.Status.PodIP, util.IpsetNetHashFlag, podKey); err != nil {
			return fmt.Errorf("[syncAddAndUpdatePod] Error: failed to add pod to namespace ipset with err: %v", err)
		}
	} else { // the IP addresses of the cached npmPod and newPodObj is the same
		// If no change in labels, then GetIPSetListCompareLabels will return empty list.
		// Otherwise it returns list of deleted PodIP from cached pod's labels and list of added PodIp from new pod's labels
		addToIPSets, deleteFromIPSets = util.GetIPSetListCompareLabels(cachedNpmPodObj.Labels, newPodObj.Labels)
	}

	// Delete the pod from its label's ipset.
	for _, podIPSetName := range deleteFromIPSets {
		log.Logf("Deleting pod %s from ipset %s", cachedNpmPodObj.PodIP, podIPSetName)
		if err = ipsMgr.DeleteFromSet(podIPSetName, cachedNpmPodObj.PodIP, podKey); err != nil {
			return fmt.Errorf("[syncAddAndUpdatePod] Error: failed to delete pod from label ipset with err: %v", err)
		}
	}

	// Add the pod to its label's ipset.
	for _, addIPSetName := range addToIPSets {
		log.Logf("Adding pod %s to ipset %s", newPodObj.Status.PodIP, addIPSetName)
		if err = ipsMgr.AddToSet(addIPSetName, newPodObj.Status.PodIP, util.IpsetNetHashFlag, podKey); err != nil {
			return fmt.Errorf("[syncAddAndUpdatePod] Error: failed to add pod to label ipset with err: %v", err)
		}
	}

	// (TODO): optimize named port addition and deletions.
	// named ports are mostly static once configured in todays usage pattern
	// so keeping this simple by deleting all and re-adding
	newPodPorts := getContainerPortList(newPodObj)
	if !reflect.DeepEqual(cachedNpmPodObj.ContainerPorts, newPodPorts) {
		// Delete cached pod's named ports from its ipset.
		if err = manageNamedPortIpsets(ipsMgr, cachedNpmPodObj.ContainerPorts, podKey, cachedNpmPodObj.PodIP, DeleteNamedPortIpsets); err != nil {
			return fmt.Errorf("[syncAddAndUpdatePod] Error: failed to delete pod from named port ipset with err: %v", err)
		}
		// Add new pod's named ports from its ipset.
		if err = manageNamedPortIpsets(ipsMgr, newPodPorts, podKey, newPodObj.Status.PodIP, AddNamedPortIpsets); err != nil {
			return fmt.Errorf("[syncAddAndUpdatePod] Error: failed to add pod to named port ipset with err: %v", err)
		}
	}

	// Updating pod cache with new npmPod information
	c.npMgr.PodMap[podKey] = newNpmPod(newPodObj)

	return nil
}

// cleanUpDeletedPod cleans up all ipset associated with this pod
func (c *podController) cleanUpDeletedPod(podObj *corev1.Pod) error {
	log.Logf("[cleanUpDeletedPod]")

	podKey, _ := cache.MetaNamespaceKeyFunc(podObj)
	// If cached npmPod does not exist, return nil
	cachedNpmPodObj, exist := c.npMgr.PodMap[podKey]
	if !exist {
		return nil
	}

	podNs := util.GetNSNameWithPrefix(podObj.Namespace)
	ipsMgr := c.npMgr.NsMap[util.KubeAllNamespacesFlag].IpsMgr
	var err error
	// Delete the pod from its namespace's ipset.
	if err = ipsMgr.DeleteFromSet(podNs, cachedNpmPodObj.PodIP, podKey); err != nil {
		return fmt.Errorf("[cleanUpDeletedPod] Error: failed to delete pod from namespace ipset with err: %v", err)
	}

	// Get lists of podLabelKey and podLabelKey + podLavelValue ,and then start deleting them from ipsets
	deleteFromIPSets := util.GetIPSetListFromLabels(cachedNpmPodObj.Labels)
	for _, podIPSetName := range deleteFromIPSets {
		log.Logf("Deleting pod %s from ipset %s", cachedNpmPodObj.PodIP, podIPSetName)
		if err = ipsMgr.DeleteFromSet(podIPSetName, cachedNpmPodObj.PodIP, podKey); err != nil {
			return fmt.Errorf("[cleanUpDeletedPod] Error: failed to delete pod from label ipset with err: %v", err)
		}
	}

	// Delete pod's named ports from its ipset. Need to pass true in the manageNamedPortIpsets function call
	if err = manageNamedPortIpsets(ipsMgr, cachedNpmPodObj.ContainerPorts, podKey, cachedNpmPodObj.PodIP, DeleteNamedPortIpsets); err != nil {
		return fmt.Errorf("[cleanUpDeletedPod] Error: failed to delete pod from named port ipset with err: %v", err)
	}

	delete(c.npMgr.PodMap, podKey)
	return nil
}

func isCompletePod(podObj *corev1.Pod) bool {
	if podObj.DeletionTimestamp != nil {
		return true
	}

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
		reflect.DeepEqual(npmPod.PodIPs, newPodObj.Status.PodIPs) &&
		reflect.DeepEqual(npmPod.ContainerPorts, getContainerPortList(newPodObj))
}

// manageNamedPortIpsets helps with adding or deleting Pod namedPort IPsets.
func manageNamedPortIpsets(ipsMgr *ipsm.IpsetManager, portList []corev1.ContainerPort, podKey string, podIP string, namedPortOperation NamedPortOperation) error {
	for _, port := range portList {
		if port.Name == "" {
			continue
		}

		// K8s guarantees port.Protocol has "TCP", "UDP", or "SCTP" if the field exists.
		var protocol string
		if len(port.Protocol) != 0 {
			// without adding ":" after protocol, ipset complains.
			protocol = fmt.Sprintf("%s:", port.Protocol)
		}

		namedPortname := util.NamedPortIPSetPrefix + port.Name
		var err error

		switch namedPortOperation {
		case DeleteNamedPortIpsets:
			if err = ipsMgr.DeleteFromSet(namedPortname, fmt.Sprintf("%s,%s%d", podIP, protocol, port.ContainerPort), podKey); err != nil {
				return err
			}
		case AddNamedPortIpsets:
			if err = ipsMgr.AddToSet(namedPortname, fmt.Sprintf("%s,%s%d", podIP, protocol, port.ContainerPort), util.IpsetIPPortHashFlag, podKey); err != nil {
				return err
			}
		}
	}

	return nil
}
