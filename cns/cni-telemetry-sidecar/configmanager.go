// Copyright Microsoft. All rights reserved.
package main

import (
	"encoding/json"
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

// ConfigManager handles CNS configuration loading for the telemetry sidecar.
// It loads config directly (without using configuration.ReadConfig()) to avoid
// dependency on the global cns/logger package, and applies sidecar-specific defaults.
type ConfigManager struct {
	configPath string
	logger     *zap.Logger
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
func (cm *ConfigManager) LoadConfig() (*configuration.CNSConfig, error) {
	configPath, err := cm.resolveConfigPath()
	if err != nil {
		cm.logger.Warn("Failed to resolve config path, using defaults", zap.Error(err))
		return cm.createDefaultConfig(), nil
	}

	cm.logger.Debug("Loading config from path", zap.String("path", configPath))

	config, err := cm.readConfigFromFile(configPath)
	if err != nil {
		cm.logger.Warn("Failed to load config file, using defaults", zap.Error(err))
		return cm.createDefaultConfig(), nil
	}

	cm.applyDefaults(config)

	cm.logger.Info("Loaded CNS configuration",
		zap.Bool("telemetryDisabled", config.TelemetrySettings.DisableAll),
		zap.Bool("cniTelemetryEnabled", config.TelemetrySettings.EnableCNITelemetry),
		zap.String("socketPath", config.TelemetrySettings.CNITelemetrySocketPath),
		zap.Bool("hasAppInsightsKey", cm.hasAppInsightsKey(&config.TelemetrySettings)))

	return config, nil
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
		return "", err
	}
	return filepath.Join(dir, defaultConfigName), nil
}

// readConfigFromFile reads and unmarshals the config file.
func (cm *ConfigManager) readConfigFromFile(path string) (*configuration.CNSConfig, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var config configuration.CNSConfig
	if err := json.Unmarshal(content, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

func (cm *ConfigManager) createDefaultConfig() *configuration.CNSConfig {
	return &configuration.CNSConfig{
		TelemetrySettings: configuration.TelemetrySettings{
			TelemetryBatchSizeBytes:      defaultBatchSizeInBytes,
			TelemetryBatchIntervalInSecs: defaultBatchIntervalInSecs,
			RefreshIntervalInSecs:        defaultRefreshTimeoutInSecs,
			CNITelemetrySocketPath:       defaultTelemetrySocketPath,
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
	if ts.CNITelemetrySocketPath == "" {
		ts.CNITelemetrySocketPath = defaultTelemetrySocketPath
	}
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
