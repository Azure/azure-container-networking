package main

// Entry point of the telemetry service if started by CNI

import (
	"time"

	"github.com/Azure/azure-container-networking/telemetry"
)

const (
	reportToHostIntervalInSeconds = 60 * time.Second
	azurecnitelemetry             = "azure-vnet-telemetry"
)

func main() {
	var tb *telemetry.TelemetryBuffer
	var err error

	for {
		tb = telemetry.NewTelemetryBuffer("")
		err = tb.StartServer()
		if err == nil || tb.FdExists {
			break
		}

		tb.Cleanup(telemetry.FdName)
		time.Sleep(time.Millisecond * 200)
	}

	tb.BufferAndPushData(reportToHostIntervalInSeconds)
}
