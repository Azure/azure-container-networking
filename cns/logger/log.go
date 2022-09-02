// Copyright Microsoft. All rights reserved.
package logger

import (
	"github.com/Azure/azure-container-networking/aitelemetry"
	"github.com/Azure/azure-container-networking/cns/types"
)

var (
	Log        *CNSLogger
	aiMetadata string // this var is set at build time.
)

// todo: the functions below should be removed. CNSLogger should be injected where needed and not used from package level scope.

func Close() {
	Log.Close()
}

func InitLogger(fileName string, logLevel, logTarget int, logDir string) {
	Log, _ = NewCNSLogger(fileName, logLevel, logTarget, logDir)
}

func InitAI(aiConfig aitelemetry.AIConfig, disableTraceLogging, disableMetricLogging, disableEventLogging bool) {
	Log.InitAI(aiConfig, disableTraceLogging, disableMetricLogging, disableEventLogging)
}

func SetContextDetails(orchestrator, nodeID string) {
	Log.SetContextDetails(orchestrator, nodeID)
}

func Printf(format string, args ...any) {
	Log.Printf(format, args...)
}

func Debugf(format string, args ...any) {
	Log.Debugf(format, args...)
}

func Warnf(format string, args ...any) {
	Log.Warnf(format, args...)
}

func LogEvent(event aitelemetry.Event) {
	Log.LogEvent(event)
}

func Errorf(format string, args ...any) {
	Log.Errorf(format, args...)
}

func Request(tag string, request any, err error) {
	Log.Request(tag, request, err)
}

func Response(tag string, response any, returnCode types.ResponseCode, err error) {
	Log.Response(tag, response, returnCode, err)
}

func ResponseEx(tag string, request, response any, returnCode types.ResponseCode, err error) {
	Log.ResponseEx(tag, request, response, returnCode, err)
}

func SendMetric(metric aitelemetry.Metric) {
	Log.SendMetric(metric)
}
