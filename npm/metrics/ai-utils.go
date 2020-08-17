package metrics

import (
	"fmt"
	"strconv"
	"time"

	"github.com/Azure/azure-container-networking/aitelemetry"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/util"
)

var (
	th aitelemetry.TelemetryHandle
)

// CreateTelemetryHandle creates
func CreateTelemetryHandle(version, aiMetadata string) error {

	aiConfig := aitelemetry.AIConfig{
		AppName:                   util.AzureNpmFlag,
		AppVersion:                version,
		BatchSize:                 util.BatchSize,
		BatchInterval:             util.BatchInterval,
		RefreshTimeout:            util.RefreshTimeout,
		DebugMode:                 util.DebugMode,
		GetEnvRetryCount:          util.GetEnvRetryCount,
		GetEnvRetryWaitTimeInSecs: util.GetEnvRetryWaitTimeInSecs,
	}

	var err error
	th, err = aitelemetry.NewAITelemetry("", aiMetadata, aiConfig)

	for i := 0; err != nil && i < util.AiInitializeRetryCount; i++ {
		log.Logf("Failed to init AppInsights with err: %+v for %d time", err, i + 1)
		time.Sleep(time.Minute * 5)
		th, err = aitelemetry.NewAITelemetry("", aiMetadata, aiConfig)
	}

	if err != nil {
		return err
	}

	if th != nil {
		log.Logf("Initialized AppInsights handle")
	}

	return nil
}

// SendErrorMetric is responsible for sending error metrics trhough AI telemetry
func SendErrorMetric(operationID int, format string, args ...interface{}) {
	// Send error metrics
	customDimensions := map[string]string {
		util.ErrorCode: strconv.Itoa(operationID),
	}
	metric := aitelemetry.Metric{
		Name:             util.ErrorMetric,
		Value:            util.ErrorValue,
		CustomDimensions: customDimensions,
	}
	SendMetric(metric)

	// Send error logs
	msg := fmt.Sprintf(format, args...)
	report := aitelemetry.Report{
		Message:          msg,
		Context:          strconv.Itoa(operationID),
		CustomDimensions: make(map[string]string),
	}
	SendLog(report)
}

// SendMetric sends metrics
func SendMetric(metric aitelemetry.Metric) {
	if th == nil {
			log.Logf("AppInsights didn't initialized.")
			return
	}
	th.TrackMetric(metric)
}

// SendLog sends log
func SendLog(report aitelemetry.Report) {
	if th == nil {
			log.Logf("AppInsights didn't initialized.")
			return
	}
	th.TrackLog(report)
}

