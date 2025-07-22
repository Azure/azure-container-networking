package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Azure/azure-container-networking/cns/configuration"
	"go.uber.org/zap"
)

const (
	// CNI telemetry constants
	cniTelemetryAppName = "azure-vnet-telemetry"
	cniTelemetryVersion = "1.0.0"
)

// TelemetrySidecar manages the lifecycle of the CNI telemetry service
type TelemetrySidecar struct {
	configPath    string
	configManager *ConfigManager
	logger        *zap.Logger
}

// NewTelemetrySidecar creates a new telemetry sidecar instance
func NewTelemetrySidecar(configPath string) *TelemetrySidecar {
	return &TelemetrySidecar{
		configPath:    configPath,
		configManager: NewConfigManager(configPath),
	}
}

// SetLogger sets the zap logger for the sidecar
func (s *TelemetrySidecar) SetLogger(logger *zap.Logger) error {
	if logger == nil {
		return fmt.Errorf("logger cannot be nil")
	}
	s.logger = logger

	// Also set the logger for the config manager
	s.configManager.SetLogger(logger)

	return nil
}

// Run starts the telemetry sidecar and manages its lifecycle
func (s *TelemetrySidecar) Run(ctx context.Context) error {
	if s.logger == nil {
		return fmt.Errorf("logger not initialized - call SetLogger() first")
	}

	s.logger.Info("Initializing Azure CNI Telemetry Sidecar")

	// Load CNS configuration from shared mount
	config, err := s.configManager.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load CNS configuration: %w", err)
	}

	// Determine if telemetry should run based on configuration and environment
	if !s.shouldRunTelemetry(config) {
		s.logger.Info("CNI Telemetry disabled, entering sleep mode")
		return s.sleepUntilShutdown(ctx)
	}

	s.logger.Info("Starting Azure CNI Telemetry collection")
	return s.runTelemetryLoop(ctx)
}

// runTelemetryLoop runs the main telemetry collection loop
func (s *TelemetrySidecar) runTelemetryLoop(ctx context.Context) error {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	s.logger.Info("CNI Telemetry collection started")

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Shutting down Azure CNI Telemetry service")
			return nil
		case <-ticker.C:
			s.collectTelemetry()
		}
	}
}

// collectTelemetry performs telemetry collection
func (s *TelemetrySidecar) collectTelemetry() {
	s.logger.Debug("Collecting CNI telemetry data")
	// TODO: Implement actual telemetry collection logic here
	// This could include:
	// - Reading CNI metrics
	// - Collecting network statistics
	// - Gathering Azure CNI specific data
}

// shouldRunTelemetry determines if CNI telemetry should be enabled
func (s *TelemetrySidecar) shouldRunTelemetry(config *configuration.CNSConfig) bool {
	// Check global telemetry disable flag in CNS configuration
	if config.TelemetrySettings.DisableAll {
		s.logger.Info("CNI Telemetry disabled globally in CNS configuration")
		return false
	}

	// Check CNI telemetry specific enable flag
	cniTelemetryEnabled := os.Getenv("CNI_TELEMETRY_ENABLED")
	if cniTelemetryEnabled != "true" {
		s.logger.Info("CNI Telemetry not enabled via CNI_TELEMETRY_ENABLED environment variable")
		return false
	}

	s.logger.Info("CNI Telemetry enabled - will collect Azure network interface events")
	return true
}

// sleepUntilShutdown keeps the container running when telemetry is disabled
func (s *TelemetrySidecar) sleepUntilShutdown(ctx context.Context) error {
	s.logger.Info("CNI Telemetry sidecar sleeping until shutdown signal received")
	<-ctx.Done()
	return ctx.Err()
}
