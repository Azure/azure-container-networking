package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Azure/azure-container-networking/cns/configuration"
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
	// Use zap logger if available, otherwise create a default config
	if cm.logger != nil {
		cm.logger.Debug("Loading CNS configuration", zap.String("path", cm.configPath))
	}

	// Check if config file exists
	if _, err := os.Stat(cm.configPath); os.IsNotExist(err) {
		if cm.logger != nil {
			cm.logger.Info("CNS config file not found, using default configuration",
				zap.String("path", cm.configPath))
		}

		// Return default configuration
		return &configuration.CNSConfig{
			TelemetrySettings: configuration.TelemetrySettings{
				DisableAll:                    false,
				TelemetryBatchSizeBytes:       16384,
				TelemetryBatchIntervalInSecs:  15,
				RefreshIntervalInSecs:         15,
				DisableMetadataRefreshThread:  false,
				DebugMode:                     false,
				DisableTrace:                  false,
				DisableMetric:                 false,
				DisableEvent:                  false,
				AppInsightsInstrumentationKey: "", // Will be set by environment or config
			},
		}, nil
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

	// Set default values for telemetry settings if not specified
	if config.TelemetrySettings.TelemetryBatchSizeBytes == 0 {
		config.TelemetrySettings.TelemetryBatchSizeBytes = 16384
	}
	if config.TelemetrySettings.TelemetryBatchIntervalInSecs == 0 {
		config.TelemetrySettings.TelemetryBatchIntervalInSecs = 15
	}
	if config.TelemetrySettings.RefreshIntervalInSecs == 0 {
		config.TelemetrySettings.RefreshIntervalInSecs = 15
	}

	if cm.logger != nil {
		cm.logger.Info("Successfully loaded CNS configuration",
			zap.String("path", cm.configPath),
			zap.Bool("telemetryDisabled", config.TelemetrySettings.DisableAll))
	}

	return &config, nil
}
