// Package npm Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Azure/azure-container-networking/aitelemetry"
	"github.com/Azure/azure-container-networking/log"

	"github.com/Azure/azure-container-networking/npm/ipsm"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/util"
	"github.com/Azure/azure-container-networking/telemetry"
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
	clientset *kubernetes.Clientset

	informerFactory informers.SharedInformerFactory
	podInformer     coreinformers.PodInformer
	podController   *podController

	nsInformer          coreinformers.NamespaceInformer
	nameSpaceController *nameSpaceController

	npInformer       networkinginformers.NetworkPolicyInformer
	netPolController *networkPolicyController

	// ipsMgr are shared in all controllers, so we need to manage lock in IpsetManager.
	ipsMgr *ipsm.IpsetManager

	// Azure-specific variables
	clusterState     telemetry.ClusterState
	serverVersion    *version.Info
	NodeName         string
	version          string
	TelemetryEnabled bool
}

// NewNetworkPolicyManager creates a NetworkPolicyManager
func NewNetworkPolicyManager(clientset *kubernetes.Clientset, informerFactory informers.SharedInformerFactory, exec utilexec.Interface, npmVersion string) *NetworkPolicyManager {
	var serverVersion *version.Info
	var err error
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
		clientset:       clientset,
		informerFactory: informerFactory,
		podInformer:     informerFactory.Core().V1().Pods(),
		nsInformer:      informerFactory.Core().V1().Namespaces(),
		npInformer:      informerFactory.Networking().V1().NetworkPolicies(),
		ipsMgr:          ipsm.NewIpsetManager(exec),
		clusterState: telemetry.ClusterState{
			PodCount:      0,
			NsCount:       0,
			NwPolicyCount: 0,
		},
		serverVersion:    serverVersion,
		NodeName:         os.Getenv("HOSTNAME"),
		version:          npmVersion,
		TelemetryEnabled: true,
	}

	// create pod controller
	npMgr.podController = NewPodController(npMgr.podInformer, clientset, npMgr.ipsMgr)
	// create NameSpace controller
	npMgr.nameSpaceController = NewNameSpaceController(npMgr.nsInformer, clientset, npMgr.ipsMgr)
	// create network policy controller
	npMgr.netPolController = NewNetworkPolicyController(npMgr.npInformer, clientset, npMgr.ipsMgr)

	// It is important to keep the order between iptables and ipset
	// 1. first initialize iptables
	npMgr.netPolController.initializeIptables()

	// err = npMgr.netPolController.initializeIptables()
	// if err != nil {
	// 	klog.Info(err.Error())
	// 	panic(err.Error())
	// }

	// 2. then clear out leftover ipsets states
	log.Logf("Azure-NPM creating, cleaning existing Azure NPM IPSets")
	npMgr.ipsMgr.DestroyNpmIpsets()

	// err = npMgr.ipsMgr.DestroyNpmIpsets()
	// if err != nil {
	// 	klog.Info(err.Error())
	// 	panic(err.Error())
	// }

	return npMgr
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
// (QUESTION): Where is this function called?
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

		// (TODO): If it needs very accurate metrics, need lock for each one
		lenOfNsMap := npMgr.nameSpaceController.LengthOfNsMap()
		nsCount.Value = float64(lenOfNsMap)

		lenOfRawNpMap := npMgr.netPolController.LengthOfRawNpMap()
		nwPolicyCount.Value += float64(lenOfRawNpMap)

		podCount.Value = 0
		lenOfPodMap := npMgr.podController.LengthOfPodMap()
		podCount.Value += float64(lenOfPodMap)

		metrics.SendMetric(podCount)
		metrics.SendMetric(nsCount)
		metrics.SendMetric(nwPolicyCount)
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
	go npMgr.netPolController.runPeriodicTasks(stopCh)
	return nil
}
