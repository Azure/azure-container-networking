// Copyright 2018 Microsoft. All rights reserved.
// MIT License

package metric

import (
	"context"
	"crypto/md5" //nolint:gosec // used for checksum
	"encoding/json"
	"time"

	"github.com/Azure/azure-container-networking/aitelemetry"
	"github.com/Azure/azure-container-networking/cns/configuration"
	"github.com/Azure/azure-container-networking/cns/logger"
)

// SendCNSConfigSnapshot emits CNS config periodically
func SendCNSConfigSnapshot(ctx context.Context, config *configuration.CNSConfig) {
	ticker := time.NewTicker(time.Minute * time.Duration(config.TelemetrySettings.ConfigSnapshotIntervalInMins))
	defer ticker.Stop()

	event, err := createCNSConfigSnapshotEvent(config)
	if err != nil {
		logger.Errorf("[Azure CNS] SendCNSConfigSnapshot failed to create event at an interval: %v", err)
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			logger.LogEvent(event)
		}
	}
}

func createCNSConfigSnapshotEvent(config *configuration.CNSConfig) (aitelemetry.Event, error) {
	bb, err := json.Marshal(config) //nolint:musttag // no tag needed for config
	if err != nil {
		return aitelemetry.Event{}, err
	}

	cs := md5.Sum(bb) //nolint:gosec // used for checksum
	csStr := string(cs[:])

	event := aitelemetry.Event{
		EventName:  logger.ConfigSnapshotMetricsStr,
		ResourceID: csStr, // not guaranteed unique, instead use VM ID and Subscription to correlate
		Properties: map[string]string{
			logger.CNSConfigPropertyStr:            string(bb),
			logger.CNSConfigMD5CheckSumPropertyStr: csStr,
		},
	}

	return event, nil
}
