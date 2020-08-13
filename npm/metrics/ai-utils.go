package metrics

import (
	"time"
	"strconv"

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
		BatchSize:                 32768,
		BatchInterval:             30,
		RefreshTimeout:            15,
		DebugMode:                 true,
		GetEnvRetryCount:          5,
		GetEnvRetryWaitTimeInSecs: 3,
	}

	var err error
	th, err = aitelemetry.NewAITelemetry("", aiMetadata, aiConfig)

	for i := 0; err != nil && i < 5; i++ {
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

// SendMetric sends
func SendMetric(metric aitelemetry.Metric) {
	if th == nil {
		log.Logf("AppInsights didn't initialized.")
		return
	}
	th.TrackMetric(metric)
}

// SendErrorMetric is responsible for sending error metrics trhough AI telemetry
func SendErrorMetric(errorCode int, packageName, functionName string) {
	customDimensions := map[string]string {
		util.PackageName: packageName,
		util.FunctionName: functionName,
		util.ErrorCode: strconv.Itoa(errorCode),
	}
	metric := aitelemetry.Metric{
		Name:             util.ErrorMetric,
		Value:            util.ErrorValue,
		CustomDimensions: customDimensions,
	}
	go SendMetric(metric)
}