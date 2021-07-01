// Copyright Microsoft. All rights reserved.
package configuration

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/common"
)

const (
	defaultConfigName = "cns_config.json"
)

type CNSConfig struct {
	ChannelMode                 string
	InitializeFromCNI           bool
	ManagedSettings             ManagedSettings
	SyncHostNCTimeoutMs         time.Duration
	SyncHostNCVersionIntervalMs time.Duration
	TLSCertificatePath          string
	TLSEndpoint                 string
	TLSPort                     string
	TLSSubjectName              string
	TelemetrySettings           TelemetrySettings
	UseHTTPS                    bool
	WireserverIP                string
}

type TelemetrySettings struct {
	// Flag to disable the telemetry.
	DisableAll bool
	// Flag to Disable sending trace.
	DisableTrace bool
	// Flag to Disable sending metric.
	DisableMetric bool
	// Flag to Disable sending events.
	DisableEvent bool
	// Configure how many bytes can be sent in one call to the data collector
	TelemetryBatchSizeBytes int
	// Configure the maximum delay before sending queued telemetry in milliseconds
	TelemetryBatchIntervalInSecs int
	// Heartbeat interval for sending heartbeat metric
	HeartBeatIntervalInMins int
	// Enable thread for getting metadata from wireserver
	DisableMetadataRefreshThread bool
	// Refresh interval in milliseconds for metadata thread
	RefreshIntervalInSecs int
	// Disable debug logging for telemetry messages
	DebugMode bool
	// Interval for sending snapshot events.
	SnapshotIntervalInMins int
}

type ManagedSettings struct {
	PrivateEndpoint           string
	InfrastructureNetworkID   string
	NodeID                    string
	NodeSyncIntervalInSeconds int
}

// This functions reads cns config file and save it in a structure
func ReadConfig() (CNSConfig, error) {
	var cnsConfig CNSConfig

	// Check if env set for config path otherwise use default path
	configpath, found := os.LookupEnv("CNS_CONFIGURATION_PATH")
	if !found {
		dir, err := common.GetExecutableDirectory()
		if err != nil {
			logger.Errorf("[Configuration] Failed to find exe dir:%v", err)
			return cnsConfig, err
		}

		configpath = filepath.Join(dir, defaultConfigName)
	}

	logger.Printf("[Configuration] Config path:%s", configpath)

	content, err := ioutil.ReadFile(configpath)
	if err != nil {
		logger.Errorf("[Configuration] Failed to read config file :%v", err)
		return cnsConfig, err
	}

	err = json.Unmarshal(content, &cnsConfig)
	return cnsConfig, err
}

// set telmetry setting defaults
func setTelemetrySettingDefaults(telemetrySettings *TelemetrySettings) {
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

	if telemetrySettings.HeartBeatIntervalInMins == 0 {
		// set the default Heartbeat interval to 30 minutes
		telemetrySettings.HeartBeatIntervalInMins = 30
	}

	if telemetrySettings.SnapshotIntervalInMins == 0 {
		telemetrySettings.SnapshotIntervalInMins = 60
	}
}

// set managed setting defaults
func setManagedSettingDefaults(managedSettings *ManagedSettings) {
	if managedSettings.NodeSyncIntervalInSeconds == 0 {
		managedSettings.NodeSyncIntervalInSeconds = 30
	}
}

// SetCNSConfigDefaults set default values of CNS config if not specified
func SetCNSConfigDefaults(config *CNSConfig) {
	setTelemetrySettingDefaults(&config.TelemetrySettings)
	setManagedSettingDefaults(&config.ManagedSettings)
	if config.ChannelMode == "" {
		config.ChannelMode = cns.Direct
	}
	config.SyncHostNCVersionIntervalMs = 1000
	config.SyncHostNCTimeoutMs = 500
}
