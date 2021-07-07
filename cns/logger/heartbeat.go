// Copyright 2018 Microsoft. All rights reserved.
// MIT License

package logger

import (
	"context"
	"time"

	"github.com/Azure/azure-container-networking/aitelemetry"
)

func SendHeartBeat(ctx context.Context, heartbeatIntervalInMins int) {
	ticker := time.NewTicker(time.Minute * time.Duration(heartbeatIntervalInMins))
	defer ticker.Stop()
	metric := aitelemetry.Metric{
		Name: HeartBeatMetricStr,
		// This signifies 1 heartbeat is sent. Sum of this metric will give us number of heartbeats received
		Value:            1.0,
		CustomDimensions: make(map[string]string),
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			SendMetric(metric)
		}
	}
}
