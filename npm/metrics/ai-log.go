package metrics

import (
	"fmt"

	"github.com/Azure/azure-container-networking/aitelemetry"
)

// Printf logs in the AI telemetry
func Printf(format string, args ...interface{}) {
	if th == nil {
		return
	}

	msg := fmt.Sprintf(format, args...)
	sendTraceInternal(msg)
}

// Send AI telemetry trace
func sendTraceInternal(msg string) {
	report := aitelemetry.Report{CustomDimensions: make(map[string]string)}
	report.Message = msg
    th.TrackLog(report)
}