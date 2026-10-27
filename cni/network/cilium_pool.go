package network

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/ipam"
	"github.com/Azure/azure-container-networking/store"
)

var errCiliumPoolNetworkState = errors.New("network state has no interfaces with active cilium pool reservations")

// The caller holds the network store lock. Never bootstrap replacement network
// state while durable IPAM ownership still refers to existing endpoints.
func validateCiliumPoolNetworkState(config *common.PluginConfig) (resultErr error) {
	if config.CiliumPool == nil {
		return nil
	}
	kvs, err := store.NewBoltStore(config.CiliumPool.StatePath("azure-vnet-ipam"), logger)
	if err != nil {
		return fmt.Errorf("opening cilium pool IPAM state: %w", err)
	}
	if err = kvs.Lock(store.DefaultLockTimeoutLinux); err != nil {
		return fmt.Errorf("locking cilium pool IPAM state: %w", err)
	}
	defer func() {
		if unlockErr := kvs.Unlock(); unlockErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("unlocking cilium pool IPAM state: %w", unlockErr))
		}
	}()
	reserved, err := ipam.CiliumPoolHasReservations(kvs)
	if err != nil {
		return fmt.Errorf("reading cilium pool reservations: %w", err)
	}
	if !reserved {
		return nil
	}
	var raw json.RawMessage
	if err := config.Store.Read("Network", &raw); err != nil {
		return fmt.Errorf("network state unavailable with active cilium pool reservations: %w", err)
	}
	var state struct {
		ExternalInterfaces map[string]json.RawMessage `json:"ExternalInterfaces"`
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return fmt.Errorf("decoding cilium pool network state: %w", err)
	}
	if len(state.ExternalInterfaces) == 0 {
		return errCiliumPoolNetworkState
	}
	return nil
}
