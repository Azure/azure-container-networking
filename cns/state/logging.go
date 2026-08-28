// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package state

import (
	"context"
	"time"

	"go.uber.org/zap"
)

func (s *DB) observeLifecycle(
	ctx context.Context,
	operation LifecycleOperation,
	result OperationResult,
	duration time.Duration,
) {
	if s.metrics == nil && s.logger == nil {
		return
	}
	status, err := s.summary(ctx)
	if err != nil {
		return
	}
	_ = s.metrics.Refresh(status)
	if s.logger == nil {
		return
	}
	s.logger.Info(
		lifecycleMessage(operation, result),
		zap.String("backend", status.Backend),
		zap.String("authority", string(status.Authority)),
		zap.Uint32("schema", status.SchemaVersion),
		zap.Uint64("generation", status.Generation),
		zap.String("operation", string(operation)),
		zap.String("result", string(result)),
		zap.Int("networkContainerCount", status.Records.NetworkContainers),
		zap.Int("ipCount", status.Records.IPs),
		zap.Int("networkCount", status.Records.Networks),
		zap.Int("endpointCount", status.Records.Endpoints),
		zap.Int("assignmentCount", status.Records.Assignments),
		zap.Int("ownerCount", status.Records.Owners),
		zap.Int("deleteIntentCount", status.Records.DeleteIntents),
		zap.Duration("duration", duration),
	)
}

func lifecycleMessage(operation LifecycleOperation, result OperationResult) string {
	switch {
	case operation == LifecycleStartup:
		return "persistent state opened"
	case operation == LifecycleImport && result == ResultNoop:
		return "persistent state import skipped"
	case operation == LifecycleImport:
		return "persistent state import completed"
	case operation == LifecycleRollback && result == ResultNoop:
		return "persistent state rollback skipped"
	case operation == LifecycleRollback:
		return "persistent state rollback completed"
	case operation == LifecycleBoot && result == ResultNoop:
		return "persistent state boot unchanged"
	case operation == LifecycleBoot:
		return "persistent state boot applied"
	default:
		return "persistent state lifecycle completed"
	}
}
