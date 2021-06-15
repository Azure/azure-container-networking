// Copyright 2018 Microsoft. All rights reserved.
// MIT License

package logger

import (
	"time"

	"github.com/Azure/azure-container-networking/aitelemetry"
)

func SendHeartBeat(heartbeatIntervalInMins int, stopheartbeat chan bool) {
	heartbeat := time.NewTicker(time.Minute * time.Duration(heartbeatIntervalInMins)).C
	metric := aitelemetry.Metric{
		Name: HeartBeatMetricStr,
		// This signifies 1 heartbeat is sent. Sum of this metric will give us number of heartbeats received
		Value:            1.0,
		CustomDimensions: make(map[string]string),
	}
	for {
		select {
		case <-heartbeat:
			SendMetric(metric)
		case <-stopheartbeat:
			return
		}
	}
}
