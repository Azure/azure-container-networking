package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/Azure/azure-container-networking/cns/configuration"
	"github.com/Azure/azure-container-networking/cns/logger"
)

// ConfigManager handles Azure CNS configuration loading and validation
type ConfigManager struct {
	configPath string
}

// NewConfigManager creates a new configuration manager for Azure CNS
func NewConfigManager(configPath string) *ConfigManager {
	return &ConfigManager{
		configPath: configPath,
	}
}

// LoadConfig loads and validates the Azure CNS configuration
func (cm *ConfigManager) LoadConfig() (*configuration.CNSConfig, error) {
	logger.Printf("Loading Azure CNS configuration from: %s", cm.configPath)

	// Wait for configuration file to become available (Kubernetes ConfigMap mount)
	if err := cm.waitForConfigFile(); err != nil {
		return nil, fmt.Errorf("Azure CNS configuration file not available: %w", err)
	}

	// Read the configuration file from mounted volume
	data, err := os.ReadFile(cm.configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read Azure CNS configuration file: %w", err)
	}

	// Parse JSON configuration into CNS config structure
	var config configuration.CNSConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse Azure CNS configuration: %w", err)
	}

	// Validate configuration for Azure telemetry requirements
	if err := cm.validateConfig(&config); err != nil {
		return nil, fmt.Errorf("Azure CNS configuration validation failed: %w", err)
	}

	logger.Printf("Azure CNS configuration loaded and validated successfully")
	return &config, nil
}

// waitForConfigFile waits for the configuration file to become available
// This is important in Kubernetes environments where ConfigMaps are mounted asynchronously
func (cm *ConfigManager) waitForConfigFile() error {
	const maxRetries = 30
	const retryInterval = 2 * time.Second

	for i := 0; i < maxRetries; i++ {
		if _, err := os.Stat(cm.configPath); err == nil {
			return nil
		}

		if i == 0 {
			logger.Printf("Waiting for Azure CNS configuration file to become available...")
		}

		time.Sleep(retryInterval)
	}

	return fmt.Errorf("Azure CNS configuration file not available after %d attempts (%v total wait time)",
		maxRetries, time.Duration(maxRetries)*retryInterval)
}

// validateConfig performs Azure-specific validation of the CNS configuration
func (cm *ConfigManager) validateConfig(config *configuration.CNSConfig) error {
	// Validate that telemetry settings are properly configured
	if config.TelemetrySettings.AppInsightsInstrumentationKey == "" && !config.TelemetrySettings.DisableAll {
		logger.Printf("Warning: No Application Insights instrumentation key configured and telemetry not disabled")
	}

	// Validate batch size settings for Azure Application Insights
	if config.TelemetrySettings.TelemetryBatchSizeBytes <= 0 {
		logger.Printf("Warning: Invalid telemetry batch size, using default")
	}

	// Validate batch interval for optimal Azure ingestion
	if config.TelemetrySettings.TelemetryBatchIntervalInSecs <= 0 {
		logger.Printf("Warning: Invalid telemetry batch interval, using default")
	}

	// Log configuration summary for Azure monitoring and debugging
	logger.Printf("Azure CNS Configuration Summary:")
	logger.Printf("  - Telemetry DisableAll: %t", config.TelemetrySettings.DisableAll)
	logger.Printf("  - Application Insights Key Present: %t",
		config.TelemetrySettings.AppInsightsInstrumentationKey != "")
	logger.Printf("  - Batch Size: %d bytes", config.TelemetrySettings.TelemetryBatchSizeBytes)
	logger.Printf("  - Batch Interval: %d seconds", config.TelemetrySettings.TelemetryBatchIntervalInSecs)
	logger.Printf("  - Disable Trace: %t", config.TelemetrySettings.DisableTrace)
	logger.Printf("  - Disable Metric: %t", config.TelemetrySettings.DisableMetric)
	logger.Printf("  - Disable Event: %t", config.TelemetrySettings.DisableEvent)
	logger.Printf("  - Debug Mode: %t", config.TelemetrySettings.DebugMode)

	return nil
}
