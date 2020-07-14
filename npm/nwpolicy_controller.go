// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"fmt"
	"time"

	"github.com/Azure/azure-container-networking/log"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	networkinginformers "k8s.io/client-go/informers/networking/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

// NwPolicy Controller struct
type nwPolicyController struct {
	queue    workqueue.RateLimitingInterface
	informer networkinginformers.NetworkPolicyInformer
	npMgr    *NetworkPolicyManager
}

// nwpolicy Controller code
func NewNwPolicyController(networkPolicyManager *NetworkPolicyManager) Controller {

	log.Logf("Init NetworkPolicy controller")
	npMgr := networkPolicyManager
	informer := npMgr.informerFactory.Networking().V1().NetworkPolicies()

	// create the workqueue to proces nwpolicy updates
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	informer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				key, err := cache.MetaNamespaceKeyFunc(obj)
				if err == nil {
					queue.Add(key)
				}
			},
			UpdateFunc: func(old, new interface{}) {
				key, err := cache.MetaNamespaceKeyFunc(new)
				if err == nil {
					queue.Add(key)
				}
			},
			DeleteFunc: func(obj interface{}) {
				// IndexerInformer uses a delta queue, therefore for deletes we have to use this
				// key function.
				key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
				if err == nil {
					queue.Add(key)
				}
			},
		},
	)

	return &nwPolicyController{
		informer: informer,
		queue:    queue,
		npMgr:    npMgr,
	}
}

// Run will start the controller.
// StopCh channel is used to send interrupt signal to stop it.
func (c *nwPolicyController) Run(stopCh <-chan struct{}) error {
	// don't let panics crash the process
	//defer utilruntime.HandleCrash()
	// make sure the work queue is shutdown which will trigger workers to end
	defer c.queue.ShutDown()

	log.Logf("Starting NetworkPolicy controller")

	go c.informer.Informer().Run(stopCh)

	// wait for the caches to synchronize before starting the worker
	if !cache.WaitForCacheSync(stopCh, c.informer.Informer().HasSynced) {
		return fmt.Errorf("NetworkPolicy informer failed to sync")
	}

	log.Logf("Kubewatch controller synced and ready")

	// runWorker will loop until "something bad" happens.  The .Until will
	// then rekick the worker after one second
	wait.Until(c.runWorker, time.Second, stopCh)

	<-stopCh
	log.Logf("Stopping NetworkPolicy controller")

	return nil
}

func (c *nwPolicyController) runWorker() {
	for c.processNextItem() {
	}
}

func (c *nwPolicyController) processNextItem() bool {
	// Wait until there is a new item in the working queue
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	// Tell the queue that we are done with processing this key. This unblocks the key for other workers
	// This allows safe parallel processing because two nwpolicy with the same key are never processed in
	// parallel.
	defer c.queue.Done(key)

	// Invoke the method containing the business logic
	err := c.ProcessUpdate(key.(string))
	// Handle the error if something went wrong during the execution of the business logic
	c.handleErr(err, key)
	return true
}

// ProcessUpdate process next nwpolicy
func (c *nwPolicyController) ProcessUpdate(key string) error {
	obj, exists, err := c.informer.Informer().GetIndexer().GetByKey(key)
	if err != nil {
		log.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}

	if !exists {
		log.Logf("Delete NwPolicy %s does not exist anymore\n", key)
		// if err = c.npMgr.DeleteNetworkPolicy(nwPolicyObj); err != nil {
		// 	log.Errorf("Error: failed to delete NwPolicy during update with error %+v", err)
		// 	return err
		// }
	} else {
		var nwPolicyObj = obj.(*networkingv1.NetworkPolicy)
		var nwPolicyName = nwPolicyObj.Name
		// Note that you also have to check the uid if you have a local controlled resource, which
		// is dependent on the actual instance, to detect that a nwpolicy was recreated with the same name

		if nwPolicyObj.ObjectMeta.DeletionTimestamp == nil &&
			nwPolicyObj.ObjectMeta.DeletionGracePeriodSeconds == nil {
			log.Logf("Sync/Add/Update for NwPolicy %s\n", nwPolicyName)
			return c.npMgr.AddNetworkPolicy(nwPolicyObj)
		} else {
			log.Logf("Deleting NwPolicy %s, name %s\n", key, nwPolicyName)
			if err = c.npMgr.DeleteNetworkPolicy(nwPolicyObj); err != nil {
				log.Errorf("Error: failed to delete NwPolicy during update with error %+v", err)
				return err
			}
		}
	}

	return nil
}

// handleErr checks if an error happened and makes sure we will retry later.
func (c *nwPolicyController) handleErr(err error, key interface{}) {
	if err == nil {
		// Forget about the #AddRateLimited history of the key on every successful synchronization.
		// This ensures that future processing of updates for this key is not delayed because of
		// an outdated error history.
		c.queue.Forget(key)
		return
	}

	// This controller retries 5 times if something goes wrong. After that, it stops trying.
	if c.queue.NumRequeues(key) < 5 {
		log.Logf("Error syncing nwpolicy %v: %v", key, err)

		// Re-enqueue the key rate limited. Based on the rate limiter on the
		// queue and the re-enqueue history, the key will be processed later again.
		c.queue.AddRateLimited(key)
		return
	}

	c.queue.Forget(key)
	// Report to an external entity that, even after several retries, we could not successfully process this key
	runtime.HandleError(err)
	log.Logf("Dropping nwpolicy %q out of the queue: %v", key, err)
}
