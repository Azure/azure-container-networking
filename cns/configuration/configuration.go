// Copyright Microsoft. All rights reserved.
package configuration

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/common"
	"github.com/pkg/errors"
)

const (
	// EnvCNSConfig is the CNS_CONFIGURATION_PATH env var key
	EnvCNSConfig      = "CNS_CONFIGURATION_PATH"
	defaultConfigName = "cns_config.json"
)

type CNSConfig struct {
	ChannelMode                 string
	EnablePprof                 bool
	EnableSubnetScarcity        bool
	InitializeFromCNI           bool
	ManagedSettings             ManagedSettings
	MetricsBindAddress          string
	SyncHostNCTimeoutMs         int
	SyncHostNCVersionIntervalMs int
	TLSCertificatePath          string
	TLSEndpoint                 string
	TLSPort                     string
	TLSSubjectName              string
	TelemetrySettings           TelemetrySettings
	UseHTTPS                    bool
	WireserverIP                string
	KeyVaultSettings            KeyVaultSettings
	MSISettings                 MSISettings
	ProgramSNATIPTables         bool
	ManageEndpointState         bool
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

type MSISettings struct {
	ResourceID string
}

type KeyVaultSettings struct {
	URL                  string
	CertificateName      string
	RefreshIntervalInHrs int
}

func getConfigFilePath(cmdLineConfigPath string) (string, error) {
	// If config path is set from cmd line, return that
	if cmdLineConfigPath != "" {
		return cmdLineConfigPath, nil
	}

	// Check if env set for config path otherwise use default path
	configpath, found := os.LookupEnv(EnvCNSConfig)
	if !found {
		dir, err := common.GetExecutableDirectory()
		if err != nil {
			return "", errors.Wrap(err, "failed to discover exec dir for config")
		}
		configpath = filepath.Join(dir, defaultConfigName)
	}
	return configpath, nil
}

// ReadConfig returns a CNS config from file or an error.
func ReadConfig(cmdLineConfigPath string) (*CNSConfig, error) {
	configpath, err := getConfigFilePath(cmdLineConfigPath)
	if err != nil {
		return nil, err
	}

	logger.Printf("[Configuration] Config path:%s", configpath)

	return readConfigFromFile(configpath)
}

func readConfigFromFile(f string) (*CNSConfig, error) {
	content, err := os.ReadFile(f)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read config file %s", f)
	}

	var config CNSConfig
	if err := json.Unmarshal(content, &config); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal config")
	}
	return &config, nil
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

func setKeyVaultSettingsDefaults(kvs *KeyVaultSettings) {
	if kvs.RefreshIntervalInHrs == 0 {
		kvs.RefreshIntervalInHrs = 12 //nolint:gomnd // default times
	}
}

// SetCNSConfigDefaults set default values of CNS config if not specified
func SetCNSConfigDefaults(config *CNSConfig) {
	setTelemetrySettingDefaults(&config.TelemetrySettings)
	setManagedSettingDefaults(&config.ManagedSettings)
	setKeyVaultSettingsDefaults(&config.KeyVaultSettings)

	if config.ChannelMode == "" {
		config.ChannelMode = cns.Direct
	}
	if config.MetricsBindAddress == "" {
		config.MetricsBindAddress = ":9090"
	}
	if config.SyncHostNCVersionIntervalMs == 0 {
		config.SyncHostNCVersionIntervalMs = 1000 //nolint:gomnd // default times
	}
	if config.SyncHostNCTimeoutMs == 0 {
		config.SyncHostNCTimeoutMs = 500 //nolint:gomnd // default times
	}
}
