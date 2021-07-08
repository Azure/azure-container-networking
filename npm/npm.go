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
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	networkinginformers "k8s.io/client-go/informers/networking/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	utilexec "k8s.io/utils/exec"
)

var aiMetadata string

const (
	restoreRetryWaitTimeInSeconds = 5
	restoreMaxRetries             = 10
	backupWaitTimeInSeconds       = 60
	telemetryRetryTimeInSeconds   = 60
	heartbeatIntervalInMinutes    = 30
	reconcileChainTimeInMinutes   = 5
	// TODO: consider increasing thread number later when logics are correct
	threadness = 1
)

// NetworkPolicyManager contains informers for pod, namespace and networkpolicy.
type NetworkPolicyManager struct {
	sync.Mutex

	Exec      utilexec.Interface
	clientset *kubernetes.Clientset

	informerFactory informers.SharedInformerFactory
	podInformer     coreinformers.PodInformer
	podController   *podController

	nsInformer          coreinformers.NamespaceInformer
	nameSpaceController *nameSpaceController

	npInformer       networkinginformers.NetworkPolicyInformer
	netPolController *networkPolicyController

	NodeName       string
	NsMap          map[string]*Namespace                  // Key is ns-<nsname>
	PodMap         map[string]*NpmPod                     // Key is <nsname>/<podname>
	RawNpMap       map[string]*networkingv1.NetworkPolicy // Key is <nsname>/<policyname>
	ProcessedNpMap map[string]*networkingv1.NetworkPolicy // Key is <nsname>/<podSelectorHash>

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
		customDimensions = map[string]string{"ClusterID": util.GetClusterID(npMgr.NodeName),
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
		podCount.Value = 0
		//Reducing one to remove all-namespaces ns obj
		nsCount.Value = float64(len(npMgr.NsMap) - 1)
		nwPolicyCount.Value += float64(len(npMgr.RawNpMap))
		podCount.Value += float64(len(npMgr.PodMap))
		npMgr.Unlock()

		metrics.SendMetric(podCount)
		metrics.SendMetric(nsCount)
		metrics.SendMetric(nwPolicyCount)
	}
}

// restore restores iptables from backup file
func (npMgr *NetworkPolicyManager) restore() {
	iptMgr := iptm.NewIptablesManager(npMgr.Exec, iptm.NewIptOperationShim())
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
func (npMgr *NetworkPolicyManager) backup(stopCh <-chan struct{}) {
	iptMgr := iptm.NewIptablesManager(npMgr.Exec, iptm.NewIptOperationShim())
	ticker := time.NewTicker(time.Second * time.Duration(backupWaitTimeInSeconds))
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			if err := iptMgr.Save(util.IptablesConfigFile); err != nil {
				metrics.SendErrorLogAndMetric(util.NpmID, "Error: failed to back up Azure-NPM states %s", err.Error())
			}
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

	// start controllers after synced
	go npMgr.podController.Run(threadness, stopCh)
	go npMgr.nameSpaceController.Run(threadness, stopCh)
	go npMgr.netPolController.Run(threadness, stopCh)
	go npMgr.reconcileChains(stopCh)
	go npMgr.backup(stopCh)

	return nil
}

// NewNetworkPolicyManager creates a NetworkPolicyManager
func NewNetworkPolicyManager(clientset *kubernetes.Clientset, informerFactory informers.SharedInformerFactory, exec utilexec.Interface, npmVersion string) *NetworkPolicyManager {
	// Clear out left over iptables states
	log.Logf("Azure-NPM creating, cleaning iptables")
	iptMgr := iptm.NewIptablesManager(exec, iptm.NewIptOperationShim())
	iptMgr.UninitNpmChains()

	log.Logf("Azure-NPM creating, cleaning existing Azure NPM IPSets")
	ipsm.NewIpsetManager(exec).DestroyNpmIpsets()

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
		Exec:            exec,
		clientset:       clientset,
		informerFactory: informerFactory,
		podInformer:     podInformer,
		nsInformer:      nsInformer,
		npInformer:      npInformer,
		NodeName:        os.Getenv("HOSTNAME"),
		NsMap:           make(map[string]*Namespace),
		PodMap:          make(map[string]*NpmPod),
		RawNpMap:        make(map[string]*networkingv1.NetworkPolicy),
		ProcessedNpMap:  make(map[string]*networkingv1.NetworkPolicy),
		clusterState: telemetry.ClusterState{
			PodCount:      0,
			NsCount:       0,
			NwPolicyCount: 0,
		},
		version:          npmVersion,
		serverVersion:    serverVersion,
		TelemetryEnabled: true,
	}

	allNs, _ := newNs(util.KubeAllNamespacesFlag, npMgr.Exec)
	npMgr.NsMap[util.KubeAllNamespacesFlag] = allNs

	// Create ipset for the namespace.
	kubeSystemNs := util.GetNSNameWithPrefix(util.KubeSystemFlag)
	if err := allNs.IpsMgr.CreateSet(kubeSystemNs, []string{util.IpsetNetHashFlag}); err != nil {
		metrics.SendErrorLogAndMetric(util.NpmID, "Error: failed to create ipset for namespace %s.", kubeSystemNs)
	}

	// create pod controller
	npMgr.podController = NewPodController(podInformer, clientset, npMgr)

	// create NameSpace controller
	npMgr.nameSpaceController = NewNameSpaceController(nsInformer, clientset, npMgr)

	// create network policy controller
	npMgr.netPolController = NewNetworkPolicyController(npInformer, clientset, npMgr)

	return npMgr
}

// reconcileChains checks for ordering of AZURE-NPM chain in FORWARD chain periodically.
func (npMgr *NetworkPolicyManager) reconcileChains(stopCh <-chan struct{}) {
	iptMgr := iptm.NewIptablesManager(npMgr.Exec, iptm.NewIptOperationShim())
	ticker := time.NewTicker(time.Minute * time.Duration(reconcileChainTimeInMinutes))
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			if err := iptMgr.CheckAndAddForwardChain(); err != nil {
				metrics.SendErrorLogAndMetric(util.NpmID, "Error: failed to reconcileChains Azure-NPM due to %s", err.Error())
			}
		}
	}
}
