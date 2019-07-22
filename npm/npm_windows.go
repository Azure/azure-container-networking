// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"fmt"
	"os"
	"reflect"
	"sync"
	"time"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/hnsm"
	"github.com/Azure/azure-container-networking/npm/util"
	"github.com/Azure/azure-container-networking/telemetry"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	networkinginformers "k8s.io/client-go/informers/networking/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

const (
	restoreRetryWaitTimeInSeconds = 5
	restoreMaxRetries             = 10
	backupWaitTimeInSeconds       = 60
)

// reports channel
var reports = make(chan interface{}, 1000)

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

	clusterState  telemetry.ClusterState
	reportManager *telemetry.ReportManager

	serverVersion    *version.Info
	TelemetryEnabled bool
}

// restore restores ACL policies from backup file
func (npMgr *NetworkPolicyManager) restore() {
	aclMgr := hnsm.NewACLPolicyManager()
	var err error
	for i := 0; i < restoreMaxRetries; i++ {
		if err = aclMgr.Restore(util.ACLPolicyConfigFile); err == nil {
			return
		}

		time.Sleep(restoreRetryWaitTimeInSeconds * time.Second)
	}

	log.Logf("Error: timeout restoring Azure-NPM states")
	panic(err.Error)
}

// backup takes snapshots of ACL policies and saves them periodically.
func (npMgr *NetworkPolicyManager) backup() {
	aclMgr := hnsm.NewACLPolicyManager()
	var err error
	for {
		time.Sleep(backupWaitTimeInSeconds * time.Second)

		if err = aclMgr.Save(util.ACLPolicyConfigFile); err != nil {
			log.Logf("Error: failed to back up Azure-NPM states")
		}
	}
}

// Start starts shared informers and waits for the shared informer cache to sync.
func (npMgr *NetworkPolicyManager) Start(stopCh <-chan struct{}) error {
	// Starts all informers manufactured by npMgr's informerFactory.
	npMgr.informerFactory.Start(stopCh)

	// Failure detected. Needs to restore Azure-NPM related ACL policies.
	if util.Exists(util.ACLPolicyConfigFile) {
		npMgr.restore()
	}

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

	go npMgr.backup()

	return nil
}

// NewNetworkPolicyManager creates a NetworkPolicyManager
func NewNetworkPolicyManager(clientset *kubernetes.Clientset, informerFactory informers.SharedInformerFactory, npmVersion string) *NetworkPolicyManager {

	podInformer := informerFactory.Core().V1().Pods()
	nsInformer := informerFactory.Core().V1().Namespaces()
	npInformer := informerFactory.Networking().V1().NetworkPolicies()

	serverVersion, err := clientset.ServerVersion()
	if err != nil {
		log.Logf("Error: failed to retrieving kubernetes version")
		panic(err.Error)
	}
	log.Logf("API server version: %+v", serverVersion)

	if err = util.SetIsNewNwPolicyVerFlag(serverVersion); err != nil {
		log.Logf("Error: failed to set IsNewNwPolicyVerFlag")
		panic(err.Error)
	}

	npMgr := &NetworkPolicyManager{
		clientset:              clientset,
		informerFactory:        informerFactory,
		podInformer:            podInformer,
		nsInformer:             nsInformer,
		npInformer:             npInformer,
		nodeName:               os.Getenv("COMPUTERNAME"),
		nsMap:                  make(map[string]*namespace),
		clusterState: telemetry.ClusterState{
			PodCount:      0,
			NsCount:       0,
			NwPolicyCount: 0,
		},
		reportManager: &telemetry.ReportManager{
			ContentType: telemetry.ContentType,
			Report:      &telemetry.NPMReport{},
		},
		serverVersion:    serverVersion,
		TelemetryEnabled: true,
	}

	// Set-up channel for Azure-NPM telemetry if it's enabled (enabled by default)
	if logger := log.GetStd(); logger != nil && npMgr.TelemetryEnabled {
		logger.SetChannel(reports)
	}

	clusterID := util.GetClusterID(npMgr.nodeName)
	clusterState := npMgr.GetClusterState()
	npMgr.reportManager.Report.(*telemetry.NPMReport).GetReport(clusterID, npMgr.nodeName, npmVersion, serverVersion.GitVersion, clusterState)

	allNs, err := newNs(util.KubeAllNamespacesFlag)
	if err != nil {
		log.Logf("Error: failed to create all-namespace.")
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
