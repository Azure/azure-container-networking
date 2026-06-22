package cniconflist

import (
	"io"

	"github.com/pkg/errors"
)

const (
	ciliumcniVersion  = "0.3.1"                   //nolint:unused,deadcode,varcheck // used in linux
	ciliumcniName     = "cilium"                  //nolint:unused,deadcode,varcheck // used in linux
	ciliumcniType     = "cilium-cni"              //nolint:unused,deadcode,varcheck // used in linux
	ciliumLogFile     = "/var/log/cilium-cni.log" //nolint:unused,deadcode,varcheck // used in linux
	ciliumIPAM        = "azure-ipam"              //nolint:unused,deadcode,varcheck // used in linux
	overlaycniVersion = "0.3.0"                   //nolint:unused,deadcode,varcheck // used in linux
	overlaycniName    = "azure"                   //nolint:unused,deadcode,varcheck // used in linux
	overlaycniType    = "azure-vnet"              //nolint:unused,deadcode,varcheck // used in linux
	nodeLocalDNSIP    = "169.254.20.10"           //nolint:unused,deadcode,varcheck // used in linux
	azurecniVersion   = "0.3.0"                   //nolint:unused,deadcode,varcheck // used in linux
	azureName         = "azure"                   //nolint:unused,deadcode,varcheck // used in linux
	azureType         = "azure-vnet"              //nolint:unused,deadcode,varcheck // used in linux
)

// cniConflist represents the containernetworking/cni/pkg/types.NetConfList
type cniConflist struct { //nolint:unused,deadcode // used in linux
	CNIVersion   string `json:"cniVersion,omitempty"`
	Name         string `json:"name,omitempty"`
	AdapterName  string `json:"adapterName,omitempty"`
	DisableCheck bool   `json:"disableCheck,omitempty"`
	Plugins      []any  `json:"plugins,omitempty"`
}

// NetConf describes a network. It represents the Cilium specific containernetworking/cni/pkg/types.NetConf
type NetConf struct {
	CNIVersion   string          `json:"cniVersion,omitempty"`
	Name         string          `json:"name,omitempty"`
	Type         string          `json:"type,omitempty"`
	Capabilities map[string]bool `json:"capabilities,omitempty"`
	IPAM         IPAM            `json:"ipam,omitempty"`
	EnableDebug  bool            `json:"enable-debug"`
	LogFile      string          `json:"log-file"`

	RawPrevResult map[string]interface{} `json:"prevResult,omitempty"`
}

type IPAM struct {
	Type string `json:"type,omitempty"`
}

type WindowsSettings struct {
	DNSServiceIP                string
	ClusterCIDRs                []string
	ServiceCIDRs                []string
	VNetCIDRs                   []string
	HNSTimeoutDurationInSeconds int
	DisableOutboundNAT          bool
	EnableLoopbackDSR           bool
}

// V4OverlayGenerator generates the Azure CNI conflist for the ipv4 Overlay scenario
type V4OverlayGenerator struct {
	Writer          io.WriteCloser
	WindowsSettings WindowsSettings
}

// DualStackOverlayGenerator generates the Azure CNI conflist for the dualstack Overlay scenario
type DualStackOverlayGenerator struct {
	Writer          io.WriteCloser
	WindowsSettings WindowsSettings
}

// OverlayGenerator generates the Azure CNI conflist for all Overlay scenarios
type OverlayGenerator struct {
	Writer          io.WriteCloser
	WindowsSettings WindowsSettings
}

// CiliumGenerator generates the Azure CNI conflist for the Cilium scenario
type CiliumGenerator struct {
	Writer          io.WriteCloser
	WindowsSettings WindowsSettings
}

// SWIFTGenerator generates the Azure CNI conflist for the SWIFT scenario
type SWIFTGenerator struct {
	Writer          io.WriteCloser
	WindowsSettings WindowsSettings
}

type AzureCNIChainedCiliumGenerator struct {
	Writer          io.WriteCloser
	WindowsSettings WindowsSettings
}

func (v *V4OverlayGenerator) Close() error {
	if err := v.Writer.Close(); err != nil {
		return errors.Wrap(err, "error closing generator")
	}

	return nil
}

func (v *DualStackOverlayGenerator) Close() error {
	if err := v.Writer.Close(); err != nil {
		return errors.Wrap(err, "error closing generator")
	}

	return nil
}

func (v *OverlayGenerator) Close() error {
	if err := v.Writer.Close(); err != nil {
		return errors.Wrap(err, "error closing generator")
	}

	return nil
}

func (v *CiliumGenerator) Close() error {
	if err := v.Writer.Close(); err != nil {
		return errors.Wrap(err, "error closing generator")
	}

	return nil
}

func (v *SWIFTGenerator) Close() error {
	if err := v.Writer.Close(); err != nil {
		return errors.Wrap(err, "error closing generator")
	}

	return nil
}

func (v *AzureCNIChainedCiliumGenerator) Close() error {
	if err := v.Writer.Close(); err != nil {
		return errors.Wrap(err, "error closing generator")
	}

	return nil
}
