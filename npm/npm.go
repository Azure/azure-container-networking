// Package npm Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/Azure/azure-container-networking/aitelemetry"
	npmconfig "github.com/Azure/azure-container-networking/npm/config"
	"github.com/Azure/azure-container-networking/npm/ipsm"
	"github.com/Azure/azure-container-networking/npm/metrics"
	controllersv1 "github.com/Azure/azure-container-networking/npm/pkg/controlplane/controllers/v1"
	controllersv2 "github.com/Azure/azure-container-networking/npm/pkg/controlplane/controllers/v2"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane"
	"github.com/Azure/azure-container-networking/npm/util"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	networkinginformers "k8s.io/client-go/informers/networking/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
	utilexec "k8s.io/utils/exec"
)

var (
	aiMetadata         string
	errMarshalNPMCache = errors.New("failed to marshal NPM Cache")
)

const (
	heartbeatIntervalInMinutes = 30
	// TODO: consider increasing thread number later when logics are correct
	// threadness = 1
)

type CacheKey string

// NPMCache Key Contract for Json marshal and unmarshal
const (
	NodeName CacheKey = "NodeName"
	NsMap    CacheKey = "NsMap"
	PodMap   CacheKey = "PodMap"
	ListMap  CacheKey = "ListMap"
	SetMap   CacheKey = "SetMap"
)

// NetworkPolicyManager contains informers for pod, namespace and networkpolicy.
type NetworkPolicyManager struct {
	config npmconfig.Config

	informerFactory informers.SharedInformerFactory
	podInformer     coreinformers.PodInformer
	nsInformer      coreinformers.NamespaceInformer

	// V1 controllers (to be deprecated)
	podControllerV1       *controllersv1.PodController
	namespaceControllerV1 *controllersv1.NamespaceController
	npmNamespaceCacheV1   *controllersv1.NpmNamespaceCache

	// V2 controllers
	podControllerV2       *controllersv2.PodController
	namespaceControllerV2 *controllersv2.NamespaceController
	npmNamespaceCacheV2   *controllersv2.NpmNamespaceCache

	npInformer       networkinginformers.NetworkPolicyInformer
	netPolController *networkPolicyController

	// ipsMgr are shared in all controllers. Thus, only one ipsMgr is created for simple management
	// and uses lock to avoid unintentional race condictions in IpsetManager.
	ipsMgr *ipsm.IpsetManager
	// Azure-specific variables
	k8sServerVersion *version.Info
	NodeName         string
	version          string
	TelemetryEnabled bool
}

// NewNetworkPolicyManager creates a NetworkPolicyManager
func NewNetworkPolicyManager(config npmconfig.Config,
	informerFactory informers.SharedInformerFactory,
	dp dataplane.GenericDataplane,
	exec utilexec.Interface,
	npmVersion string,
	k8sServerVersion *version.Info) *NetworkPolicyManager {
	klog.Infof("API server version: %+v ai meta data %+v", k8sServerVersion, aiMetadata)

	npMgr := &NetworkPolicyManager{
		config:              config,
		informerFactory:     informerFactory,
		podInformer:         informerFactory.Core().V1().Pods(),
		nsInformer:          informerFactory.Core().V1().Namespaces(),
		npInformer:          informerFactory.Networking().V1().NetworkPolicies(),
		ipsMgr:              ipsm.NewIpsetManager(exec),
		npmNamespaceCacheV1: &controllersv1.NpmNamespaceCache{NsMap: make(map[string]*controllersv1.Namespace)},
		k8sServerVersion:    k8sServerVersion,
		NodeName:            os.Getenv("HOSTNAME"),
		version:             npmVersion,
		TelemetryEnabled:    true,
	}

	if npMgr.config.Toggles.EnableV2Controllers {
		// create pod controller
		npMgr.podControllerV2 = controllersv2.NewPodController(npMgr.podInformer, dp, npMgr.npmNamespaceCacheV2)
		// create NameSpace controller
		npMgr.namespaceControllerV2 = controllersv2.NewNamespaceController(npMgr.nsInformer, dp, npMgr.npmNamespaceCacheV2)
		return npMgr
	}

	// create pod controller
	npMgr.podControllerV1 = controllersv1.NewPodController(npMgr.podInformer, npMgr.ipsMgr, npMgr.npmNamespaceCacheV1)
	// create NameSpace controller
	npMgr.namespaceControllerV1 = controllersv1.NewNameSpaceController(npMgr.nsInformer, npMgr.ipsMgr, npMgr.npmNamespaceCacheV1)
	// create network policy controller
	npMgr.netPolController = NewNetworkPolicyController(npMgr.npInformer, npMgr.ipsMgr)

	return npMgr
}

func (npMgr *NetworkPolicyManager) MarshalJSON() ([]byte, error) {
	m := map[CacheKey]json.RawMessage{}

	npmNamespaceCacheRaw, err := json.Marshal(npMgr.npmNamespaceCacheV1)
	if err != nil {
		return nil, errors.Errorf("%s: %v", errMarshalNPMCache, err)
	}
	m[NsMap] = npmNamespaceCacheRaw

	podControllerRaw, err := json.Marshal(npMgr.podControllerV1)
	if err != nil {
		return nil, errors.Errorf("%s: %v", errMarshalNPMCache, err)
	}
	m[PodMap] = podControllerRaw

	if npMgr.ipsMgr != nil {
		listMapRaw, listMapMarshalErr := npMgr.ipsMgr.MarshalListMapJSON()
		if listMapMarshalErr != nil {
			return nil, errors.Errorf("%s: %v", errMarshalNPMCache, listMapMarshalErr)
		}
		m[ListMap] = listMapRaw

		setMapRaw, setMapMarshalErr := npMgr.ipsMgr.MarshalSetMapJSON()
		if setMapMarshalErr != nil {
			return nil, errors.Errorf("%s: %v", errMarshalNPMCache, setMapMarshalErr)
		}
		m[SetMap] = setMapRaw
	}

	nodeNameRaw, err := json.Marshal(npMgr.NodeName)
	if err != nil {
		return nil, errors.Errorf("%s: %v", errMarshalNPMCache, err)
	}
	m[NodeName] = nodeNameRaw

	npmCacheRaw, err := json.Marshal(m)
	if err != nil {
		return nil, errors.Errorf("%s: %v", errMarshalNPMCache, err)
	}

	return npmCacheRaw, nil
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
// TODO(jungukcho): need to move codes into metrics packages
func (npMgr *NetworkPolicyManager) SendClusterMetrics() {
	var (
		heartbeat        = time.NewTicker(time.Minute * heartbeatIntervalInMinutes).C
		customDimensions = map[string]string{
			"ClusterID": util.GetClusterID(npMgr.NodeName),
			"APIServer": npMgr.k8sServerVersion.String(),
		}
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

		// Reducing one to remove all-namespaces ns obj
		lenOfNsMap := len(npMgr.npmNamespaceCacheV1.NsMap)
		nsCount.Value = float64(lenOfNsMap - 1)

		lenOfRawNpMap := npMgr.netPolController.lengthOfRawNpMap()
		nwPolicyCount.Value += float64(lenOfRawNpMap)

		lenOfPodMap := npMgr.podControllerV1.LengthOfPodMap()
		podCount.Value += float64(lenOfPodMap)

		metrics.SendMetric(podCount)
		metrics.SendMetric(nsCount)
		metrics.SendMetric(nwPolicyCount)
	}
}

// Start starts shared informers and waits for the shared informer cache to sync.
func (npMgr *NetworkPolicyManager) Start(config npmconfig.Config, stopCh <-chan struct{}) error {
	// Do initialization of data plane before starting syncup of each controller to avoid heavy call to api-server
	if err := npMgr.netPolController.resetDataPlane(); err != nil {
		return fmt.Errorf("Failed to initialized data plane")
	}

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
		return fmt.Errorf("Network policy informer failed to sync")
	}

	if config.Toggles.EnableV2Controllers {
		go npMgr.podControllerV2.Run(stopCh)
		go npMgr.namespaceControllerV2.Run(stopCh)
		go npMgr.netPolController.Run(stopCh)
		go npMgr.netPolController.runPeriodicTasks(stopCh)
		return nil
	}

	// start controllers after synced
	go npMgr.podControllerV1.Run(stopCh)
	go npMgr.namespaceControllerV1.Run(stopCh)
	go npMgr.netPolController.Run(stopCh)
	go npMgr.netPolController.runPeriodicTasks(stopCh)

	return nil
}
