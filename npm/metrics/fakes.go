package metrics

import "github.com/Azure/azure-container-networking/aitelemetry"

func CreateFakeTelemetryHandle() {
	th = aitelemetry.NewFakeTelemtry()
}
