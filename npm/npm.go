// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"encoding/json"
	"fmt"

	npmconfig "github.com/Azure/azure-container-networking/npm/config"
	"github.com/Azure/azure-container-networking/npm/ipsm"
	controllersv1 "github.com/Azure/azure-container-networking/npm/pkg/controlplane/controllers/v1"
	controllersv2 "github.com/Azure/azure-container-networking/npm/pkg/controlplane/controllers/v2"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane"
	"github.com/Azure/azure-container-networking/npm/pkg/models"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
	utilexec "k8s.io/utils/exec"
)

var aiMetadata string //nolint // aiMetadata is set in Makefile

// NetworkPolicyManager contains informers for pod, namespace and networkpolicy.
type NetworkPolicyManager struct {
	config npmconfig.Config

	// ipsMgr are shared in all controllers. Thus, only one ipsMgr is created for simple management
	// and uses lock to avoid unintentional race condictions in IpsetManager.
	ipsMgr *ipsm.IpsetManager

	// Informers are the Kubernetes Informer
	// https://pkg.go.dev/k8s.io/client-go/informers
	models.Informers

	// Legacy controllers for handling Kubernetes resource watcher events
	// To be deprecated
	models.K8SControllersV1

	// Controllers for handling Kubernetes resource watcher events
	models.K8SControllersV2

	// Azure-specific variables
	models.AzureConfig
}

// NewNetworkPolicyManager creates a NetworkPolicyManager
func NewNetworkPolicyManager(config npmconfig.Config,
	informerFactory informers.SharedInformerFactory,
	dp dataplane.GenericDataplane,
	exec utilexec.Interface,
	npmVersion string,
	k8sServerVersion *version.Info) *NetworkPolicyManager {
	klog.Infof("API server version: %+v AI metadata %+v", k8sServerVersion, aiMetadata)

	npMgr := &NetworkPolicyManager{
		config: config,
		Informers: models.Informers{
			InformerFactory: informerFactory,
			PodInformer:     informerFactory.Core().V1().Pods(),
			NsInformer:      informerFactory.Core().V1().Namespaces(),
			NpInformer:      informerFactory.Networking().V1().NetworkPolicies(),
		},
		AzureConfig: models.AzureConfig{
			K8sServerVersion: k8sServerVersion,
			NodeName:         models.GetNodeName(),
			Version:          npmVersion,
			TelemetryEnabled: true,
		},
	}

	// create v2 NPM specific components.
	if npMgr.config.Toggles.EnableV2NPM {
		npMgr.NpmNamespaceCacheV2 = &controllersv2.NpmNamespaceCache{NsMap: make(map[string]*controllersv2.Namespace)}
		npMgr.PodControllerV2 = controllersv2.NewPodController(npMgr.PodInformer, dp, npMgr.NpmNamespaceCacheV2)
		npMgr.NamespaceControllerV2 = controllersv2.NewNamespaceController(npMgr.NsInformer, dp, npMgr.NpmNamespaceCacheV2)
		// Question(jungukcho): Is config.Toggles.PlaceAzureChainFirst needed for v2?
		npMgr.NetPolControllerV2 = controllersv2.NewNetworkPolicyController(npMgr.NpInformer, dp)
		return npMgr
	}

	// create v1 NPM specific components.
	npMgr.ipsMgr = ipsm.NewIpsetManager(exec)

	npMgr.NpmNamespaceCacheV1 = &controllersv1.NpmNamespaceCache{NsMap: make(map[string]*controllersv1.Namespace)}
	npMgr.PodControllerV1 = controllersv1.NewPodController(npMgr.PodInformer, npMgr.ipsMgr, npMgr.NpmNamespaceCacheV1)
	npMgr.NamespaceControllerV1 = controllersv1.NewNameSpaceController(npMgr.NsInformer, npMgr.ipsMgr, npMgr.NpmNamespaceCacheV1)
	npMgr.NetPolControllerV1 = controllersv1.NewNetworkPolicyController(npMgr.NpInformer, npMgr.ipsMgr, config.Toggles.PlaceAzureChainFirst)
	return npMgr
}

func (npMgr *NetworkPolicyManager) MarshalJSON() ([]byte, error) {
	m := map[models.CacheKey]json.RawMessage{}

	var npmNamespaceCacheRaw []byte
	var err error
	if npMgr.config.Toggles.EnableV2NPM {
		npmNamespaceCacheRaw, err = json.Marshal(npMgr.NpmNamespaceCacheV2)
	} else {
		npmNamespaceCacheRaw, err = json.Marshal(npMgr.NpmNamespaceCacheV1)
	}

	if err != nil {
		return nil, errors.Errorf("%s: %v", models.ErrMarshalNPMCache, err)
	}
	m[models.NsMap] = npmNamespaceCacheRaw

	var podControllerRaw []byte
	if npMgr.config.Toggles.EnableV2NPM {
		podControllerRaw, err = json.Marshal(npMgr.PodControllerV2)
	} else {
		podControllerRaw, err = json.Marshal(npMgr.PodControllerV1)
	}

	if err != nil {
		return nil, errors.Errorf("%s: %v", models.ErrMarshalNPMCache, err)
	}
	m[models.PodMap] = podControllerRaw

	// TODO(jungukcho): NPM debug may be broken.
	// Will fix it later after v2 controller and linux test if it is broken.
	if !npMgr.config.Toggles.EnableV2NPM && npMgr.ipsMgr != nil {
		listMapRaw, listMapMarshalErr := npMgr.ipsMgr.MarshalListMapJSON()
		if listMapMarshalErr != nil {
			return nil, errors.Errorf("%s: %v", models.ErrMarshalNPMCache, listMapMarshalErr)
		}
		m[models.ListMap] = listMapRaw

		setMapRaw, setMapMarshalErr := npMgr.ipsMgr.MarshalSetMapJSON()
		if setMapMarshalErr != nil {
			return nil, errors.Errorf("%s: %v", models.ErrMarshalNPMCache, setMapMarshalErr)
		}
		m[models.SetMap] = setMapRaw
	}

	nodeNameRaw, err := json.Marshal(npMgr.NodeName)
	if err != nil {
		return nil, errors.Errorf("%s: %v", models.ErrMarshalNPMCache, err)
	}
	m[models.NodeName] = nodeNameRaw

	npmCacheRaw, err := json.Marshal(m)
	if err != nil {
		return nil, errors.Errorf("%s: %v", models.ErrMarshalNPMCache, err)
	}

	return npmCacheRaw, nil
}

// GetAppVersion returns network policy manager app version
func (npMgr *NetworkPolicyManager) GetAppVersion() string {
	return npMgr.Version
}

// Start starts shared informers and waits for the shared informer cache to sync.
func (npMgr *NetworkPolicyManager) Start(config npmconfig.Config, stopCh <-chan struct{}) error {
	if !config.Toggles.EnableV2NPM {
		// Do initialization of data plane before starting syncup of each controller to avoid heavy call to api-server
		if err := npMgr.NetPolControllerV1.BootupDataplane(); err != nil {
			return fmt.Errorf("Failed to initialized data plane with err %w", err)
		}
	}

	// Starts all informers manufactured by npMgr's informerFactory.
	npMgr.InformerFactory.Start(stopCh)

	// Wait for the initial sync of local cache.
	if !cache.WaitForCacheSync(stopCh, npMgr.PodInformer.Informer().HasSynced) {
		return fmt.Errorf("Pod informer error: %w", models.ErrInformerSyncFailure)
	}

	if !cache.WaitForCacheSync(stopCh, npMgr.NsInformer.Informer().HasSynced) {
		return fmt.Errorf("Namespace informer error: %w", models.ErrInformerSyncFailure)
	}

	if !cache.WaitForCacheSync(stopCh, npMgr.NpInformer.Informer().HasSynced) {
		return fmt.Errorf("NetworkPolicy informer error: %w", models.ErrInformerSyncFailure)
	}

	// start v2 NPM controllers after synced
	if config.Toggles.EnableV2NPM {
		go npMgr.PodControllerV2.Run(stopCh)
		go npMgr.NamespaceControllerV2.Run(stopCh)
		go npMgr.NetPolControllerV2.Run(stopCh)
		return nil
	}

	// start v1 NPM controllers after synced
	go npMgr.PodControllerV1.Run(stopCh)
	go npMgr.NamespaceControllerV1.Run(stopCh)
	go npMgr.NetPolControllerV1.Run(stopCh)
	go npMgr.NetPolControllerV1.RunPeriodicTasks(stopCh)

	return nil
}

// GetAIMetadata returns ai metadata number
func GetAIMetadata() string {
	return aiMetadata
}
