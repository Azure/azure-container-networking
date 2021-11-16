// Copyright 2017 Microsoft. All rights reserved.
// MIT License

//go:build windows
// +build windows

package hnswrapper

import (
	"encoding/json"
	"reflect"
	"sync"

	"github.com/Microsoft/hcsshim/hcn"
)

type FakeHNSCache struct {
	networks  map[string]*hcn.HostComputeNetwork
	endpoints map[string]*hcn.HostComputeEndpoint
}

func (fCache FakeHNSCache) SetPolicyExists(setId string) bool {
	for _, network := range fCache.networks {
		for _, policy := range network.Policies {
			var setPolSettings hcn.SetPolicySetting
			err := json.Unmarshal(policy.Settings, &setPolSettings)
			if err != nil {
				return false
			}
			if setPolSettings.Name == setId {
				return true
			}
		}
	}
	return false
}

type Hnsv2wrapperFake struct {
	Cache FakeHNSCache
	sync.Mutex
}

func NewHnsv2wrapperFake() *Hnsv2wrapperFake {
	return &Hnsv2wrapperFake{
		Cache: FakeHNSCache{
			networks:  map[string]*hcn.HostComputeNetwork{},
			endpoints: map[string]*hcn.HostComputeEndpoint{},
		},
	}
}

func (f Hnsv2wrapperFake) CreateNetwork(network *hcn.HostComputeNetwork) (*hcn.HostComputeNetwork, error) {
	f.Lock()
	defer f.Unlock()

	f.Cache.networks[network.Name] = network
	return network, nil
}

func (f Hnsv2wrapperFake) DeleteNetwork(network *hcn.HostComputeNetwork) error {
	return nil
}

func (f Hnsv2wrapperFake) ModifyNetworkSettings(network *hcn.HostComputeNetwork, request *hcn.ModifyNetworkSettingRequest) error {
	f.Lock()
	defer f.Unlock()
	switch request.RequestType {
	case hcn.RequestTypeAdd:
		var setPolSettings hcn.PolicyNetworkRequest
		err := json.Unmarshal(request.Settings, &setPolSettings)
		if err != nil {
			return err
		}
		for _, setPolSetting := range setPolSettings.Policies {
			if setPolSetting.Type == hcn.SetPolicy {
				network.Policies = append(network.Policies, setPolSetting)
			}
		}
	case hcn.RequestTypeRemove:
		newtempPolicies := network.Policies
		var setPolSettings hcn.PolicyNetworkRequest
		err := json.Unmarshal(request.Settings, &setPolSettings)
		if err != nil {
			return err
		}
		for i, policy := range network.Policies {
			for _, newPolicy := range setPolSettings.Policies {
				if policy.Type != newPolicy.Type {
					continue
				}
				if reflect.DeepEqual(policy.Settings, newPolicy.Settings) {
					newtempPolicies = append(newtempPolicies[:i], newtempPolicies[i+1:]...)
					break
				}
			}
		}
		network.Policies = newtempPolicies
	case hcn.RequestTypeUpdate:
		network.Policies = []hcn.NetworkPolicy{}
		var setPolSettings hcn.PolicyNetworkRequest
		err := json.Unmarshal(request.Settings, &setPolSettings)
		if err != nil {
			return err
		}
		for _, setPolSetting := range setPolSettings.Policies {
			if setPolSetting.Type == hcn.SetPolicy {
				network.Policies = append(network.Policies, setPolSetting)
			}
		}
	}

	return nil
}

func (Hnsv2wrapperFake) AddNetworkPolicy(network *hcn.HostComputeNetwork, networkPolicy hcn.PolicyNetworkRequest) error {
	return nil
}

func (Hnsv2wrapperFake) RemoveNetworkPolicy(network *hcn.HostComputeNetwork, networkPolicy hcn.PolicyNetworkRequest) error {
	return nil
}

func (f Hnsv2wrapperFake) GetNetworkByName(networkName string) (*hcn.HostComputeNetwork, error) {
	f.Lock()
	defer f.Unlock()
	if network, ok := f.Cache.networks[networkName]; ok {
		return network, nil
	}
	return &hcn.HostComputeNetwork{}, nil
}

func (f Hnsv2wrapperFake) GetNetworkByID(networkID string) (*hcn.HostComputeNetwork, error) {
	f.Lock()
	defer f.Unlock()
	for _, network := range f.Cache.networks {
		if network.Id == networkID {
			return network, nil
		}
	}
	return &hcn.HostComputeNetwork{}, nil
}

func (f Hnsv2wrapperFake) GetEndpointByID(endpointID string) (*hcn.HostComputeEndpoint, error) {
	f.Lock()
	defer f.Unlock()
	if ep, ok := f.Cache.endpoints[endpointID]; ok {
		return ep, nil
	}
	return &hcn.HostComputeEndpoint{}, nil
}

func (f Hnsv2wrapperFake) CreateEndpoint(endpoint *hcn.HostComputeEndpoint) (*hcn.HostComputeEndpoint, error) {
	f.Lock()
	defer f.Unlock()
	f.Cache.endpoints[endpoint.Id] = endpoint
	return endpoint, nil
}

func (f Hnsv2wrapperFake) DeleteEndpoint(endpoint *hcn.HostComputeEndpoint) error {
	f.Lock()
	defer f.Unlock()
	delete(f.Cache.endpoints, endpoint.Id)
	return nil
}

func (Hnsv2wrapperFake) GetNamespaceByID(netNamespacePath string) (*hcn.HostComputeNamespace, error) {
	nameSpace := &hcn.HostComputeNamespace{Id: "ea37ac15-119e-477b-863b-cc23d6eeaa4d", NamespaceId: 1000}
	return nameSpace, nil
}

func (Hnsv2wrapperFake) AddNamespaceEndpoint(namespaceId string, endpointId string) error {
	return nil
}

func (Hnsv2wrapperFake) RemoveNamespaceEndpoint(namespaceId string, endpointId string) error {
	return nil
}

func (f Hnsv2wrapperFake) ListEndpointsOfNetwork(networkId string) ([]hcn.HostComputeEndpoint, error) {
	f.Lock()
	defer f.Unlock()
	endpoints := make([]hcn.HostComputeEndpoint, 0)
	for _, endpoint := range f.Cache.endpoints {
		if endpoint.HostComputeNetwork == networkId {
			endpoints = append(endpoints, *endpoint)
		}
	}
	return endpoints, nil
}

func (f Hnsv2wrapperFake) ApplyEndpointPolicy(endpoint *hcn.HostComputeEndpoint, requestType hcn.RequestType, endpointPolicy hcn.PolicyEndpointRequest) error {
	f.Lock()
	defer f.Unlock()
	switch requestType {
	case hcn.RequestTypeAdd:
		for _, newPolicy := range endpointPolicy.Policies {
			if newPolicy.Type != hcn.ACL {
				continue
			}
			endpoint.Policies = append(endpoint.Policies, newPolicy)
		}
	case hcn.RequestTypeRemove:
		newtempPolicies := endpoint.Policies
		for i, policy := range endpoint.Policies {
			for _, newPolicy := range endpointPolicy.Policies {
				if policy.Type != newPolicy.Type {
					continue
				}
				if reflect.DeepEqual(policy.Settings, newPolicy.Settings) {
					newtempPolicies = append(newtempPolicies[:i], newtempPolicies[i+1:]...)
					break
				}
			}
		}
		endpoint.Policies = newtempPolicies
	case hcn.RequestTypeUpdate:
		endpoint.Policies = make([]hcn.EndpointPolicy, 0)
		for _, newPolicy := range endpointPolicy.Policies {
			if newPolicy.Type != hcn.ACL {
				continue
			}
			endpoint.Policies = append(endpoint.Policies, newPolicy)
		}
	}

	return nil
}
