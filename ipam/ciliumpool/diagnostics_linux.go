package ciliumpool

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/ipam"
	"github.com/Azure/azure-container-networking/store"
	"go.uber.org/zap"
)

// RunDiagnostics observes state independently of reconciliation failures.
func (m *Monitor) RunDiagnostics(ctx context.Context, cfg *common.CiliumPoolConfig, credentials func() (time.Time, error), logger *zap.Logger) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		start := time.Now()
		status, boot, err := readLocalStatus(cfg, logger)
		if err == nil {
			m.setLocal(status, boot)
		}
		m.Observe("storage", time.Since(start), err)
		if err != nil {
			logger.Warn("pool health observation failed", zap.String("stage", "storage"), zap.Error(err))
		}
		start = time.Now()
		// Without local ownership, route inspection cannot exclude pod-local conflicts.
		if err == nil {
			err = checkHostRoutes(cfg, status.Addresses)
		}
		m.Observe("routes", time.Since(start), err)
		if err != nil {
			logger.Warn("pool health observation failed", zap.String("stage", "routes"), zap.Error(err))
		}
		start = time.Now()
		expiry, err := credentials()
		m.mu.Lock()
		m.snapshot.CredentialExpiry = expiry
		m.mu.Unlock()
		m.Observe("credentials", time.Since(start), err)
		if err != nil {
			logger.Warn("pool health observation failed", zap.String("stage", "credentials"), zap.Error(err))
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func readLocalStatus(cfg *common.CiliumPoolConfig, logger *zap.Logger) (status ipam.CiliumPoolStatus, boot string, resultErr error) {
	kvs, err := store.NewReadOnlyBoltStore(cfg.StatePath("azure-vnet-ipam"), logger)
	if err != nil {
		return status, "", fmt.Errorf("opening pool health store: %w", err)
	}
	if err = kvs.Lock(100 * time.Millisecond); err != nil {
		return status, "", fmt.Errorf("reading pool health: %w", err)
	}
	defer func() { resultErr = errors.Join(resultErr, kvs.Unlock()) }()
	status, err = ipam.ReadCiliumPoolStatus(kvs)
	if err != nil {
		return status, "", fmt.Errorf("reading committed pool status: %w", err)
	}
	boot, err = common.CiliumPoolBootID()
	if err != nil {
		return status, "", fmt.Errorf("reading pool health boot ID: %w", err)
	}
	return status, boot, nil
}
