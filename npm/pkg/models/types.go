// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package models

import (
	controllersv1 "github.com/Azure/azure-container-networking/npm/pkg/controlplane/controllers/v1"
	controllersv2 "github.com/Azure/azure-container-networking/npm/pkg/controlplane/controllers/v2"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	networkinginformers "k8s.io/client-go/informers/networking/v1"
)

var (
	ErrMarshalNPMCache     = errors.New("failed to marshal NPM Cache")
	ErrInformerSyncFailure = errors.New("informer sync failure")
)

// Cache is the cache lookup key for the NPM cache
type CacheKey string

// K8SControllerV1 are the legacy k8s controllers
type K8SControllersV1 struct {
	PodControllerV1       *controllersv1.PodController           //nolint:structcheck //ignore this error
	NamespaceControllerV1 *controllersv1.NamespaceController     //nolint:structcheck // false lint error
	NpmNamespaceCacheV1   *controllersv1.NpmNamespaceCache       //nolint:structcheck // false lint error
	NetPolControllerV1    *controllersv1.NetworkPolicyController //nolint:structcheck // false lint error
}

// K8SControllerV2 are the optimized k8s controllers that replace the legacy controllers
type K8SControllersV2 struct {
	PodControllerV2       *controllersv2.PodController           //nolint:structcheck //ignore this error
	NamespaceControllerV2 *controllersv2.NamespaceController     //nolint:structcheck // false lint error
	NpmNamespaceCacheV2   *controllersv2.NpmNamespaceCache       //nolint:structcheck // false lint error
	NetPolControllerV2    *controllersv2.NetworkPolicyController //nolint:structcheck // false lint error
}

// Informers are the informers for the k8s controllers
type Informers struct {
	InformerFactory informers.SharedInformerFactory           //nolint:structcheck //ignore this error
	PodInformer     coreinformers.PodInformer                 //nolint:structcheck // false lint error
	NsInformer      coreinformers.NamespaceInformer           //nolint:structcheck // false lint error
	NpInformer      networkinginformers.NetworkPolicyInformer //nolint:structcheck // false lint error
}

// AzureConfig captures the Azure specific configurations and fields
type AzureConfig struct {
	K8sServerVersion *version.Info
	NodeName         string
	Version          string
	TelemetryEnabled bool
}
