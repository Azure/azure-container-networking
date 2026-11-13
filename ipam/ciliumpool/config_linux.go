package ciliumpool

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"path/filepath"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cni/util"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
)

const PluginName = cni.AzureCiliumIPAM

var ErrConfig = errors.New("invalid Azure Cilium IPAM configuration")

type Config struct {
	NodeName          string `json:"nodeName"`
	Kubeconfig        string `json:"kubeconfig"`
	Interface         string `json:"interface"`
	NICID             string `json:"nicID"`
	Subnet            string `json:"subnet"`
	PreAllocate       int    `json:"preAllocate"`
	MinAllocate       int    `json:"minAllocate"`
	MaxAboveWatermark int    `json:"maxAboveWatermark"`
}

func ParseConfig(data []byte) (*cni.NetworkConfig, Config, error) {
	var list struct {
		Name    string            `json:"name"`
		Version string            `json:"cniVersion"`
		Plugins []json.RawMessage `json:"plugins"`
	}
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, Config{}, fmt.Errorf("decoding configuration: %w", err)
	}
	if len(list.Plugins) > 0 {
		data = nil
		for _, raw := range list.Plugins {
			var entry struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(raw, &entry); err != nil {
				return nil, Config{}, fmt.Errorf("decoding conflist entry: %w", err)
			}
			if entry.Type == "azure-vnet" {
				if data != nil {
					return nil, Config{}, fmt.Errorf("%w: multiple Azure CNI entries", ErrConfig)
				}
				data = raw
			}
		}
	}
	nw, err := cni.ParseNetworkConfig(data)
	if err != nil {
		return nil, Config{}, fmt.Errorf("decoding network settings: %w", err)
	}
	if len(list.Plugins) > 0 {
		nw.Name, nw.CNIVersion = list.Name, list.Version
	}
	if nw.Name == "" || nw.IPAM.Type != PluginName || nw.Mode != "transparent-tunnel" ||
		nw.IPV6Mode != "" || nw.MultiTenancy || nw.IPAM.Environment != "" || nw.IPAM.Mode != "" ||
		len(nw.PodNamespaceForDualNetwork) != 0 ||
		(nw.ExecutionMode != "" && nw.ExecutionMode != string(util.Default)) {
		return nil, Config{}, ErrConfig
	}
	var cfg Config
	d := json.NewDecoder(bytes.NewReader(nw.IPAM.Cilium))
	d.DisallowUnknownFields()
	if err := d.Decode(&cfg); err != nil {
		return nil, Config{}, fmt.Errorf("decoding IPAM settings: %w", err)
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return nil, Config{}, fmt.Errorf("%w: trailing settings", ErrConfig)
	}
	if err := cfg.validate(); err != nil {
		return nil, Config{}, err
	}
	return nw, cfg, nil
}

func (cfg Config) validate() error {
	prefix, prefixErr := netip.ParsePrefix(cfg.Subnet)
	nic, nicErr := arm.ParseResourceID(cfg.NICID)
	if cfg.NodeName == "" || cfg.Interface == "" || !filepath.IsAbs(cfg.Kubeconfig) ||
		filepath.Clean(cfg.Kubeconfig) != cfg.Kubeconfig ||
		prefixErr != nil || !prefix.Addr().Is4() || prefix != prefix.Masked() ||
		nicErr != nil || nic.ResourceType.String() != "Microsoft.Network/networkInterfaces" ||
		cfg.PreAllocate < 1 || cfg.PreAllocate > 255 || cfg.MinAllocate < 0 || cfg.MinAllocate > 255 ||
		cfg.MaxAboveWatermark < 0 || cfg.MaxAboveWatermark > 255 || cfg.MinAllocate+cfg.MaxAboveWatermark > 255 {
		return ErrConfig
	}
	return nil
}
