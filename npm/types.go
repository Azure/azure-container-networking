// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	npmconfig "github.com/Azure/azure-container-networking/npm/config"
	"github.com/Azure/azure-container-networking/npm/ipsm"
	controllersv1 "github.com/Azure/azure-container-networking/npm/pkg/controlplane/controllers/v1"
	controllersv2 "github.com/Azure/azure-container-networking/npm/pkg/controlplane/controllers/v2"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	networkinginformers "k8s.io/client-go/informers/networking/v1"
)

var (
	aiMetadata         string
	errMarshalNPMCache = errors.New("failed to marshal NPM Cache")
)

// NetworkPolicyManager contains informers for pod, namespace and networkpolicy.
type NetworkPolicyManager struct {
	config npmconfig.Config

	// ipsMgr are shared in all controllers. Thus, only one ipsMgr is created for simple management
	// and uses lock to avoid unintentional race condictions in IpsetManager.
	ipsMgr *ipsm.IpsetManager

	// Informers are the Kubernetes Informer
	// https://pkg.go.dev/k8s.io/client-go/informers
	Informers

	// Legacy controllers for handling Kubernetes resource watcher events
	// To be deprecated
	K8SControllersV1

	// Controllers for handling Kubernetes resource watcher events
	K8SControllersV2

	// Azure-specific variables
	AzureConfig
}

// Cache is the cache lookup key for the NPM cache
type CacheKey string

// K8SControllerV1 are the legacy k8s controllers
type K8SControllersV1 struct {
	podControllerV1       *controllersv1.PodController           //nolint:structcheck //ignore this error
	namespaceControllerV1 *controllersv1.NamespaceController     //nolint:structcheck // false lint error
	npmNamespaceCacheV1   *controllersv1.NpmNamespaceCache       //nolint:structcheck // false lint error
	netPolControllerV1    *controllersv1.NetworkPolicyController //nolint:structcheck // false lint error
}

// K8SControllerV2 are the optimized k8s controllers that replace the legacy controllers
type K8SControllersV2 struct {
	podControllerV2       *controllersv2.PodController           //nolint:structcheck //ignore this error
	namespaceControllerV2 *controllersv2.NamespaceController     //nolint:structcheck // false lint error
	npmNamespaceCacheV2   *controllersv2.NpmNamespaceCache       //nolint:structcheck // false lint error
	netPolControllerV2    *controllersv2.NetworkPolicyController //nolint:structcheck // false lint error
}

// Informers are the informers for the k8s controllers
type Informers struct {
	informerFactory informers.SharedInformerFactory           //nolint:structcheck //ignore this error
	podInformer     coreinformers.PodInformer                 //nolint:structcheck // false lint error
	nsInformer      coreinformers.NamespaceInformer           //nolint:structcheck // false lint error
	npInformer      networkinginformers.NetworkPolicyInformer //nolint:structcheck // false lint error
}

// AzureConfig captures the Azure specific configurations and fields
type AzureConfig struct {
	k8sServerVersion *version.Info
	NodeName         string
	version          string
	TelemetryEnabled bool
}
