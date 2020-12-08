// Copyright Microsoft. All rights reserved.
package configuration

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/common"
)

const (
	defaultConfigName = "cns_config.json"
)

type CNSConfig struct {
	TelemetrySettings  TelemetrySettings
	ManagedSettings    ManagedSettings
	HttpClientSettings HttpClientSettings
	ChannelMode        string
	UseHTTPS           bool
	TLSSubjectName     string
	TLSCertificatePath string
	TLSPort            string
	WireserverIP       string
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

// ManagedSettings indicate settings when CNS is running with managed DNC
type ManagedSettings struct {
	// DNS for DNC endpoint
	DncEndpointDns string
	// Network ID of the node where CNS is running
	InfrastructureNetworkID string
	// ID of the node where CNS is running
	NodeID string
	// Managed identity (MSI) client ID
	NodeManagedIdentity string
	// Interval between successive node sync call from CNS to DNC
	NodeSyncIntervalInSeconds int
}

// HttpClientSettings - Http client settings to be used when making http calls
type HttpClientSettings struct {
	// ConnectionTimeout indicates the timeout used to establish the connection
	ConnectionTimeout int
	// ResponseHeaderTimeout indicates the timeout used to get the header
	ResponseHeaderTimeout int
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

// Set Default values of CNS config if not specified
func SetCNSConfigDefaults(config *CNSConfig) {
	setTelemetrySettingDefaults(&config.TelemetrySettings)
	setManagedSettingDefaults(&config.ManagedSettings)
	setHttpSettingDefaults(&config.HttpClientSettings)
	if config.ChannelMode == "" {
		config.ChannelMode = cns.Direct
	}
}

func setHttpSettingDefaults(httpClientSettings *HttpClientSettings) {
	if httpClientSettings.ConnectionTimeout == 0 {
		// set the default connection timeout to 5 seconds
		httpClientSettings.ConnectionTimeout = 5
	}

	if httpClientSettings.ResponseHeaderTimeout == 0 {
		// set the default response header timeout to 120 seconds
		httpClientSettings.ResponseHeaderTimeout = 120
	}
}

// ValidateManagedSettings validates the ManagedSettings if CNS is running is the managed mode.
// This function validates if all the required fields are set in the configuration.
func ValidateManagedSettings(config *CNSConfig) bool {
	if config.ChannelMode == cns.Managed {
		if config.ManagedSettings.DncEndpointDns == "" ||
			config.ManagedSettings.InfrastructureNetworkID == "" ||
			config.ManagedSettings.NodeID == "" ||
			config.ManagedSettings.NodeManagedIdentity == "" {
			return false
		}
	}

	return true
}
