// Copyright 2017 Microsoft. All rights reserved.
// MIT License

//go:build windows
// +build windows

package hnswrapper

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/Microsoft/hcsshim/hcn"
)

const networkName = "azure"

var errorFakeHNS = errors.New("errorFakeHNS Error")

func newErrorFakeHNS(errStr string) error {
	return fmt.Errorf("%w : %s", errorFakeHNS, errStr)
}

type Hnsv2wrapperFake struct {
	Cache FakeHNSCache
	*sync.Mutex
	Delay time.Duration
}

func NewHnsv2wrapperFake() *Hnsv2wrapperFake {
	return &Hnsv2wrapperFake{
		Mutex: &sync.Mutex{},
		Cache: FakeHNSCache{
			networks:  map[string]*FakeHostComputeNetwork{},
			endpoints: map[string]*FakeHostComputeEndpoint{},
		},
	}
}

func delayHnsCall(delay time.Duration) {
	time.Sleep(delay)
}

// NewMockIOShim is dependent on this function never returning an error
func (f Hnsv2wrapperFake) CreateNetwork(network *hcn.HostComputeNetwork) (*hcn.HostComputeNetwork, error) {
	f.Lock()
	defer f.Unlock()

	delayHnsCall(f.Delay)
	f.Cache.networks[network.Name] = NewFakeHostComputeNetwork(network)
	return network, nil
}

func (f Hnsv2wrapperFake) DeleteNetwork(network *hcn.HostComputeNetwork) error {
	delayHnsCall(f.Delay)
	return nil
}

func (f Hnsv2wrapperFake) ModifyNetworkSettings(network *hcn.HostComputeNetwork, request *hcn.ModifyNetworkSettingRequest) error {
	f.Lock()
	defer f.Unlock()

	delayHnsCall(f.Delay)

	networkCache, ok := f.Cache.networks[network.Name]
	if !ok {
		return nil
	}
	switch request.RequestType {
	case hcn.RequestTypeAdd:
		var setPolSettings hcn.PolicyNetworkRequest
		err := json.Unmarshal(request.Settings, &setPolSettings)
		if err != nil {
			return newErrorFakeHNS(err.Error())
		}
		for _, setPolSetting := range setPolSettings.Policies {
			if setPolSetting.Type == hcn.SetPolicy {
				var setpol hcn.SetPolicySetting
				err := json.Unmarshal(setPolSetting.Settings, &setpol)
				if err != nil {
					return newErrorFakeHNS(err.Error())
				}
				if setpol.PolicyType != hcn.SetPolicyTypeIpSet {
					// Check Nested SetPolicy members
					// checking for the case of no members in nested policy. iMgrCfg.AddEmptySetToLists is set to false in some tests so it creates a nested policy with no members
					if setpol.Values != "" {
						members := strings.Split(setpol.Values, ",")
						for _, memberID := range members {
							_, ok := networkCache.Policies[memberID]
							if !ok {
								return newErrorFakeHNS(fmt.Sprintf("Member Policy %s not found", memberID))
							}
						}
					}
				}
				networkCache.Policies[setpol.Id] = &setpol
			}
		}
	case hcn.RequestTypeRemove:
		var setPolSettings hcn.PolicyNetworkRequest
		err := json.Unmarshal(request.Settings, &setPolSettings)
		if err != nil {
			return newErrorFakeHNS(err.Error())
		}
		for _, newPolicy := range setPolSettings.Policies {
			var setpol hcn.SetPolicySetting
			err := json.Unmarshal(newPolicy.Settings, &setpol)
			if err != nil {
				return newErrorFakeHNS(err.Error())
			}
			if _, ok := networkCache.Policies[setpol.Id]; !ok {
				return newErrorFakeHNS(fmt.Sprintf("[FakeHNS] could not find %s ipset", setpol.Name))
			}
			if setpol.PolicyType == hcn.SetPolicyTypeIpSet {
				// For 1st level sets check if they are being referred by nested sets
				for _, cacheSet := range networkCache.Policies {
					if cacheSet.PolicyType == hcn.SetPolicyTypeIpSet {
						continue
					}
					if strings.Contains(cacheSet.Values, setpol.Id) {
						return newErrorFakeHNS(fmt.Sprintf("Set %s is being referred by another %s set", setpol.Name, cacheSet.Name))
					}
				}
			}
			delete(networkCache.Policies, setpol.Id)
		}
	case hcn.RequestTypeUpdate:
		var setPolSettings hcn.PolicyNetworkRequest
		err := json.Unmarshal(request.Settings, &setPolSettings)
		if err != nil {
			return newErrorFakeHNS(err.Error())
		}
		for _, newPolicy := range setPolSettings.Policies {
			var setpol hcn.SetPolicySetting
			err := json.Unmarshal(newPolicy.Settings, &setpol)
			if err != nil {
				return newErrorFakeHNS(err.Error())
			}
			if _, ok := networkCache.Policies[setpol.Id]; !ok {
				return newErrorFakeHNS(fmt.Sprintf("[FakeHNS] could not find %s ipset", setpol.Name))
			}
			_, ok := networkCache.Policies[setpol.Id]
			if !ok {
				// Replicating HNS behavior, we will not update non-existent set policy
				continue
			}
			if setpol.PolicyType != hcn.SetPolicyTypeIpSet {
				// Check Nested SetPolicy members
				members := strings.Split(setpol.Values, ",")
				for _, memberID := range members {
					_, ok := networkCache.Policies[memberID]
					if !ok {
						return newErrorFakeHNS(fmt.Sprintf("Member Policy %s not found", memberID))
					}
				}
			}
			networkCache.Policies[setpol.Id] = &setpol
		}
	case hcn.RequestTypeRefresh:
		return nil
	}

	return nil
}

func (f Hnsv2wrapperFake) AddNetworkPolicy(network *hcn.HostComputeNetwork, networkPolicy hcn.PolicyNetworkRequest) error {
	delayHnsCall(f.Delay)
	return nil
}

func (f Hnsv2wrapperFake) RemoveNetworkPolicy(network *hcn.HostComputeNetwork, networkPolicy hcn.PolicyNetworkRequest) error {
	delayHnsCall(f.Delay)
	return nil
}

func (f Hnsv2wrapperFake) GetNetworkByName(networkName string) (*hcn.HostComputeNetwork, error) {
	f.Lock()
	defer f.Unlock()
	delayHnsCall(f.Delay)
	if network, ok := f.Cache.networks[networkName]; ok {
		return network.GetHCNObj(), nil
	}
	return nil, hcn.NetworkNotFoundError{}
}

func (f Hnsv2wrapperFake) GetNetworkByID(networkID string) (*hcn.HostComputeNetwork, error) {
	f.Lock()
	defer f.Unlock()
	delayHnsCall(f.Delay)
	for _, network := range f.Cache.networks {
		if network.ID == networkID {
			return network.GetHCNObj(), nil
		}
	}
	return &hcn.HostComputeNetwork{}, nil
}

func (f Hnsv2wrapperFake) GetEndpointByID(endpointID string) (*hcn.HostComputeEndpoint, error) {
	f.Lock()
	defer f.Unlock()
	delayHnsCall(f.Delay)
	if ep, ok := f.Cache.endpoints[endpointID]; ok {
		return ep.GetHCNObj(), nil
	}
	return &hcn.HostComputeEndpoint{}, hcn.EndpointNotFoundError{EndpointID: endpointID}
}

func (f Hnsv2wrapperFake) CreateEndpoint(endpoint *hcn.HostComputeEndpoint) (*hcn.HostComputeEndpoint, error) {
	f.Lock()
	defer f.Unlock()
	delayHnsCall(f.Delay)
	f.Cache.endpoints[endpoint.Id] = NewFakeHostComputeEndpoint(endpoint)
	return endpoint, nil
}

func (f Hnsv2wrapperFake) DeleteEndpoint(endpoint *hcn.HostComputeEndpoint) error {
	f.Lock()
	defer f.Unlock()
	delayHnsCall(f.Delay)
	delete(f.Cache.endpoints, endpoint.Id)
	return nil
}

func (f Hnsv2wrapperFake) GetNamespaceByID(netNamespacePath string) (*hcn.HostComputeNamespace, error) {
	delayHnsCall(f.Delay)
	nameSpace := &hcn.HostComputeNamespace{Id: "ea37ac15-119e-477b-863b-cc23d6eeaa4d", NamespaceId: 1000}
	return nameSpace, nil
}

func (f Hnsv2wrapperFake) AddNamespaceEndpoint(namespaceId string, endpointId string) error {
	delayHnsCall(f.Delay)
	return nil
}

func (f Hnsv2wrapperFake) RemoveNamespaceEndpoint(namespaceId string, endpointId string) error {
	delayHnsCall(f.Delay)
	return nil
}

func (f Hnsv2wrapperFake) ListEndpointsOfNetwork(networkId string) ([]hcn.HostComputeEndpoint, error) {
	f.Lock()
	defer f.Unlock()
	delayHnsCall(f.Delay)
	endpoints := make([]hcn.HostComputeEndpoint, 0)
	for _, endpoint := range f.Cache.endpoints {
		if endpoint.HostComputeNetwork == networkId {
			endpoints = append(endpoints, *endpoint.GetHCNObj())
		}
	}
	return endpoints, nil
}

func (f Hnsv2wrapperFake) ApplyEndpointPolicy(endpoint *hcn.HostComputeEndpoint, requestType hcn.RequestType, endpointPolicy hcn.PolicyEndpointRequest) error {
	f.Lock()
	defer f.Unlock()
	delayHnsCall(f.Delay)
	epCache, ok := f.Cache.endpoints[endpoint.Id]
	if !ok {
		return newErrorFakeHNS(fmt.Sprintf("[FakeHNS] could not find endpoint %s", endpoint.Id))
	}
	switch requestType {
	case hcn.RequestTypeAdd:
		for _, newPolicy := range endpointPolicy.Policies {
			if newPolicy.Type != hcn.ACL {
				continue
			}
			var aclPol FakeEndpointPolicy
			err := json.Unmarshal(newPolicy.Settings, &aclPol)
			if err != nil {
				return newErrorFakeHNS(err.Error())
			}
			epCache.Policies = append(epCache.Policies, &aclPol)
		}
	case hcn.RequestTypeRemove:
		for _, newPolicy := range endpointPolicy.Policies {
			if newPolicy.Type != hcn.ACL {
				continue
			}
			var aclPol FakeEndpointPolicy
			err := json.Unmarshal(newPolicy.Settings, &aclPol)
			if err != nil {
				return newErrorFakeHNS(err.Error())
			}
			err = epCache.RemovePolicy(&aclPol)
			if err != nil {
				return err
			}
		}
	case hcn.RequestTypeUpdate:
		epCache.Policies = make([]*FakeEndpointPolicy, 0)
		for _, newPolicy := range endpointPolicy.Policies {
			if newPolicy.Type != hcn.ACL {
				continue
			}
			var aclPol FakeEndpointPolicy
			err := json.Unmarshal(newPolicy.Settings, &aclPol)
			if err != nil {
				return newErrorFakeHNS(err.Error())
			}
			epCache.Policies = append(epCache.Policies, &aclPol)
		}
	case hcn.RequestTypeRefresh:
		return nil
	}

	return nil
}

func (f Hnsv2wrapperFake) GetEndpointByName(endpointName string) (*hcn.HostComputeEndpoint, error) {
	delayHnsCall(f.Delay)
	return nil, hcn.EndpointNotFoundError{EndpointName: endpointName}
}

type FakeHNSCache struct {
	// networks maps network name to network object
	networks map[string]*FakeHostComputeNetwork
	// endpoints maps endpoint ID to endpoint object
	endpoints map[string]*FakeHostComputeEndpoint
}

// SetPolicy returns the first SetPolicy found with this ID in any network.
func (fCache FakeHNSCache) SetPolicy(setID string) *hcn.SetPolicySetting {
	for _, network := range fCache.networks {
		for _, policy := range network.Policies {
			if policy.Id == setID {
				return policy
			}
		}
	}
	return nil
}

func (fCache FakeHNSCache) PrettyString() string {
	networkStrings := make([]string, 0, len(fCache.networks))
	for _, network := range fCache.networks {
		networkStrings = append(networkStrings, fmt.Sprintf("[%+v]", network.PrettyString()))
	}

	endpointStrings := make([]string, 0, len(fCache.endpoints))
	for _, endpoint := range fCache.endpoints {
		endpointStrings = append(endpointStrings, fmt.Sprintf("[%+v]", endpoint.PrettyString()))
	}

	return fmt.Sprintf("networks: %s\nendpoints: %s", strings.Join(networkStrings, ","), strings.Join(endpointStrings, ","))
}

// AllSetPolicies returns all SetPolicies in a given network as a map of SetPolicy ID to SetPolicy object.
func (fCache FakeHNSCache) AllSetPolicies(networkID string) map[string]*hcn.SetPolicySetting {
	setPolicies := make(map[string]*hcn.SetPolicySetting)
	for _, network := range fCache.networks {
		if network.ID == networkID {
			for _, setPolicy := range network.Policies {
				setPolicies[setPolicy.Id] = setPolicy
			}
			break
		}
	}
	return setPolicies
}

// ACLPolicies returns a map of the inputed Endpoint IDs to Policies with the given policyID.
func (fCache FakeHNSCache) ACLPolicies(epList map[string]string, policyID string) (map[string][]*FakeEndpointPolicy, error) {
	aclPols := make(map[string][]*FakeEndpointPolicy)
	for ip, epID := range epList {
		epCache, ok := fCache.endpoints[epID]
		if !ok {
			return nil, newErrorFakeHNS(fmt.Sprintf("[FakeHNS] could not find endpoint %s", epID))
		}
		if epCache.IPConfiguration != ip {
			return nil, newErrorFakeHNS(fmt.Sprintf("[FakeHNS] Mismatch in IP addr of endpoint %s Got: %s, Expect %s",
				epID, epCache.IPConfiguration, ip))
		}
		aclPols[epID] = make([]*FakeEndpointPolicy, 0)
		for _, policy := range epCache.Policies {
			if policy.ID == policyID {
				aclPols[epID] = append(aclPols[epID], policy)
			}
		}

	}
	return aclPols, nil
}

// GetAllACLs maps all Endpoint IDs to ACLs
func (fCache FakeHNSCache) GetAllACLs() map[string][]*FakeEndpointPolicy {
	aclPols := make(map[string][]*FakeEndpointPolicy)
	for _, ep := range fCache.endpoints {
		aclPols[ep.ID] = ep.Policies
	}
	return aclPols
}

// EndpointIP returns the Endpoint's IP or an empty string if the Endpoint doesn't exist.
func (fCache FakeHNSCache) EndpointIP(id string) string {
	for _, ep := range fCache.endpoints {
		if ep.ID == id {
			return ep.IPConfiguration
		}
	}
	return ""
}

type FakeHostComputeNetwork struct {
	ID   string
	Name string
	// Policies maps SetPolicy ID to SetPolicy object
	Policies map[string]*hcn.SetPolicySetting
}

func NewFakeHostComputeNetwork(network *hcn.HostComputeNetwork) *FakeHostComputeNetwork {
	return &FakeHostComputeNetwork{
		ID:       network.Id,
		Name:     network.Name,
		Policies: make(map[string]*hcn.SetPolicySetting),
	}
}

func (fNetwork *FakeHostComputeNetwork) PrettyString() string {
	setPolicyStrings := make([]string, 0, len(fNetwork.Policies))
	for _, setPolicy := range fNetwork.Policies {
		setPolicyStrings = append(setPolicyStrings, fmt.Sprintf("[%+v]", setPolicy))
	}
	return fmt.Sprintf("ID: %s, Name: %s, SetPolicies: [%s]", fNetwork.ID, fNetwork.Name, strings.Join(setPolicyStrings, ","))
}

func (fNetwork *FakeHostComputeNetwork) GetHCNObj() *hcn.HostComputeNetwork {
	setPolicies := make([]hcn.NetworkPolicy, 0)
	for _, setPolicy := range fNetwork.Policies {
		rawSettings, err := json.Marshal(setPolicy)
		if err != nil {
			fmt.Printf("FakeHostComputeNetwork: error marshalling SetPolicy: %+v. err: %s\n", setPolicy, err.Error())
			continue
		}
		policy := hcn.NetworkPolicy{
			Type:     hcn.SetPolicy,
			Settings: rawSettings,
		}
		setPolicies = append(setPolicies, policy)
	}

	return &hcn.HostComputeNetwork{
		Id:       fNetwork.ID,
		Name:     fNetwork.Name,
		Policies: setPolicies,
	}
}

type FakeHostComputeEndpoint struct {
	ID                 string
	Name               string
	HostComputeNetwork string
	Policies           []*FakeEndpointPolicy
	IPConfiguration    string
}

func NewFakeHostComputeEndpoint(endpoint *hcn.HostComputeEndpoint) *FakeHostComputeEndpoint {
	ip := ""
	if endpoint.IpConfigurations != nil {
		ip = endpoint.IpConfigurations[0].IpAddress
	}
	return &FakeHostComputeEndpoint{
		ID:                 endpoint.Id,
		Name:               endpoint.Name,
		HostComputeNetwork: endpoint.HostComputeNetwork,
		IPConfiguration:    ip,
	}
}

func (fEndpoint *FakeHostComputeEndpoint) PrettyString() string {
	aclStrings := make([]string, 0, len(fEndpoint.Policies))
	for _, acl := range fEndpoint.Policies {
		aclStrings = append(aclStrings, fmt.Sprintf("[%+v]", acl))
	}
	return fmt.Sprintf("ID: %s, Name: %s, IP: %s, ACLs: [%s]",
		fEndpoint.ID, fEndpoint.Name, fEndpoint.IPConfiguration, strings.Join(aclStrings, ","))
}

func (fEndpoint *FakeHostComputeEndpoint) GetHCNObj() *hcn.HostComputeEndpoint {
	acls := make([]hcn.EndpointPolicy, 0)
	for _, acl := range fEndpoint.Policies {
		rawSettings, err := json.Marshal(acl)
		if err != nil {
			fmt.Printf("FakeHostComputeEndpoint: error marshalling ACL: %+v. err: %s\n", acl, err.Error())
			continue
		}
		policy := hcn.EndpointPolicy{
			Type:     hcn.ACL,
			Settings: rawSettings,
		}
		acls = append(acls, policy)
	}

	return &hcn.HostComputeEndpoint{
		Id:                 fEndpoint.ID,
		Name:               fEndpoint.Name,
		HostComputeNetwork: fEndpoint.HostComputeNetwork,
		IpConfigurations: []hcn.IpConfig{
			{
				IpAddress: fEndpoint.IPConfiguration,
			},
		},
		Policies: acls,
	}
}

func (fEndpoint *FakeHostComputeEndpoint) RemovePolicy(toRemovePol *FakeEndpointPolicy) error {
	for i, policy := range fEndpoint.Policies {
		if reflect.DeepEqual(policy, toRemovePol) {
			fEndpoint.Policies = append(fEndpoint.Policies[:i], fEndpoint.Policies[i+1:]...)
			return nil
		}
	}
	return newErrorFakeHNS(fmt.Sprintf("Could not find policy %+v", toRemovePol))
}

type FakeEndpointPolicy struct {
	ID              string            `json:",omitempty"`
	Protocols       string            `json:",omitempty"` // EX: 6 (TCP), 17 (UDP), 1 (ICMPv4), 58 (ICMPv6), 2 (IGMP)
	Action          hcn.ActionType    `json:","`
	Direction       hcn.DirectionType `json:","`
	LocalAddresses  string            `json:",omitempty"`
	RemoteAddresses string            `json:",omitempty"`
	LocalPorts      string            `json:",omitempty"`
	RemotePorts     string            `json:",omitempty"`
	Priority        int               `json:",omitempty"`
}
