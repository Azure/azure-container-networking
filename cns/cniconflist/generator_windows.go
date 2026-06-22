package cniconflist

import (
	"encoding/json"
	"errors"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cni/util"
	"github.com/Azure/azure-container-networking/network"
	"github.com/Azure/azure-container-networking/network/policy"
	cniTypes "github.com/containernetworking/cni/pkg/types"
	pkgerrors "github.com/pkg/errors"
)

var errNotImplemented = errors.New("cni conflist generator not implemented on windows")

const (
	defaultWindowsHNSTimeoutDurationInSeconds = 120
	azureDNSServer                            = "168.63.129.16"
)

func (v *V4OverlayGenerator) Generate() error {
	return generateWindowsAzureCNSConflist(
		v.Writer,
		overlaycniVersion,
		string(util.Overlay),
		"",
		v.WindowsSettings,
	)
}

func (v *DualStackOverlayGenerator) Generate() error {
	return generateWindowsAzureCNSConflist(
		v.Writer,
		overlaycniVersion,
		string(util.DualStackOverlay),
		"",
		v.WindowsSettings,
	)
}

func (v *OverlayGenerator) Generate() error {
	return generateWindowsAzureCNSConflist(
		v.Writer,
		overlaycniVersion,
		string(util.Overlay),
		"",
		v.WindowsSettings,
	)
}

func (v *CiliumGenerator) Generate() error {
	return errNotImplemented
}

func (v *SWIFTGenerator) Generate() error {
	return generateWindowsAzureCNSConflist(
		v.Writer,
		"1.0.0",
		"",
		string(util.V4Swift),
		v.WindowsSettings,
	)
}

func (v *AzureCNIChainedCiliumGenerator) Generate() error {
	return errNotImplemented
}

func generateWindowsAzureCNSConflist(
	writer interface{ Write([]byte) (int, error) },
	version string,
	ipamMode string,
	executionMode string,
	settings WindowsSettings,
) error {
	endpointPolicies, err := windowsEndpointPolicies(settings)
	if err != nil {
		return err
	}
	nwCfg := windowsNetworkConfig{
		Type:          azureType,
		ExecutionMode: executionMode,
		Capabilities: map[string]bool{
			"portMappings": true,
			"dns":          true,
		},
		IPAM: cni.IPAM{
			Type: network.AzureCNS,
			Mode: ipamMode,
		},
		DNS:            cniTypesDNS(settings.DNSServiceIP),
		AdditionalArgs: endpointPolicies,
		WindowsSettings: cni.WindowsSettings{
			EnableLoopbackDSR:           settings.EnableLoopbackDSR,
			HnsTimeoutDurationInSeconds: hnsTimeoutDuration(settings.HNSTimeoutDurationInSeconds),
		},
	}

	conflist := cniConflist{
		CNIVersion:  version,
		Name:        azureName,
		AdapterName: "",
		Plugins:     []any{nwCfg},
	}

	enc := json.NewEncoder(writer)
	enc.SetIndent("", "\t")
	if err := enc.Encode(conflist); err != nil {
		return pkgerrors.Wrap(err, "error encoding conflist to json")
	}

	return nil
}

type windowsNetworkConfig struct {
	Type            string              `json:"type,omitempty"`
	ExecutionMode   string              `json:"executionMode,omitempty"`
	Capabilities    map[string]bool     `json:"capabilities,omitempty"`
	IPAM            cni.IPAM            `json:"ipam,omitempty"`
	DNS             cniTypes.DNS        `json:"dns,omitempty"`
	AdditionalArgs  []cni.KVPair        `json:"AdditionalArgs,omitempty"`
	WindowsSettings cni.WindowsSettings `json:"windowsSettings,omitempty"`
}

func cniTypesDNS(dnsServiceIP string) cniTypes.DNS {
	nameservers := []string{azureDNSServer}
	if dnsServiceIP != "" {
		nameservers = append([]string{dnsServiceIP}, nameservers...)
	}

	return cniTypes.DNS{
		Nameservers: nameservers,
		Search:      []string{"svc.cluster.local"},
	}
}

func windowsEndpointPolicies(settings WindowsSettings) ([]cni.KVPair, error) {
	policies := make([]cni.KVPair, 0, 2)
	if exceptions := outboundNATExceptions(settings); len(exceptions) > 0 && !settings.DisableOutboundNAT {
		kv, err := endpointPolicyKV(outboundNATPolicy{
			Type:          string(policy.OutBoundNatPolicy),
			ExceptionList: exceptions,
		})
		if err != nil {
			return nil, err
		}
		policies = append(policies, kv)
	}
	for _, serviceCIDR := range settings.ServiceCIDRs {
		if serviceCIDR == "" {
			continue
		}
		kv, err := endpointPolicyKV(routePolicy{
			Type:              string(policy.RoutePolicy),
			DestinationPrefix: serviceCIDR,
			NeedEncap:         true,
		})
		if err != nil {
			return nil, err
		}
		policies = append(policies, kv)
	}

	return policies, nil
}

func outboundNATExceptions(settings WindowsSettings) []string {
	exceptions := make([]string, 0, len(settings.ClusterCIDRs)+len(settings.ServiceCIDRs)+len(settings.VNetCIDRs))
	exceptions = append(exceptions, settings.ClusterCIDRs...)
	exceptions = append(exceptions, settings.ServiceCIDRs...)
	exceptions = append(exceptions, settings.VNetCIDRs...)
	return exceptions
}

func endpointPolicyKV(value any) (cni.KVPair, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return cni.KVPair{}, pkgerrors.Wrap(err, "marshal endpoint policy")
	}
	return cni.KVPair{
		Name:  string(policy.EndpointPolicy),
		Value: raw,
	}, nil
}

func hnsTimeoutDuration(configured int) int {
	if configured > 0 {
		return configured
	}

	return defaultWindowsHNSTimeoutDurationInSeconds
}

type outboundNATPolicy struct {
	Type          string   `json:"Type"`
	ExceptionList []string `json:"ExceptionList,omitempty"`
}

type routePolicy struct {
	Type              string `json:"Type"`
	DestinationPrefix string `json:"DestinationPrefix"`
	NeedEncap         bool   `json:"NeedEncap"`
}
