package network

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cni/util"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/ipam/ciliumpool/persistence"
)

var errCiliumPoolNetworkConfig = errors.New("cilium pool requires IPv4 transparent-tunnel with azure-vnet-ipam and default execution")

const (
	transparentTunnelMode = "transparent-tunnel"
	azureIPAMPlugin       = "azure-vnet-ipam"
)

func validateCiliumPoolNetworkConfig(poolEnabled bool, cfg *cni.NetworkConfig) error {
	if !poolEnabled {
		if cfg.IPAM.Environment == common.OptEnvironmentCiliumPool {
			return fmt.Errorf("%w: missing host configuration", errCiliumPoolNetworkConfig)
		}
		return nil
	}
	if cfg.Mode != transparentTunnelMode || cfg.IPV6Mode != "" || cfg.IPAM.Type != azureIPAMPlugin ||
		cfg.MultiTenancy || len(cfg.PodNamespaceForDualNetwork) != 0 || cfg.IPAM.Mode != "" ||
		(cfg.ExecutionMode != "" && cfg.ExecutionMode != string(util.Default)) ||
		(cfg.IPAM.Environment != "" && cfg.IPAM.Environment != common.OptEnvironmentCiliumPool) {
		return errCiliumPoolNetworkConfig
	}
	return nil
}

func validateCiliumPoolNetworkState(config *common.PluginConfig) error {
	if config.CiliumPool == nil {
		return nil
	}
	state, err := persistence.NewLocalState(config.CiliumPool, logger)
	if err != nil {
		return fmt.Errorf("initializing cilium pool state: %w", err)
	}
	if err = state.ValidateNetworkState(context.Background(), config.Store); err != nil {
		return fmt.Errorf("validating cilium pool network state: %w", err)
	}
	return nil
}
