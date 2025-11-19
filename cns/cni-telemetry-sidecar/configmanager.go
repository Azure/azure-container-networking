package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Azure/azure-container-networking/cns/configuration"
	"github.com/Azure/azure-container-networking/telemetry"
	"go.uber.org/zap"
)

// ConfigManager handles CNS configuration loading
type ConfigManager struct {
	configPath string
	logger     *zap.Logger
}

// NewConfigManager creates a new config manager
func NewConfigManager(configPath string) *ConfigManager {
	return &ConfigManager{
		configPath: configPath,
	}
}

// SetLogger sets the zap logger for the config manager
func (cm *ConfigManager) SetLogger(logger *zap.Logger) {
	cm.logger = logger
}

// LoadConfig loads the CNS configuration from file
func (cm *ConfigManager) LoadConfig() (*configuration.CNSConfig, error) {
	if cm.logger != nil {
		cm.logger.Debug("Loading CNS configuration", zap.String("path", cm.configPath))
	}

	// Check if config file exists
	if _, err := os.Stat(cm.configPath); os.IsNotExist(err) {
		if cm.logger != nil {
			cm.logger.Info("CNS config file not found, using default configuration",
				zap.String("path", cm.configPath))
		}
		return cm.createDefaultConfig(), nil
	}

	// Read the config file
	data, err := os.ReadFile(cm.configPath)
	if err != nil {
		if cm.logger != nil {
			cm.logger.Error("Failed to read CNS config file",
				zap.String("path", cm.configPath),
				zap.Error(err))
		}
		return nil, fmt.Errorf("failed to read config file %s: %w", cm.configPath, err)
	}

	// Parse the JSON configuration
	var config configuration.CNSConfig
	if err := json.Unmarshal(data, &config); err != nil {
		if cm.logger != nil {
			cm.logger.Error("Failed to parse CNS config file",
				zap.String("path", cm.configPath),
				zap.Error(err))
		}
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Apply defaults and environment variable overrides
	cm.setConfigDefaults(&config)

	// Check for AppInsights key from all sources (build-time, config, env)
	hasAppInsightsKey := cm.hasEffectiveAppInsightsKey(&config.TelemetrySettings)

	if cm.logger != nil {
		cm.logger.Info("Successfully loaded CNS configuration",
			zap.String("path", cm.configPath),
			zap.Bool("telemetryDisabled", config.TelemetrySettings.DisableAll),
			zap.Bool("cniTelemetryEnabled", config.TelemetrySettings.EnableCNITelemetry),
			zap.String("socketPath", config.TelemetrySettings.CNITelemetrySocketPath),
			zap.Bool("hasAppInsightsKey", hasAppInsightsKey))
	}

	return &config, nil
}

// createDefaultConfig creates a default configuration
func (cm *ConfigManager) createDefaultConfig() *configuration.CNSConfig {
	config := &configuration.CNSConfig{
		TelemetrySettings: configuration.TelemetrySettings{
			DisableAll:                   false,
			TelemetryBatchSizeBytes:      defaultBatchSizeInBytes,
			TelemetryBatchIntervalInSecs: defaultBatchIntervalInSecs,
			RefreshIntervalInSecs:        defaultRefreshTimeoutInSecs,
			DisableMetadataRefreshThread: false,
			DebugMode:                    false,
			DisableTrace:                 false,
			DisableMetric:                false,
			DisableEvent:                 false,
			EnableCNITelemetry:           false, // Default to false
			CNITelemetrySocketPath:       "/var/run/azure-vnet-telemetry.sock",
		},
	}

	// Set AppInsights key from environment variables (if any)
	cm.setAppInsightsKeyFromEnv(&config.TelemetrySettings)

	return config
}

// setConfigDefaults applies default values and environment variable overrides
func (cm *ConfigManager) setConfigDefaults(config *configuration.CNSConfig) {
	// Set default values for telemetry settings if not specified
	if config.TelemetrySettings.TelemetryBatchSizeBytes == 0 {
		config.TelemetrySettings.TelemetryBatchSizeBytes = defaultBatchSizeInBytes
	}
	if config.TelemetrySettings.TelemetryBatchIntervalInSecs == 0 {
		config.TelemetrySettings.TelemetryBatchIntervalInSecs = defaultBatchIntervalInSecs
	}
	if config.TelemetrySettings.RefreshIntervalInSecs == 0 {
		config.TelemetrySettings.RefreshIntervalInSecs = defaultRefreshTimeoutInSecs
	}

	// Set default CNI telemetry socket path
	if config.TelemetrySettings.CNITelemetrySocketPath == "" {
		config.TelemetrySettings.CNITelemetrySocketPath = "/var/run/azure-vnet-telemetry.sock"
	}

	// Handle AppInsights instrumentation key from environment variables
	cm.setAppInsightsKeyFromEnv(&config.TelemetrySettings)
}

// setAppInsightsKeyFromEnv sets the AppInsights instrumentation key from environment variables
func (cm *ConfigManager) setAppInsightsKeyFromEnv(ts *configuration.TelemetrySettings) {
	// Try multiple environment variable names
	envKeys := []string{
		"APPINSIGHTS_INSTRUMENTATIONKEY",
		"APPLICATIONINSIGHTS_CONNECTION_STRING",
		"AI_INSTRUMENTATION_KEY",
	}

	// If no key is set in config, try environment variables
	if ts.AppInsightsInstrumentationKey == "" {
		for _, envKey := range envKeys {
			if key := os.Getenv(envKey); key != "" {
				ts.AppInsightsInstrumentationKey = key
				if cm.logger != nil {
					cm.logger.Debug("Found AppInsights key in environment variable",
						zap.String("envVar", envKey))
				}
				break
			}
		}
	}
}

// hasEffectiveAppInsightsKey checks if AppInsights key is available from any source
// (build-time aiMetadata, config file, or environment variables)
func (cm *ConfigManager) hasEffectiveAppInsightsKey(ts *configuration.TelemetrySettings) bool {
	// Priority 1: Build-time embedded key via telemetry.aiMetadata
	if buildTimeKey := telemetry.GetAIMetadata(); buildTimeKey != "" {
		return true
	}

	// Priority 2: Config file
	if ts.AppInsightsInstrumentationKey != "" {
		return true
	}

	// Priority 3: Environment variables
	envKeys := []string{
		"APPINSIGHTS_INSTRUMENTATIONKEY",
		"APPLICATIONINSIGHTS_CONNECTION_STRING",
		"AI_INSTRUMENTATION_KEY",
	}

	for _, envKey := range envKeys {
		if key := os.Getenv(envKey); key != "" {
			return true
		}
	}

	return false
}
