package network

import (
	"errors"
	"runtime"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cni/util"
)

var errCiliumIPAMConfig = errors.New("azure-cilium-ipam requires stateful Linux IPv4 transparent-tunnel with an explicit master and default execution")

// Cilium IPAM supports stateful Linux IPv4 transparent-tunnel with default
// execution. Alternate IPAM modes, multitenancy and dual-network setups
// are not supported.
func validateCiliumIPAMConfig(cfg *cni.NetworkConfig, stateless bool) error {
	if cfg.IPAM.Type != cni.AzureCiliumIPAM {
		return nil
	}
	if stateless || runtime.GOOS != "linux" || cfg.Master == "" || cfg.Mode != "transparent-tunnel" || cfg.IPV6Mode != "" ||
		cfg.MultiTenancy || len(cfg.PodNamespaceForDualNetwork) != 0 ||
		cfg.IPAM.Environment != "" || cfg.IPAM.Mode != "" ||
		(cfg.ExecutionMode != "" && cfg.ExecutionMode != string(util.Default)) {
		return errCiliumIPAMConfig
	}
	return nil
}
