package cniconflist

import (
	"encoding/json"

	"github.com/Azure/azure-container-networking/cni"
	cninet "github.com/Azure/azure-container-networking/cni/network"
	"github.com/Azure/azure-container-networking/cni/util"
	"github.com/Azure/azure-container-networking/network"
	"github.com/pkg/errors"
)

// portmapConfig is the config for the upstream portmap plugin
var portmapConfig any = struct {
	Type         string          `json:"type"`
	Capabilities map[string]bool `json:"capabilities"`
	SNAT         bool            `json:"snat"`
}{
	Type: "portmap",
	Capabilities: map[string]bool{
		"portMappings": true,
	},
	SNAT: true,
}

// Generate writes the CNI conflist to the Generator's output stream
func (v *V4OverlayGenerator) Generate() error {
	conflist := cniConflist{
		CNIVersion: cniVersion,
		Name:       cniName,
		Plugins: []any{
			cni.NetworkConfig{
				Type:              cniType,
				Mode:              cninet.OpModeTransparent,
				ExecutionMode:     string(util.V4Swift),
				IPsToRouteViaHost: []string{nodeLocalDNSIP},
				IPAM: cni.IPAM{
					Type: network.AzureCNS,
					Mode: string(util.V4Overlay),
				},
			},
			portmapConfig,
		},
	}

	enc := json.NewEncoder(v.Writer)
	enc.SetIndent("", "\t")
	if err := enc.Encode(conflist); err != nil {
		return errors.Wrap(err, "error encoding conflist to json")
	}

	return nil
}
