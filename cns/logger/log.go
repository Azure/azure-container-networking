// Copyright Microsoft. All rights reserved.
package logger

import (
	"github.com/Azure/azure-container-networking/aitelemetry"
	"github.com/Azure/azure-container-networking/cns/types"
)

type loggershim interface {
	Close()
	InitAI(aitelemetry.AIConfig, bool, bool, bool)
	InitAIWithIKey(aitelemetry.AIConfig, string, bool, bool, bool)
	SetContextDetails(string, string)
	SetAPIServer(string)
	Printf(string, ...any)
	Debugf(string, ...any)
	Warnf(string, ...any)
	LogEvent(aitelemetry.Event)
	Errorf(string, ...any)
	Request(string, any, error)
	Response(string, any, types.ResponseCode, error)
	ResponseEx(string, any, any, types.ResponseCode, error)
	SendMetric(aitelemetry.Metric)
}

var (
	Log             loggershim
	AppInsightsIKey = aiMetadata
	aiMetadata      string // this var is set at build time.
)

// Deprecated: The global logger is deprecated. Migrate to zap using the cns/logger/v2 package and pass the logger instead.
func Close() {
	Log.Close()
}

// Deprecated: The global logger is deprecated. Migrate to zap using the cns/logger/v2 package and pass the logger instead.
func InitLogger(fileName string, logLevel, logTarget int, logDir string) {
	Log, _ = New(fileName, logLevel, logTarget, logDir)
}

// Deprecated: The global logger is deprecated. Migrate to zap using the cns/logger/v2 package and pass the logger instead.
func InitAI(aiConfig aitelemetry.AIConfig, disableTraceLogging, disableMetricLogging, disableEventLogging bool) {
	Log.InitAI(aiConfig, disableTraceLogging, disableMetricLogging, disableEventLogging)
}

// Deprecated: The global logger is deprecated. Migrate to zap using the cns/logger/v2 package and pass the logger instead.
func InitAIWithIKey(aiConfig aitelemetry.AIConfig, instrumentationKey string, disableTraceLogging, disableMetricLogging, disableEventLogging bool) {
	Log.InitAIWithIKey(aiConfig, instrumentationKey, disableTraceLogging, disableMetricLogging, disableEventLogging)
}

// Deprecated: The global logger is deprecated. Migrate to zap using the cns/logger/v2 package and pass the logger instead.
func SetContextDetails(orchestrator, nodeID string) {
	Log.SetContextDetails(orchestrator, nodeID)
}

// Deprecated: The global logger is deprecated. Migrate to zap using the cns/logger/v2 package and pass the logger instead.
func Printf(format string, args ...any) {
	Log.Printf(format, args...)
}

// Deprecated: The global logger is deprecated. Migrate to zap using the cns/logger/v2 package and pass the logger instead.
func Debugf(format string, args ...any) {
	Log.Debugf(format, args...)
}

// Deprecated: The global logger is deprecated. Migrate to zap using the cns/logger/v2 package and pass the logger instead.
func Warnf(format string, args ...any) {
	Log.Warnf(format, args...)
}

// Deprecated: The global logger is deprecated. Migrate to zap using the cns/logger/v2 package and pass the logger instead.
func LogEvent(event aitelemetry.Event) {
	Log.LogEvent(event)
}

// Deprecated: The global logger is deprecated. Migrate to zap using the cns/logger/v2 package and pass the logger instead.
func Errorf(format string, args ...any) {
	Log.Errorf(format, args...)
}

// Deprecated: The global logger is deprecated. Migrate to zap using the cns/logger/v2 package and pass the logger instead.
func Request(tag string, request any, err error) {
	Log.Request(tag, request, err)
}

// Deprecated: The global logger is deprecated. Migrate to zap using the cns/logger/v2 package and pass the logger instead.
func Response(tag string, response any, returnCode types.ResponseCode, err error) {
	Log.Response(tag, response, returnCode, err)
}

// Deprecated: The global logger is deprecated. Migrate to zap using the cns/logger/v2 package and pass the logger instead.
func ResponseEx(tag string, request, response any, returnCode types.ResponseCode, err error) {
	Log.ResponseEx(tag, request, response, returnCode, err)
}

// Deprecated: The global logger is deprecated. Migrate to zap using the cns/logger/v2 package and pass the logger instead.
func SendMetric(metric aitelemetry.Metric) {
	Log.SendMetric(metric)
}
