// Copyright Microsoft. All rights reserved.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Azure/azure-container-networking/cns/configuration"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/telemetry"
	"go.uber.org/zap"
)

const (
	defaultTelemetrySocketPath = "/var/run/azure-vnet-telemetry.sock"
	defaultConfigName          = "cns_config.json"
	// appInsightsEnvVar is the standard environment variable for AppInsights instrumentation key.
	// Note: Connection strings (APPLICATIONINSIGHTS_CONNECTION_STRING) require different handling
	// and are not supported here.
	appInsightsEnvVar = "APPINSIGHTS_INSTRUMENTATIONKEY"
	// envCNSConfig is the environment variable for CNS config path.
	envCNSConfig = "CNS_CONFIGURATION_PATH"
)

// SidecarTelemetrySettings contains telemetry settings specific to the CNI telemetry sidecar.
// These fields are parsed from the same configmap as CNSConfig, but are only used by the sidecar.
// Go's JSON unmarshaling ignores unknown fields, so CNS discards these when parsing its config.
type SidecarTelemetrySettings struct {
	// Flag to enable CNI telemetry collection via sidecar
	EnableCNITelemetry bool `json:"EnableCNITelemetry"`
	// Path to the CNI telemetry socket file that azure-vnet CNI connects to
	CNITelemetrySocketPath string `json:"CNITelemetrySocketPath"`
}

// SidecarConfig wraps the sidecar-specific telemetry settings.
// It is used to parse sidecar-specific fields from the CNS configmap.
type SidecarConfig struct {
	TelemetrySettings SidecarTelemetrySettings `json:"TelemetrySettings"`
}

// ConfigManager handles CNS configuration loading for the telemetry sidecar.
// It loads config directly (without using configuration.ReadConfig()) to avoid
// dependency on the global cns/logger package, and applies sidecar-specific defaults.
type ConfigManager struct {
	configPath    string
	logger        *zap.Logger
	sidecarConfig *SidecarConfig
}

// NewConfigManager creates a new ConfigManager.
func NewConfigManager(cmdConfigPath string, logger *zap.Logger) *ConfigManager {
	return &ConfigManager{
		configPath: cmdConfigPath,
		logger:     logger,
	}
}

// GetConfigPath returns the config path that will be used.
func (cm *ConfigManager) GetConfigPath() string {
	return cm.configPath
}

// LoadConfig loads the CNS configuration from file and applies sidecar-specific defaults.
// This method loads the config directly to avoid depending on the global cns/logger package.
// It always returns a valid config, falling back to defaults if loading fails.
func (cm *ConfigManager) LoadConfig() *configuration.CNSConfig {
	configPath, err := cm.resolveConfigPath()
	if err != nil {
		cm.logger.Warn("Failed to resolve config path, using defaults", zap.Error(err))
		return cm.createDefaultConfig()
	}

	cm.logger.Debug("Loading config from path", zap.String("path", configPath))

	config, sidecarConfig, err := cm.readConfigFromFile(configPath)
	if err != nil {
		cm.logger.Warn("Failed to load config file, using defaults", zap.Error(err))
		return cm.createDefaultConfig()
	}

	cm.sidecarConfig = sidecarConfig
	cm.applySidecarDefaults()
	cm.applyDefaults(config)

	cm.logger.Info("Loaded CNS configuration",
		zap.Bool("telemetryDisabled", config.TelemetrySettings.DisableAll),
		zap.Bool("cniTelemetryEnabled", cm.sidecarConfig.TelemetrySettings.EnableCNITelemetry),
		zap.String("socketPath", cm.sidecarConfig.TelemetrySettings.CNITelemetrySocketPath),
		zap.Bool("hasAppInsightsKey", cm.hasAppInsightsKey(&config.TelemetrySettings)))

	return config
}

// resolveConfigPath determines the config file path from command line, environment, or default.
func (cm *ConfigManager) resolveConfigPath() (string, error) {
	// If config path is set from cmd line, return that.
	if strings.TrimSpace(cm.configPath) != "" {
		return cm.configPath, nil
	}
	// If config path is set from env, return that.
	if envPath := os.Getenv(envCNSConfig); strings.TrimSpace(envPath) != "" {
		return envPath, nil
	}
	// Otherwise compose the default config path and return that.
	dir, err := common.GetExecutableDirectory()
	if err != nil {
		return "", fmt.Errorf("failed to get executable directory: %w", err)
	}
	return filepath.Join(dir, defaultConfigName), nil
}

// readConfigFromFile reads and unmarshals the config file into both CNSConfig and SidecarConfig.
// Go's JSON unmarshaling ignores unknown fields, so each struct gets only the fields it defines.
func (cm *ConfigManager) readConfigFromFile(path string) (*configuration.CNSConfig, *SidecarConfig, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	var config configuration.CNSConfig
	//nolint:musttag // CNSConfig is from cns/configuration package and uses default json field matching
	if err := json.Unmarshal(content, &config); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal CNS config: %w", err)
	}

	var sidecarConfig SidecarConfig
	if err := json.Unmarshal(content, &sidecarConfig); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal sidecar config: %w", err)
	}

	return &config, &sidecarConfig, nil
}

func (cm *ConfigManager) createDefaultConfig() *configuration.CNSConfig {
	cm.sidecarConfig = &SidecarConfig{
		TelemetrySettings: SidecarTelemetrySettings{
			CNITelemetrySocketPath: defaultTelemetrySocketPath,
		},
	}
	return &configuration.CNSConfig{
		TelemetrySettings: configuration.TelemetrySettings{
			TelemetryBatchSizeBytes:      defaultBatchSizeInBytes,
			TelemetryBatchIntervalInSecs: defaultBatchIntervalInSecs,
			RefreshIntervalInSecs:        defaultRefreshTimeoutInSecs,
		},
	}
}

func (cm *ConfigManager) applyDefaults(config *configuration.CNSConfig) {
	ts := &config.TelemetrySettings
	if ts.TelemetryBatchSizeBytes == 0 {
		ts.TelemetryBatchSizeBytes = defaultBatchSizeInBytes
	}
	if ts.TelemetryBatchIntervalInSecs == 0 {
		ts.TelemetryBatchIntervalInSecs = defaultBatchIntervalInSecs
	}
	if ts.RefreshIntervalInSecs == 0 {
		ts.RefreshIntervalInSecs = defaultRefreshTimeoutInSecs
	}
}

func (cm *ConfigManager) applySidecarDefaults() {
	if cm.sidecarConfig.TelemetrySettings.CNITelemetrySocketPath == "" {
		cm.sidecarConfig.TelemetrySettings.CNITelemetrySocketPath = defaultTelemetrySocketPath
	}
}

// GetSidecarConfig returns the sidecar-specific configuration.
func (cm *ConfigManager) GetSidecarConfig() *SidecarConfig {
	return cm.sidecarConfig
}

// hasAppInsightsKey checks if an AppInsights key is available from any source:
// build-time (aiMetadata), config file, or environment variable.
func (cm *ConfigManager) hasAppInsightsKey(ts *configuration.TelemetrySettings) bool {
	if telemetry.GetAIMetadata() != "" {
		return true
	}
	if ts.AppInsightsInstrumentationKey != "" {
		return true
	}
	return os.Getenv(appInsightsEnvVar) != ""
}
