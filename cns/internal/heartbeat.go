// Copyright 2018 Microsoft. All rights reserved.
// MIT License

package internal

import (
	"context"
	"strconv"
	"time"

	"github.com/Azure/azure-container-networking/aitelemetry"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/restserver"
	"github.com/Azure/azure-container-networking/cns/types"
)

// SendHeartBeat emits node metrics periodically
func SendHeartBeat(ctx context.Context, heartbeatInterval time.Duration, homeAzMonitor *restserver.HomeAzMonitor) {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			metric := aitelemetry.Metric{
				Name: logger.HeartBeatMetricStr,
				// This signifies 1 heartbeat is sent. Sum of this metric will give us number of heartbeats received
				Value:            1.0,
				CustomDimensions: make(map[string]string),
			}
			getHomeAzResp := homeAzMonitor.GetHomeAz(ctx)
			if getHomeAzResp.Response.ReturnCode == types.Success {
				metric.CustomDimensions[logger.IsAZRSupportedStr] = strconv.FormatBool(getHomeAzResp.HomeAzResponse.IsSupported)
				metric.CustomDimensions[logger.HomeAZStr] = strconv.FormatUint(uint64(getHomeAzResp.HomeAzResponse.HomeAz), 10)
			}

			logger.SendMetric(metric)
		}
	}
}
