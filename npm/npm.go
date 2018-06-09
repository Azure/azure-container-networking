package npm

import (
	"fmt"
	"os"
	"sync"

	"github.com/Azure/azure-container-networking/npm/util"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	networkinginformers "k8s.io/client-go/informers/networking/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// NetworkPolicyManager contains informers for pod, namespace and networkpolicy.
type NetworkPolicyManager struct {
	sync.Mutex
	clientset *kubernetes.Clientset

	informerFactory informers.SharedInformerFactory
	podInformer     coreinformers.PodInformer
	nsInformer      coreinformers.NamespaceInformer
	npInformer      networkinginformers.NetworkPolicyInformer

	nodeName               string
	nsMap                  map[string]*namespace
	isAzureNpmChainCreated bool
}

// Run starts shared informers and waits for the shared informer cache to sync.
func (npMgr *NetworkPolicyManager) Run(stopCh <-chan struct{}) error {
	// Starts all informers manufactured by npMgr's informerFactory.
	npMgr.informerFactory.Start(stopCh)

	// Wait for the initial sync of local cache.
	if !cache.WaitForCacheSync(stopCh, npMgr.podInformer.Informer().HasSynced) {
		return fmt.Errorf("Pod informer failed to sync")
	}

	if !cache.WaitForCacheSync(stopCh, npMgr.nsInformer.Informer().HasSynced) {
		return fmt.Errorf("Namespace informer failed to sync")
	}

	if !cache.WaitForCacheSync(stopCh, npMgr.npInformer.Informer().HasSynced) {
		return fmt.Errorf("Namespace informer failed to sync")
	}

	return nil
}

// NewNetworkPolicyManager creates a NetworkPolicyManager
func NewNetworkPolicyManager(clientset *kubernetes.Clientset, informerFactory informers.SharedInformerFactory) *NetworkPolicyManager {

	podInformer := informerFactory.Core().V1().Pods()
	nsInformer := informerFactory.Core().V1().Namespaces()
	npInformer := informerFactory.Networking().V1().NetworkPolicies()

	npMgr := &NetworkPolicyManager{
		clientset:       clientset,
		informerFactory: informerFactory,
		podInformer:     podInformer,
		nsInformer:      nsInformer,
		npInformer:      npInformer,
		nodeName:        os.Getenv("HOSTNAME"),
		nsMap:           make(map[string]*namespace),
		isAzureNpmChainCreated: false,
	}

	allNs, err := newNs(util.KubeAllNamespacesFlag)
	if err != nil {
		panic(err.Error)
	}
	npMgr.nsMap[util.KubeAllNamespacesFlag] = allNs

	podInformer.Informer().AddEventHandler(
		// Pod event handlers
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				npMgr.AddPod(obj.(*corev1.Pod))
			},
			UpdateFunc: func(old, new interface{}) {
				npMgr.UpdatePod(old.(*corev1.Pod), new.(*corev1.Pod))
			},
			DeleteFunc: func(obj interface{}) {
				npMgr.DeletePod(obj.(*corev1.Pod))
			},
		},
	)

	nsInformer.Informer().AddEventHandler(
		// Namespace event handlers
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				npMgr.AddNamespace(obj.(*corev1.Namespace))
			},
			UpdateFunc: func(old, new interface{}) {
				npMgr.UpdateNamespace(old.(*corev1.Namespace), new.(*corev1.Namespace))
			},
			DeleteFunc: func(obj interface{}) {
				npMgr.DeleteNamespace(obj.(*corev1.Namespace))
			},
		},
	)

	npInformer.Informer().AddEventHandler(
		// Network policy event handlers
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				npMgr.AddNetworkPolicy(obj.(*networkingv1.NetworkPolicy))
			},
			UpdateFunc: func(old, new interface{}) {
				npMgr.UpdateNetworkPolicy(old.(*networkingv1.NetworkPolicy), new.(*networkingv1.NetworkPolicy))
			},
			DeleteFunc: func(obj interface{}) {
				npMgr.DeleteNetworkPolicy(obj.(*networkingv1.NetworkPolicy))
			},
		},
	)

	return npMgr
}
