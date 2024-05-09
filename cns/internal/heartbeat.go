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

func SendHeartBeat(ctx context.Context, heartbeatIntervalInMins int, homeAzMonitor *restserver.HomeAzMonitor) {
	ticker := time.NewTicker(time.Minute * time.Duration(heartbeatIntervalInMins))
	defer ticker.Stop()
	metric := aitelemetry.Metric{
		Name: logger.HeartBeatMetricStr,
		// This signifies 1 heartbeat is sent. Sum of this metric will give us number of heartbeats received
		Value:            1.0,
		CustomDimensions: make(map[string]string),
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			getHomeAzResp := homeAzMonitor.GetHomeAz(ctx)
			if getHomeAzResp.Response.ReturnCode == types.Success {
				metric.CustomDimensions[logger.IsAZRSupportedStr] = strconv.FormatBool(getHomeAzResp.HomeAzResponse.IsSupported)
				metric.CustomDimensions[logger.HomeAZStr] = strconv.FormatUint(uint64(getHomeAzResp.HomeAzResponse.HomeAz), 10)
			}

			logger.SendMetric(metric)
		}
	}
}
