package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/ipam"
	"github.com/Azure/azure-container-networking/store"
	"go.uber.org/zap"
)

const bridgeStateName = "azure-vnet-ipam-bridge"

var (
	errLocalState   = errors.New("ciliumpool: invalid local state")
	errStateConfig  = errors.New("ciliumpool: state configuration and logger are required")
	errNetworkState = errors.New("network state has no interfaces with active cilium pool reservations")
)

// LocalState owns persistence and coordinated access for the pool consumer.
type LocalState struct {
	networkPath  string
	ipamPath     string
	lifetimePath string
	logger       *zap.Logger
}

func NewLocalState(config *common.CiliumPoolConfig, logger *zap.Logger) (*LocalState, error) {
	if config == nil || logger == nil {
		return nil, errStateConfig
	}
	if err := os.MkdirAll(config.StateDir, 0o700); err != nil {
		return nil, fmt.Errorf("creating pool state directory: %w", err)
	}
	return &LocalState{
		networkPath:  config.StatePath("azure-vnet"),
		ipamPath:     config.StatePath("azure-vnet-ipam"),
		lifetimePath: config.StatePath(bridgeStateName),
		logger:       logger,
	}, nil
}

// Acquire excludes another publisher until the returned release is called.
func (s *LocalState) Acquire(ctx context.Context) (func(), error) {
	kvs, err := lockStore(ctx, s.lifetimePath, time.Millisecond, s.logger)
	if err != nil {
		return nil, err
	}
	return func() { unlockStore(kvs, s.logger) }, nil
}

// CanInitialize requires no prior logical network or IPAM initialization.
func (s *LocalState) CanInitialize(ctx context.Context) error {
	network, err := lockStore(ctx, s.networkPath, store.DefaultLockTimeoutLinux, s.logger)
	if err != nil {
		return err
	}
	defer unlockStore(network, s.logger)
	kvs, err := lockStore(ctx, s.ipamPath, store.DefaultLockTimeoutLinux, s.logger)
	if err != nil {
		return err
	}
	defer unlockStore(kvs, s.logger)
	for _, entry := range []struct {
		store store.KeyValueStore
		key   string
	}{{network, "Network"}, {kvs, "IPAM"}} {
		var raw json.RawMessage
		readErr := entry.store.Read(entry.key, &raw)
		if errors.Is(readErr, store.ErrKeyNotFound) || errors.Is(readErr, store.ErrStoreEmpty) {
			continue
		}
		if readErr != nil {
			return fmt.Errorf("reading local %s state: %w", entry.key, readErr)
		}
		return fmt.Errorf("%w: missing cilium node with initialized local %s state", errLocalState, entry.key)
	}
	return nil
}

func (s *LocalState) Synchronize(ctx context.Context, id ipam.CiliumPoolIdentity,
	supply []ipam.CiliumPoolAddress, initialize, admit bool,
) (used map[string]ipam.CiliumPoolUsage, quarantined int, err error) {
	// Coordinate recovery using CNI's network-before-IPAM lock order.
	network, err := lockStore(ctx, s.networkPath, store.DefaultLockTimeoutLinux, s.logger)
	if err != nil {
		return nil, 0, err
	}
	defer unlockStore(network, s.logger)
	kvs, err := lockStore(ctx, s.ipamPath, store.DefaultLockTimeoutLinux, s.logger)
	if err != nil {
		return nil, 0, fmt.Errorf("opening local IPAM state: %w", err)
	}
	defer unlockStore(kvs, s.logger)
	used, quarantined, err = ipam.CiliumPoolTransaction(kvs, id, supply, initialize, admit)
	if err != nil {
		return nil, 0, fmt.Errorf("synchronizing local pool: %w", err)
	}
	if len(used) > 0 {
		var networkState json.RawMessage
		if err := network.Read("Network", &networkState); err != nil {
			return nil, 0, errors.Join(fmt.Errorf("ciliumpool: network state missing with active reservations: %w", err),
				ipam.FenceCiliumPool(kvs, "network state missing"))
		}
	}
	return used, quarantined, nil
}

func (s *LocalState) Fence(ctx context.Context, reason string) error {
	kvs, err := lockStore(ctx, s.ipamPath, store.DefaultLockTimeoutLinux, s.logger)
	if err != nil {
		return fmt.Errorf("opening local IPAM state for fencing: %w", err)
	}
	defer unlockStore(kvs, s.logger)
	if err := ipam.FenceCiliumPool(kvs, reason); err != nil {
		return fmt.Errorf("fencing local pool: %w", err)
	}
	return nil
}

// ValidateNetworkState is called while the CNI caller holds network ownership.
func (s *LocalState) ValidateNetworkState(ctx context.Context, network store.KeyValueStore) (resultErr error) {
	kvs, err := lockStore(ctx, s.ipamPath, store.DefaultLockTimeoutLinux, s.logger)
	if err != nil {
		return fmt.Errorf("opening cilium pool IPAM state: %w", err)
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
	var state struct {
		ExternalInterfaces map[string]json.RawMessage `json:"ExternalInterfaces"`
	}
	if err := network.Read("Network", &state); err != nil {
		return fmt.Errorf("network state unavailable with active cilium pool reservations: %w", err)
	}
	if len(state.ExternalInterfaces) == 0 {
		return errNetworkState
	}
	return nil
}

func lockStore(ctx context.Context, path string, timeout time.Duration, logger *zap.Logger) (*store.BoltStore, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("waiting for state database: %w", err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		timeout = min(timeout, time.Until(deadline))
	}
	kvs, err := store.NewBoltStore(path, logger)
	if err != nil {
		return nil, fmt.Errorf("creating state store: %w", err)
	}
	if err := kvs.Lock(timeout); err != nil {
		return nil, fmt.Errorf("locking state database: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.Join(err, kvs.Unlock())
	}
	return kvs, nil
}

func unlockStore(kvs *store.BoltStore, logger *zap.Logger) {
	if err := kvs.Unlock(); err != nil {
		logger.Warn("closing state store failed", zap.Error(err))
	}
}
