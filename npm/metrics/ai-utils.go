package metrics

import (
	"fmt"
	"strconv"
	"time"

	"github.com/Azure/azure-container-networking/aitelemetry"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/util"
	"k8s.io/klog"
)

var (
	th         aitelemetry.TelemetryHandle
	npmVersion int
	PrintLog   = true
	DonotPrint = false
)

// CreateTelemetryHandle creates a handler to initialize AI telemetry
func CreateTelemetryHandle(npmVersionNum int, imageVersion, aiMetadata string) error {
	npmVersion = npmVersionNum
	aiConfig := aitelemetry.AIConfig{
		AppName:                   util.AzureNpmFlag,
		AppVersion:                imageVersion,
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

// SendErrorLogAndMetric sends a metric through AI telemetry and sends a log to the Kusto Messages table
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
	// SendMetric prints out a log visible in kubectl logs
	SendMetric(metric)

	// Send error logs
	msg := fmt.Sprintf(format, args...)
	log.Errorf(msg)
	SendLog(operationID, msg, DonotPrint)
}

// SendMetric sends metrics
func SendMetric(metric aitelemetry.Metric) {
	if th == nil {
		return
	}
	th.TrackMetric(metric)
}

// SendLog sends log
func SendLog(operationID int, msg string, printLog bool) {
	msg = fmt.Sprintf("%s - (NPM v%d)", msg, npmVersion)
	report := aitelemetry.Report{
		Message:          msg,
		Context:          strconv.Itoa(operationID),
		CustomDimensions: make(map[string]string),
	}
	if printLog {
		klog.Infof(msg)
	}
	if th == nil {
		return
	}
	th.TrackLog(report)
}

func SendHeartbeatWithNumPolicies() {
	var message string
	numPolicies, err := GetNumPolicies()
	if err == nil {
		message = fmt.Sprintf("info: NPM heartbeat. Current num policies: %d", numPolicies)
	} else {
		message = fmt.Sprintf("warn: NPM hearbeat. Couldn't get number of policies for telemetry log: %v", err)
		klog.Warning(message)
	}
	SendLog(util.NpmID, message, DonotPrint)
}
