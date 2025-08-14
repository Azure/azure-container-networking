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
	defaultConfigName          = "cns_config.json"
	defaultTelemetrySocketPath = "/var/run/azure-vnet-telemetry.sock"
)

// appInsightsEnvVars are the environment variables checked for AppInsights keys.
var appInsightsEnvVars = []string{
	"APPINSIGHTS_INSTRUMENTATIONKEY",
	"APPLICATIONINSIGHTS_CONNECTION_STRING",
	"AI_INSTRUMENTATION_KEY",
}

// ConfigManager handles CNS configuration loading for the telemetry sidecar.
type ConfigManager struct {
	configPath string
	logger     *zap.Logger
}

// NewConfigManager creates a new ConfigManager.
// Config path resolution priority:
//  1. Command line flag (if provided)
//  2. CNS_CONFIGURATION_PATH environment variable
//  3. Default path ({executable_directory}/cns_config.json)
func NewConfigManager(cmdConfigPath string) *ConfigManager {
	return &ConfigManager{
		configPath: resolveConfigPath(cmdConfigPath),
	}
}

func resolveConfigPath(cmdPath string) string {
	if strings.TrimSpace(cmdPath) != "" {
		return cmdPath
	}
	if envPath := os.Getenv(configuration.EnvCNSConfig); strings.TrimSpace(envPath) != "" {
		return envPath
	}
	dir, err := common.GetExecutableDirectory()
	if err != nil {
		return defaultConfigName
	}
	return filepath.Join(dir, defaultConfigName)
}

// GetConfigPath returns the resolved config path.
func (cm *ConfigManager) GetConfigPath() string {
	return cm.configPath
}

// SetLogger sets the logger for the ConfigManager.
func (cm *ConfigManager) SetLogger(logger *zap.Logger) {
	cm.logger = logger
}

// LoadConfig loads and validates the CNS configuration from file.
func (cm *ConfigManager) LoadConfig() (*configuration.CNSConfig, error) {
	if _, err := os.Stat(cm.configPath); os.IsNotExist(err) {
		if cm.logger != nil {
			cm.logger.Info("Config file not found, using defaults", zap.String("path", cm.configPath))
		}
		return cm.createDefaultConfig(), nil
	}

	data, err := os.ReadFile(cm.configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", cm.configPath, err)
	}

	var config configuration.CNSConfig
	if err := json.Unmarshal(data, &config); err != nil { //nolint:musttag // CNSConfig has json tags in configuration package
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	cm.applyDefaults(&config)

	if cm.logger != nil {
		cm.logger.Info("Loaded CNS configuration",
			zap.String("path", cm.configPath),
			zap.Bool("telemetryDisabled", config.TelemetrySettings.DisableAll),
			zap.Bool("cniTelemetryEnabled", config.TelemetrySettings.EnableCNITelemetry),
			zap.String("socketPath", config.TelemetrySettings.CNITelemetrySocketPath),
			zap.Bool("hasAppInsightsKey", cm.hasAppInsightsKey(&config.TelemetrySettings)))
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
// build-time (aiMetadata), config file, or environment variables.
func (cm *ConfigManager) hasAppInsightsKey(ts *configuration.TelemetrySettings) bool {
	if telemetry.GetAIMetadata() != "" {
		return true
	}
	if ts.AppInsightsInstrumentationKey != "" {
		return true
	}
	for _, env := range appInsightsEnvVars {
		if os.Getenv(env) != "" {
			return true
		}
	}
	return false
}
