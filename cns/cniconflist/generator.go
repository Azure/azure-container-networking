package cniconflist

import (
	"io"

	"github.com/pkg/errors"
)

const (
	cniVersion     = "0.3.0"         //nolint:unused,deadcode,varcheck // used in linux
	cniName        = "azure"         //nolint:unused,deadcode,varcheck // used in linux
	cniType        = "azure-vnet"    //nolint:unused,deadcode,varcheck // used in linux
	nodeLocalDNSIP = "169.254.20.10" //nolint:unused,deadcode,varcheck // used in linux
)

// cniConflist represents the containernetworking/cni/pkg/types.NetConfList
type cniConflist struct { //nolint:unused,deadcode // used in linux
	CNIVersion   string `json:"cniVersion,omitempty"`
	Name         string `json:"name,omitempty"`
	DisableCheck bool   `json:"disableCheck,omitempty"`
	Plugins      []any  `json:"plugins,omitempty"`
}

// V4OverlayGenerator generates the Azure CNI conflist for the ipv4 Overlay scenario
type V4OverlayGenerator struct {
	Writer io.WriteCloser
}

func (v *V4OverlayGenerator) Close() error {
	if err := v.Writer.Close(); err != nil {
		return errors.Wrap(err, "error closing generator")
	}

	return nil
}
