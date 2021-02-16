// Package npm Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/Azure/azure-container-networking/aitelemetry"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/ipsm"
	"github.com/Azure/azure-container-networking/npm/iptm"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/util"
	"github.com/Azure/azure-container-networking/telemetry"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	networkinginformers "k8s.io/client-go/informers/networking/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

var aiMetadata string

const (
	restoreRetryWaitTimeInSeconds = 5
	restoreMaxRetries             = 10
	backupWaitTimeInSeconds       = 60
	telemetryRetryTimeInSeconds   = 60
	heartbeatIntervalInMinutes    = 30
)

// NetworkPolicyManager contains informers for pod, namespace and networkpolicy.
type NetworkPolicyManager struct {
	sync.Mutex
	clientset *kubernetes.Clientset

	informerFactory informers.SharedInformerFactory
	podInformer     coreinformers.PodInformer
	nsInformer      coreinformers.NamespaceInformer
	npInformer      networkinginformers.NetworkPolicyInformer

	nodeName                     string
	nsMap                        map[string]*namespace
	podMap                       map[string]string // Key: Pod uuid, Value: PodIp
	isAzureNpmChainCreated       bool
	isSafeToCleanUpAzureNpmChain bool

	clusterState telemetry.ClusterState
	version      string

	serverVersion    *version.Info
	TelemetryEnabled bool
}

// GetClusterState returns current cluster state.
func (npMgr *NetworkPolicyManager) GetClusterState() telemetry.ClusterState {
	pods, err := npMgr.clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Logf("Error: Failed to list pods in GetClusterState")
	}

	namespaces, err := npMgr.clientset.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Logf("Error: Failed to list namespaces in GetClusterState")
	}

	networkpolicies, err := npMgr.clientset.NetworkingV1().NetworkPolicies("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Logf("Error: Failed to list networkpolicies in GetClusterState")
	}

	npMgr.clusterState.PodCount = len(pods.Items)
	npMgr.clusterState.NsCount = len(namespaces.Items)
	npMgr.clusterState.NwPolicyCount = len(networkpolicies.Items)

	return npMgr.clusterState
}

// GetAppVersion returns network policy manager app version
func (npMgr *NetworkPolicyManager) GetAppVersion() string {
	return npMgr.version
}

// GetAIMetadata returns ai metadata number
func GetAIMetadata() string {
	return aiMetadata
}

// SendClusterMetrics :- send NPM cluster metrics using AppInsights
func (npMgr *NetworkPolicyManager) SendClusterMetrics() {
	var (
		heartbeat        = time.NewTicker(time.Minute * heartbeatIntervalInMinutes).C
		customDimensions = map[string]string{"ClusterID": util.GetClusterID(npMgr.nodeName),
			"APIServer": npMgr.serverVersion.String()}
		podCount = aitelemetry.Metric{
			Name:             "PodCount",
			CustomDimensions: customDimensions,
		}
		nsCount = aitelemetry.Metric{
			Name:             "NsCount",
			CustomDimensions: customDimensions,
		}
		nwPolicyCount = aitelemetry.Metric{
			Name:             "NwPolicyCount",
			CustomDimensions: customDimensions,
		}
	)

	for {
		<-heartbeat
		npMgr.Lock()
		podCount.Value = float64(len(npMgr.podMap))
		//Reducing one to remove all-namespaces ns obj
		nsCount.Value = float64(len(npMgr.nsMap) - 1)
		nwPolCount := 0
		for _, ns := range npMgr.nsMap {
			nwPolCount = nwPolCount + len(ns.rawNpMap)
		}
		nwPolicyCount.Value = float64(nwPolCount)
		npMgr.Unlock()

		metrics.SendMetric(podCount)
		metrics.SendMetric(nsCount)
		metrics.SendMetric(nwPolicyCount)
	}
}

// restore restores iptables from backup file
func (npMgr *NetworkPolicyManager) restore() {
	iptMgr := iptm.NewIptablesManager()
	var err error
	for i := 0; i < restoreMaxRetries; i++ {
		if err = iptMgr.Restore(util.IptablesConfigFile); err == nil {
			return
		}

		time.Sleep(restoreRetryWaitTimeInSeconds * time.Second)
	}

	metrics.SendErrorLogAndMetric(util.NpmID, "Error: timeout restoring Azure-NPM states")
	panic(err.Error)
}

// backup takes snapshots of iptables filter table and saves it periodically.
func (npMgr *NetworkPolicyManager) backup() {
	iptMgr := iptm.NewIptablesManager()
	var err error
	for {
		time.Sleep(backupWaitTimeInSeconds * time.Second)

		if err = iptMgr.Save(util.IptablesConfigFile); err != nil {
			metrics.SendErrorLogAndMetric(util.NpmID, "Error: failed to back up Azure-NPM states")
		}
	}
}

// Start starts shared informers and waits for the shared informer cache to sync.
func (npMgr *NetworkPolicyManager) Start(stopCh <-chan struct{}) error {
	// Starts all informers manufactured by npMgr's informerFactory.
	npMgr.informerFactory.Start(stopCh)

	// Wait for the initial sync of local cache.
	if !cache.WaitForCacheSync(stopCh, npMgr.podInformer.Informer().HasSynced) {
		metrics.SendErrorLogAndMetric(util.NpmID, "Pod informer failed to sync")
		return fmt.Errorf("Pod informer failed to sync")
	}

	if !cache.WaitForCacheSync(stopCh, npMgr.nsInformer.Informer().HasSynced) {
		metrics.SendErrorLogAndMetric(util.NpmID, "Namespace informer failed to sync")
		return fmt.Errorf("Namespace informer failed to sync")
	}

	if !cache.WaitForCacheSync(stopCh, npMgr.npInformer.Informer().HasSynced) {
		metrics.SendErrorLogAndMetric(util.NpmID, "Network policy informer failed to sync")
		return fmt.Errorf("Network policy informer failed to sync")
	}

	go npMgr.backup()

	return nil
}

// NewNetworkPolicyManager creates a NetworkPolicyManager
func NewNetworkPolicyManager(clientset *kubernetes.Clientset, informerFactory informers.SharedInformerFactory, npmVersion string) *NetworkPolicyManager {
	// Clear out left over iptables states
	log.Logf("Azure-NPM creating, cleaning iptables")
	iptMgr := iptm.NewIptablesManager()
	iptMgr.UninitNpmChains()

	log.Logf("Azure-NPM creating, cleaning existing Azure NPM IPSets")
	ipsm.NewIpsetManager().DestroyNpmIpsets()

	var (
		podInformer   = informerFactory.Core().V1().Pods()
		nsInformer    = informerFactory.Core().V1().Namespaces()
		npInformer    = informerFactory.Networking().V1().NetworkPolicies()
		serverVersion *version.Info
		err           error
	)

	for ticker, start := time.NewTicker(1*time.Second).C, time.Now(); time.Since(start) < time.Minute*1; {
		<-ticker
		serverVersion, err = clientset.ServerVersion()
		if err == nil {
			break
		}
	}
	if err != nil {
		metrics.SendErrorLogAndMetric(util.NpmID, "Error: failed to retrieving kubernetes version")
		panic(err.Error)
	}
	log.Logf("API server version: %+v", serverVersion)

	if err = util.SetIsNewNwPolicyVerFlag(serverVersion); err != nil {
		metrics.SendErrorLogAndMetric(util.NpmID, "Error: failed to set IsNewNwPolicyVerFlag")
		panic(err.Error)
	}

	npMgr := &NetworkPolicyManager{
		clientset:                    clientset,
		informerFactory:              informerFactory,
		podInformer:                  podInformer,
		nsInformer:                   nsInformer,
		npInformer:                   npInformer,
		nodeName:                     os.Getenv("HOSTNAME"),
		nsMap:                        make(map[string]*namespace),
		podMap:                       make(map[string]string),
		isAzureNpmChainCreated:       false,
		isSafeToCleanUpAzureNpmChain: false,
		clusterState: telemetry.ClusterState{
			PodCount:      0,
			NsCount:       0,
			NwPolicyCount: 0,
		},
		version:          npmVersion,
		serverVersion:    serverVersion,
		TelemetryEnabled: true,
	}

	allNs, _ := newNs(util.KubeAllNamespacesFlag)
	npMgr.nsMap[util.KubeAllNamespacesFlag] = allNs

	// Create ipset for the namespace.
	kubeSystemNs := "ns-" + util.KubeSystemFlag
	if err := allNs.ipsMgr.CreateSet(kubeSystemNs, append([]string{util.IpsetNetHashFlag})); err != nil {
		metrics.SendErrorLogAndMetric(util.NpmID, "Error: failed to create ipset for namespace %s.", kubeSystemNs)
	}

	podInformer.Informer().AddEventHandler(
		// Pod event handlers
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				npMgr.Lock()
				npMgr.AddPod(obj.(*corev1.Pod))
				npMgr.Unlock()
			},
			UpdateFunc: func(old, new interface{}) {
				npMgr.Lock()
				npMgr.UpdatePod(old.(*corev1.Pod), new.(*corev1.Pod))
				npMgr.Unlock()
			},
			DeleteFunc: func(obj interface{}) {
				npMgr.Lock()
				npMgr.DeletePod(obj.(*corev1.Pod))
				npMgr.Unlock()
			},
		},
	)

	nsInformer.Informer().AddEventHandler(
		// Namespace event handlers
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				npMgr.Lock()
				npMgr.AddNamespace(obj.(*corev1.Namespace))
				npMgr.Unlock()
			},
			UpdateFunc: func(old, new interface{}) {
				npMgr.Lock()
				npMgr.UpdateNamespace(old.(*corev1.Namespace), new.(*corev1.Namespace))
				npMgr.Unlock()
			},
			DeleteFunc: func(obj interface{}) {
				npMgr.Lock()
				npMgr.DeleteNamespace(obj.(*corev1.Namespace))
				npMgr.Unlock()
			},
		},
	)

	npInformer.Informer().AddEventHandler(
		// Network policy event handlers
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				npMgr.Lock()
				npMgr.AddNetworkPolicy(obj.(*networkingv1.NetworkPolicy))
				npMgr.Unlock()
			},
			UpdateFunc: func(old, new interface{}) {
				npMgr.Lock()
				npMgr.UpdateNetworkPolicy(old.(*networkingv1.NetworkPolicy), new.(*networkingv1.NetworkPolicy))
				npMgr.Unlock()
			},
			DeleteFunc: func(obj interface{}) {
				npMgr.Lock()
				npMgr.DeleteNetworkPolicy(obj.(*networkingv1.NetworkPolicy))
				npMgr.Unlock()
			},
		},
	)

	return npMgr
}
