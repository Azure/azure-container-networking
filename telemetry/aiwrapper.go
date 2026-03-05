// Copyright Microsoft. All rights reserved.
package telemetry

import (
	"errors"

	"github.com/Azure/azure-container-networking/aitelemetry"
	"github.com/Azure/azure-container-networking/log"
)

var (
	aiMetadata               string
	connectionString         string
	enableAIInSovereignCloud bool
	th                       aitelemetry.TelemetryHandle
	gDisableTrace            bool
	gDisableMetric           bool
	ErrTelemetryDisabled     = errors.New("telemetry is disabled")
)

const (
	// Wait time for AI to gracefully close AI telemetry session
	waitTimeInSecs = 10
)

func (tb *TelemetryBuffer) CreateAITelemetryHandle(aiConfig aitelemetry.AIConfig, disableAll, disableMetric, disableTrace bool) error {
	var err error

	if disableAll {
		if tb.logger != nil {
			tb.logger.Info("Telemetry is disabled")
		} else {
			log.Printf("Telemetry is disabled")
		}
		return ErrTelemetryDisabled
	}

	// Use connection string only if sovereign cloud is enabled
	if enableAIInSovereignCloud {
		th, err = aitelemetry.NewWithConnectionString(connectionString, aiConfig)
		if err != nil {
			return err
		}
	} else {
		th, err = aitelemetry.NewAITelemetry("", aiMetadata, aiConfig)
		if err != nil {
			return err
		}
	}

	gDisableMetric = disableMetric
	gDisableTrace = disableTrace
	return nil
}

func SendAITelemetry(cnireport CNIReport) {
	if th == nil || gDisableTrace {
		return
	}

	var msg string
	if cnireport.ErrorMessage != "" {
		msg = cnireport.ErrorMessage
	} else if cnireport.EventMessage != "" {
		msg = cnireport.EventMessage
	} else {
		return
	}

	report := aitelemetry.Report{
		Message:          msg,
		Context:          cnireport.ContainerName,
		AppVersion:       cnireport.Version,
		CustomDimensions: make(map[string]string),
	}

	report.CustomDimensions[ContextStr] = cnireport.Context
	report.CustomDimensions[SubContextStr] = cnireport.SubContext
	report.CustomDimensions[VMUptimeStr] = cnireport.VMUptime
	report.CustomDimensions[OperationTypeStr] = cnireport.OperationType
	report.CustomDimensions[VersionStr] = cnireport.Version

	th.TrackLog(report)
}

func SendAIMetric(aiMetric AIMetric) {
	if th == nil || gDisableMetric {
		return
	}

	th.TrackMetric(aiMetric.Metric)
}

func CloseAITelemetryHandle() {
	if th != nil {
		th.Close(waitTimeInSecs)
	}
}

// GetAIMetadata returns the current aiMetadata value
func GetAIMetadata() string {
	return aiMetadata
}

// SetAIMetadata sets the aiMetadata value (for runtime configuration)
func SetAIMetadata(metadata string) {
	aiMetadata = metadata
}

// GetAIConnectionString returns the current AI connection string value
func GetAIConnectionString() string {
	return connectionString
}

// SetAIConnectionString sets the AI connection string value (for runtime configuration)
func SetAIConnectionString(connStr string) {
	connectionString = connStr
}

// GetEnableAIInSovereignCloud returns the current AI sovereign cloud flag value
func GetEnableAIInSovereignCloud() bool {
	return enableAIInSovereignCloud
}

// SetEnableAIInSovereignCloud sets the AI sovereign cloud flag (for runtime configuration)
func SetEnableAIInSovereignCloud(isSovereign bool) {
	enableAIInSovereignCloud = isSovereign
}
