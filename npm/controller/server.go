// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package controller

import (
	"encoding/json"
	"fmt"

	npmconfig "github.com/Azure/azure-container-networking/npm/config"
	"github.com/Azure/azure-container-networking/npm/pkg/controlplane/controllers/common"
	controllersv2 "github.com/Azure/azure-container-networking/npm/pkg/controlplane/controllers/v2"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane"
	"github.com/Azure/azure-container-networking/npm/pkg/models"
	"github.com/Azure/azure-container-networking/npm/pkg/transport"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
)

var aiMetadata string //nolint // aiMetadata is set in Makefile

type NetworkPolicyServer struct {
	config npmconfig.Config

	// tm is the transport layer (gRPC) manager/server
	tm *transport.EventsServer

	// Informers are the Kubernetes Informer
	// https://pkg.go.dev/k8s.io/client-go/informers
	models.Informers

	// Controllers for handling Kubernetes resource watcher events
	models.K8SControllersV2

	// Azure-specific variables
	models.AzureConfig
}

var (
	ErrInformerFactoryNil      = errors.New("informer factory is nil")
	ErrTransportManagerNil     = errors.New("transport manager is nil")
	ErrK8SServerVersionNil     = errors.New("k8s server version is nil")
	ErrDataplaneNotInitialized = errors.New("dataplane is not initialized")
)

func NewNetworkPolicyServer(
	config npmconfig.Config,
	informerFactory informers.SharedInformerFactory,
	mgr *transport.EventsServer,
	dp dataplane.GenericDataplane,
	npmVersion string,
	k8sServerVersion *version.Info,
) (*NetworkPolicyServer, error) {
	klog.Infof("API server version: %+v AI metadata %+v", k8sServerVersion, aiMetadata)

	if informerFactory == nil {
		return nil, ErrInformerFactoryNil
	}

	if mgr == nil {
		return nil, ErrTransportManagerNil
	}

	if dp == nil {
		return nil, ErrDataplaneNotInitialized
	}

	if k8sServerVersion == nil {
		return nil, ErrK8SServerVersionNil
	}

	n := &NetworkPolicyServer{
		config: config,
		tm:     mgr,
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

	n.NpmNamespaceCacheV2 = &controllersv2.NpmNamespaceCache{NsMap: make(map[string]*common.Namespace)}
	n.PodControllerV2 = controllersv2.NewPodController(n.PodInformer, dp, n.NpmNamespaceCacheV2)
	n.NamespaceControllerV2 = controllersv2.NewNamespaceController(n.NsInformer, dp, n.NpmNamespaceCacheV2)
	n.NetPolControllerV2 = controllersv2.NewNetworkPolicyController(n.NpInformer, dp)

	return n, nil
}

func (n *NetworkPolicyServer) MarshalJSON() ([]byte, error) {
	m := map[models.CacheKey]json.RawMessage{}

	var npmNamespaceCacheRaw []byte
	var err error
	npmNamespaceCacheRaw, err = json.Marshal(n.NpmNamespaceCacheV2)

	if err != nil {
		return nil, errors.Errorf("%s: %v", models.ErrMarshalNPMCache, err)
	}
	m[models.NsMap] = npmNamespaceCacheRaw

	var podControllerRaw []byte
	podControllerRaw, err = json.Marshal(n.PodControllerV2)

	if err != nil {
		return nil, errors.Errorf("%s: %v", models.ErrMarshalNPMCache, err)
	}
	m[models.PodMap] = podControllerRaw

	nodeNameRaw, err := json.Marshal(n.NodeName)
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

func (n *NetworkPolicyServer) GetAppVersion() string {
	return n.Version
}

func (n *NetworkPolicyServer) Start(config npmconfig.Config, stopCh <-chan struct{}) error {
	// Starts all informers manufactured by n's InformerFactory.
	n.InformerFactory.Start(stopCh)

	// Wait for the initial sync of local cache.
	if !cache.WaitForCacheSync(stopCh, n.PodInformer.Informer().HasSynced) {
		return fmt.Errorf("Pod informer error: %w", models.ErrInformerSyncFailure)
	}

	if !cache.WaitForCacheSync(stopCh, n.NsInformer.Informer().HasSynced) {
		return fmt.Errorf("Namespace informer error: %w", models.ErrInformerSyncFailure)
	}

	if !cache.WaitForCacheSync(stopCh, n.NpInformer.Informer().HasSynced) {
		return fmt.Errorf("NetworkPolicy informer error: %w", models.ErrInformerSyncFailure)
	}

	// start v2 NPM controllers after synced
	go n.PodControllerV2.Run(stopCh)
	go n.NamespaceControllerV2.Run(stopCh)
	go n.NetPolControllerV2.Run(stopCh)

	// start the transport layer (gRPC) server
	// We block the main thread here until the server is stopped.
	// This is unlike the other start methods in this package, which returns nil
	// and blocks in the main thread during command invocation through the select {}
	// statement.
	return n.tm.Start(stopCh) //nolint:wrapcheck // ignore: can't use n.tm.Start() directly
}
