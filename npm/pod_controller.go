// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"fmt"
	"reflect"
	"time"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/util"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

var npMgr *NetworkPolicyManager

// PodController struct
type podController struct {
	queue          workqueue.RateLimitingInterface
	informer       coreinformers.PodInformer
	deletedIndexer cache.Indexer
}

// pod Controller code
func NewPodController(networkPolicyManager *NetworkPolicyManager) Controller {
	log.Logf("Init Pod Controller")
	npMgr = networkPolicyManager
	informer := npMgr.informerFactory.Core().V1().Pods()

	// create the workqueue to proces POD updates
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	// stored deleted objects
	deletedIndexer := cache.NewIndexer(cache.DeletionHandlingMetaNamespaceKeyFunc, cache.Indexers{})

	informer.Informer().AddEventHandler(
		// Pod event handlers
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				// Validate pod
				if !isValidPod(obj.(*corev1.Pod)) {
					return
				}

				key, err := cache.MetaNamespaceKeyFunc(obj)
				if err == nil {
					queue.Add(key)
					deletedIndexer.Delete(obj)
				}
			},
			UpdateFunc: func(old, new interface{}) {
				if !isValidPod(new.(*corev1.Pod)) {
					return
				}

				// if isPodIpSame(old.(*corev1.Pod), new.(*corev1.Pod)) {
				// 	return
				// }

				if isInvalidPodUpdate(old.(*corev1.Pod), new.(*corev1.Pod)) {
					return
				}

				key, err := cache.MetaNamespaceKeyFunc(new)
				if err == nil {
					queue.Add(key)
				}
			},
			DeleteFunc: func(obj interface{}) {
				if !isValidPod(obj.(*corev1.Pod)) {
					return
				}

				// IndexerInformer uses a delta queue, therefore for deletes we have to use this
				// key function.
				key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
				if err == nil {
					deletedIndexer.Add(obj)
					queue.Add(key)
				}
			},
		},
	)

	return &podController{
		informer:       informer,
		queue:          queue,
		deletedIndexer: deletedIndexer,
	}
}

// Run will start the controller.
// StopCh channel is used to send interrupt signal to stop it.
func (c *podController) Run(stopCh <-chan struct{}) error {
	// don't let panics crash the process
	//defer utilruntime.HandleCrash()
	// make sure the work queue is shutdown which will trigger workers to end
	defer c.queue.ShutDown()

	log.Logf("Starting Pod controller")
	go c.informer.Informer().Run(stopCh)

	// wait for the caches to synchronize before starting the worker
	if !cache.WaitForCacheSync(stopCh, c.informer.Informer().HasSynced) {
		return fmt.Errorf("Pod informer failed to sync")
	}

	log.Logf("Pod kube-controller synced and ready")

	// runWorker will loop until "something bad" happens.  The .Until will
	// then rekick the worker after one second
	wait.Until(c.runWorker, time.Second, stopCh)

	<-stopCh
	log.Logf("Stopping Pod controller")

	return nil
}

func (c *podController) runWorker() {
	for c.processNextItem() {
	}
}

func (c *podController) processNextItem() bool {
	// Wait until there is a new item in the working queue
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	// Tell the queue that we are done with processing this key. This unblocks the key for other workers
	// This allows safe parallel processing because two pods with the same key are never processed in
	// parallel.
	defer c.queue.Done(key)

	// Invoke the method containing the business logic
	err := c.ProcessPodUpdate(key.(string))
	// Handle the error if something went wrong during the execution of the business logic
	c.handleErr(err, key)
	return true
}

// ProcessPodUpdate process next pod
func (c *podController) ProcessPodUpdate(key string) error {
	obj, exists, err := c.informer.Informer().GetIndexer().GetByKey(key)
	if err != nil {
		log.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}

	if !exists {
		// Below we will warm up our cache with a Pod, so that we will see a delete for one pod
		log.Logf("Pod %s does not exist anymore\n", key)

		if obj, exists, err = c.deletedIndexer.GetByKey(key); err == nil && exists {
			var podObj = obj.(*corev1.Pod)
			log.Logf("Deleting pod %s obj %v\n", key, podObj)
			if err = npMgr.DeletePod(podObj); err != nil {
				log.Errorf("Error: failed to delete pod during update with error %+v", err)
				return err
			}

			// Delete from delete indexer
			c.deletedIndexer.Delete(key)
		}
	} else {
		var podObj = obj.(*corev1.Pod)
		podObjPhase := podObj.Status.Phase

		// There can be an add or update event, thus first delete the existing map and add it again
		// TODO: Need to optimize this
		log.Logf("Sync/Add/Update for Pod %v, phase: %s, ip: %s\n", podObj, podObjPhase, podObj.Status.PodIP)
		if err = npMgr.DeletePod(podObj); err != nil {
			log.Errorf("Error: failed to delete pod during update with error %+v", err)
			return err
		}

		// Assume that the pod IP will be released when pod moves to succeeded or failed state.
		// If the pod transitions back to an active state, then add operation will re establish the updated pod info.
		if podObj.ObjectMeta.DeletionTimestamp == nil && podObj.ObjectMeta.DeletionGracePeriodSeconds == nil &&
			podObjPhase != corev1.PodSucceeded && podObjPhase != corev1.PodFailed {
			if err = npMgr.AddPod(podObj); err != nil {
				log.Errorf("Error: failed to add pod during update with error %+v", err)
			}
		}
	}

	return nil
}

// handleErr checks if an error happened and makes sure we will retry later.
func (c *podController) handleErr(err error, key interface{}) {
	if err == nil {
		// Forget about the #AddRateLimited history of the key on every successful synchronization.
		// This ensures that future processing of updates for this key is not delayed because of
		// an outdated error history.
		c.queue.Forget(key)
		return
	}

	// This controller retries 5 times if something goes wrong. After that, it stops trying.
	if c.queue.NumRequeues(key) < 5 {
		log.Logf("Error syncing pod %v: %v", key, err)

		// Re-enqueue the key rate limited. Based on the rate limiter on the
		// queue and the re-enqueue history, the key will be processed later again.
		c.queue.AddRateLimited(key)
		return
	}

	c.queue.Forget(key)
	// Report to an external entity that, even after several retries, we could not successfully process this key
	runtime.HandleError(err)
	log.Logf("Dropping pod %q out of the queue: %v", key, err)
}

func isValidPod(podObj *corev1.Pod) bool {
	return len(podObj.Status.PodIP) > 0
}

func isSystemPod(podObj *corev1.Pod) bool {
	return podObj.ObjectMeta.Namespace == util.KubeSystemFlag
}

func isPodIpSame(oldPodObj, newPodObj *corev1.Pod) (isPodIpSame bool) {
	return oldPodObj.Status.PodIP == newPodObj.Status.PodIP
}

func isInvalidPodUpdate(oldPodObj, newPodObj *corev1.Pod) (isInvalidUpdate bool) {
	isInvalidUpdate = oldPodObj.Status.PodIP == newPodObj.Status.PodIP &&
		oldPodObj.ObjectMeta.Namespace == newPodObj.ObjectMeta.Namespace &&
		oldPodObj.ObjectMeta.Name == newPodObj.ObjectMeta.Name &&
		oldPodObj.Status.Phase == newPodObj.Status.Phase &&
		newPodObj.ObjectMeta.DeletionTimestamp == nil &&
		newPodObj.ObjectMeta.DeletionGracePeriodSeconds == nil
	isInvalidUpdate = isInvalidUpdate && reflect.DeepEqual(oldPodObj.ObjectMeta.Labels, newPodObj.ObjectMeta.Labels)

	return isInvalidUpdate
}
