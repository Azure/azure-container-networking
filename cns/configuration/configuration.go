package configuration

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/Azure/azure-container-networking/cns/logger"
)

const (
	defaultConfigName = "config.json"
)

type CNSConfig struct {
	TelemetrySettings TelemetrySettings
}

type TelemetrySettings struct {
	// Flag to disable the telemetry.
	DisableAll bool
	// Flag to Disable sending trace.
	DisableTrace bool
	// Flag to Disable sending metric.
	DisableMetric bool
	// Configure how many bytes can be sent in one call to the data collector
	TelemetryBatchSizeBytes int
	// Configure the maximum delay before sending queued telemetry in milliseconds
	TelemetryBatchIntervalInSecs int
	// Enable thread for getting metadata from wireserver
	DisableMetadataRefreshThread bool
	// Refresh interval in milliseconds for metadata thread
	RefreshIntervalInSecs int
	// Disable debug logging for telemetry messages
	DebugMode bool
}

func ReadConfig() (CNSConfig, error) {
	var cnsConfig CNSConfig

	// Check if env set for config path otherwise use default path
	configpath, found := os.LookupEnv("CNS_CONFIGURATION_PATH")
	if !found {
		dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
		if err != nil {
			logger.Errorf("[Configuration] Failed to find exe dir:%v", err)
			return cnsConfig, err
		}

		configpath = dir + string(os.PathSeparator) + defaultConfigName
	}

	content, err := ioutil.ReadFile(configpath)
	if err != nil {
		logger.Errorf("[Configuration] Failed to read config file :%v", err)
		return cnsConfig, err
	}

	err = json.Unmarshal(content, &cnsConfig)
	return cnsConfig, err
}

func setTelemetrySettingDefaults(telemetrySettings TelemetrySettings) {
	if telemetrySettings.RefreshIntervalInSecs == 0 {
		// set the default refresh interval of metadata thread to 15 seconds
		telemetrySettings.RefreshIntervalInSecs = 15
	}

	if telemetrySettings.TelemetryBatchIntervalInSecs == 0 {
		// set the default AI telemetry batch interval to 30 seconds
		telemetrySettings.TelemetryBatchIntervalInSecs = 30
	}

	if telemetrySettings.TelemetryBatchSizeBytes == 0 {
		// set the default AI telemetry batch size to 32768 bytes
		telemetrySettings.TelemetryBatchSizeBytes = 32768
	}
}

func SetCNSConfigDefaults(config *CNSConfig) {
	setTelemetrySettingDefaults(config.TelemetrySettings)
}
