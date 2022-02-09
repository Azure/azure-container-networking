package metrics

import (
	"fmt"
	"strconv"
	"time"

	"github.com/Azure/azure-container-networking/aitelemetry"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/util"
)

var th aitelemetry.TelemetryHandle

// CreateTelemetryHandle creates a handler to initialize AI telemetry
func CreateTelemetryHandle(version, aiMetadata string) error {
	aiConfig := aitelemetry.AIConfig{
		AppName:                   util.AzureNpmFlag,
		AppVersion:                version,
		BatchSize:                 util.BatchSizeInBytes,
		BatchInterval:             util.BatchIntervalInSecs,
		RefreshTimeout:            util.RefreshTimeoutInSecs,
		DebugMode:                 util.DebugMode,
		GetEnvRetryCount:          util.GetEnvRetryCount,
		GetEnvRetryWaitTimeInSecs: util.GetEnvRetryWaitTimeInSecs,
	}

	var err error
	for i := 0; i < util.AiInitializeRetryCount; i++ {
		th, err = aitelemetry.NewAITelemetry("", aiMetadata, aiConfig)
		if err != nil {
			log.Logf("Failed to init AppInsights with err: %+v for %d time", err, i+1)
			time.Sleep(time.Minute * time.Duration(util.AiInitializeRetryInMin))
		} else {
			break
		}
	}

	if err != nil {
		return err
	}

	if th != nil {
		log.Logf("Initialized AppInsights handle")
	}

	return nil
}

// SendErrorLogAndMetric is responsible for sending log and error metrics through AI telemetry
func SendErrorLogAndMetric(operationID int, format string, args ...interface{}) {
	// Send error metrics
	customDimensions := map[string]string{
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
	log.Errorf(msg)
	SendLog(operationID, msg)
}

// SendMetric sends metrics
func SendMetric(metric aitelemetry.Metric) {
	if th == nil {
		log.Logf("AppInsights didn't initialize")
		return
	}
	th.TrackMetric(metric)
}

// SendLog sends log
func SendLog(operationID int, msg string) {
	report := aitelemetry.Report{
		Message:          msg,
		Context:          strconv.Itoa(operationID),
		CustomDimensions: make(map[string]string),
	}
	if th == nil {
		log.Logf("AppInsights didn't initialized.")
		return
	}
	th.TrackLog(report)
}

func SendHeartbeatLog() {
	numPolicies, err := GetNumPolicies()
	if err != nil {
		message := fmt.Sprintf("Info: NPM currently has %d policies", numPolicies)
		SendLog(util.NpmID, message)
	}
}
