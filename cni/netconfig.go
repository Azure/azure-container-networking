// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package cni

import (
	"encoding/json"
	"strings"

	"github.com/Azure/azure-container-networking/network/policy"

	cniTypes "github.com/containernetworking/cni/pkg/types"
)

const (
	PolicyStr string = "Policy"
)

// KVPair represents a K-V pair of a json object.
type KVPair struct {
	Name  string          `json:"name"`
	Value json.RawMessage `json:"value"`
}

type PortMapping struct {
	HostPort      int    `json:"hostPort"`
	ContainerPort int    `json:"containerPort"`
	Protocol      string `json:"protocol"`
	HostIp        string `json:"hostIP,omitempty"`
}
type RuntimeConfig struct {
	PortMappings []PortMapping    `json:"portMappings,omitempty"`
	DNS          RuntimeDNSConfig `json:"dns,omitempty"`
}

// https://github.com/kubernetes/kubernetes/blob/master/pkg/kubelet/dockershim/network/cni/cni.go#L104
type RuntimeDNSConfig struct {
	Servers  []string `json:"servers,omitempty"`
	Searches []string `json:"searches,omitempty"`
	Options  []string `json:"options,omitempty"`
}

// NetworkConfig represents Azure CNI plugin network configuration.
type NetworkConfig struct {
	CNIVersion                    string   `json:"cniVersion,omitempty"`
	Name                          string   `json:"name,omitempty"`
	Type                          string   `json:"type,omitempty"`
	Mode                          string   `json:"mode,omitempty"`
	Master                        string   `json:"master,omitempty"`
	AdapterName                   string   `json:"adapterName,omitempty"`
	Bridge                        string   `json:"bridge,omitempty"`
	LogLevel                      string   `json:"logLevel,omitempty"`
	LogTarget                     string   `json:"logTarget,omitempty"`
	InfraVnetAddressSpace         string   `json:"infraVnetAddressSpace,omitempty"`
	IPV6Mode                      string   `json:"ipv6Mode,omitempty"`
	ServiceCidrs                  string   `json:"serviceCidrs,omitempty"`
	VnetCidrs                     string   `json:"vnetCidrs,omitempty"`
	PodNamespaceForDualNetwork    []string `json:"podNamespaceForDualNetwork,omitempty"`
	IPsToRouteViaHost             []string `json:"ipsToRouteViaHost,omitempty"`
	MultiTenancy                  bool     `json:"multiTenancy,omitempty"`
	EnableSnatOnHost              bool     `json:"enableSnatOnHost,omitempty"`
	EnableExactMatchForPodName    bool     `json:"enableExactMatchForPodName,omitempty"`
	DisableHairpinOnHostInterface bool     `json:"disableHairpinOnHostInterface,omitempty"`
	DisableIPTableLock            bool     `json:"disableIPTableLock,omitempty"`
	CNSUrl                        string   `json:"cnsurl,omitempty"`
	ExecutionMode                 string   `json:"executionMode,omitempty"`
	Ipam                          struct {
		Type          string `json:"type"`
		Environment   string `json:"environment,omitempty"`
		AddrSpace     string `json:"addressSpace,omitempty"`
		Subnet        string `json:"subnet,omitempty"`
		Address       string `json:"ipAddress,omitempty"`
		QueryInterval string `json:"queryInterval,omitempty"`
	} `json:"ipam,omitempty"`
	DNS            cniTypes.DNS  `json:"dns,omitempty"`
	RuntimeConfig  RuntimeConfig `json:"runtimeConfig,omitempty"`
	AdditionalArgs []KVPair      `json:"AdditionalArgs,omitempty"`
}

type K8SPodEnvArgs struct {
	cniTypes.CommonArgs
	K8S_POD_NAMESPACE          cniTypes.UnmarshallableString `json:"K8S_POD_NAMESPACE,omitempty"`
	K8S_POD_NAME               cniTypes.UnmarshallableString `json:"K8S_POD_NAME,omitempty"`
	K8S_POD_INFRA_CONTAINER_ID cniTypes.UnmarshallableString `json:"K8S_POD_INFRA_CONTAINER_ID,omitempty"`
}

// ParseCniArgs unmarshals cni arguments.
func ParseCniArgs(args string) (*K8SPodEnvArgs, error) {
	podCfg := K8SPodEnvArgs{}
	err := cniTypes.LoadArgs(args, &podCfg)
	if err != nil {
		return nil, err
	}

	return &podCfg, nil
}

// ParseNetworkConfig unmarshals network configuration from bytes.
func ParseNetworkConfig(b []byte) (*NetworkConfig, error) {
	nwCfg := NetworkConfig{}

	err := json.Unmarshal(b, &nwCfg)
	if err != nil {
		return nil, err
	}

	if nwCfg.CNIVersion == "" {
		nwCfg.CNIVersion = defaultVersion
	}

	return &nwCfg, nil
}

// GetPoliciesFromNwCfg returns network policies from network config.
func GetPoliciesFromNwCfg(kvp []KVPair) []policy.Policy {
	var policies []policy.Policy
	for _, pair := range kvp {
		if strings.Contains(pair.Name, PolicyStr) {
			policy := policy.Policy{
				Type: policy.CNIPolicyType(pair.Name),
				Data: pair.Value,
			}
			policies = append(policies, policy)
		}
	}

	return policies
}

// Serialize marshals a network configuration to bytes.
func (nwcfg *NetworkConfig) Serialize() []byte {
	bytes, _ := json.Marshal(nwcfg)
	return bytes
}
