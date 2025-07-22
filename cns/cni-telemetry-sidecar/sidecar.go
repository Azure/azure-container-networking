package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Azure/azure-container-networking/aitelemetry"
	"github.com/Azure/azure-container-networking/cns/configuration"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/telemetry"
)

const (
	// CNI telemetry constants aligned with azure-vnet-telemetry
	cniTelemetryAppName = "azure-vnet-telemetry"
	cniTelemetryVersion = "1.0.0"
	telemetrySocket     = "/var/run/azure-vnet-telemetry.sock"
)

// TelemetrySidecar manages the lifecycle of the CNI telemetry service
type TelemetrySidecar struct {
	configPath      string
	configManager   *ConfigManager
	telemetryBuffer *telemetry.TelemetryBuffer
}

// NewTelemetrySidecar creates a new telemetry sidecar instance
func NewTelemetrySidecar(configPath string) *TelemetrySidecar {
	return &TelemetrySidecar{
		configPath:    configPath,
		configManager: NewConfigManager(configPath),
	}
}

// Run starts the telemetry sidecar and manages its lifecycle
func (s *TelemetrySidecar) Run(ctx context.Context) error {
	logger.Printf("Initializing Azure CNI Telemetry Sidecar for azure-vnet-telemetry")

	// Load CNS configuration from shared mount
	config, err := s.configManager.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load CNS configuration: %w", err)
	}

	// Determine if telemetry should run based on configuration and environment
	if !s.shouldRunTelemetry(config) {
		logger.Printf("CNI Telemetry disabled, entering sleep mode")
		return s.sleepUntilShutdown(ctx)
	}

	// Initialize CNI telemetry using existing Azure packages
	if err := s.initializeCNITelemetry(config); err != nil {
		return fmt.Errorf("failed to initialize CNI telemetry: %w", err)
	}

	// Start telemetry service mimicking azure-vnet-telemetry behavior
	logger.Printf("Starting Azure VNet Telemetry service (CNI mode)")
	return s.runCNITelemetryService(ctx)
}

// initializeCNITelemetry sets up telemetry specifically for CNI operations
func (s *TelemetrySidecar) initializeCNITelemetry(config *configuration.CNSConfig) error {
	ts := config.TelemetrySettings

	// Create AI configuration matching azure-vnet-telemetry behavior
	aiConfig := aitelemetry.AIConfig{
		AppName:                      cniTelemetryAppName,
		AppVersion:                   cniTelemetryVersion,
		BatchSize:                    ts.TelemetryBatchSizeBytes,
		BatchInterval:                ts.TelemetryBatchIntervalInSecs,
		RefreshTimeout:               ts.RefreshIntervalInSecs,
		DisableMetadataRefreshThread: ts.DisableMetadataRefreshThread,
		DebugMode:                    ts.DebugMode,
	}

	// Create telemetry buffer for CNI-specific data collection
	s.telemetryBuffer = telemetry.NewTelemetryBuffer(nil)

	// Validate Application Insights instrumentation key
	if config.TelemetrySettings.AppInsightsInstrumentationKey == "" {
		return fmt.Errorf("Application Insights instrumentation key is required for CNI telemetry")
	}

	// Initialize AI telemetry handle with Azure-specific configuration
	err := s.telemetryBuffer.CreateAITelemetryHandle(
		aiConfig,
		ts.DisableTrace,
		ts.DisableMetric,
		ts.DisableEvent,
	)
	if err != nil {
		return fmt.Errorf("failed to create AI telemetry handle for CNI: %w", err)
	}

	logger.Printf("CNI Telemetry initialized with Application Insights (App: %s, Version: %s, BatchSize: %d)",
		cniTelemetryAppName, cniTelemetryVersion, ts.TelemetryBatchSizeBytes)
	return nil
}

// runCNITelemetryService runs the telemetry service mimicking azure-vnet-telemetry
func (s *TelemetrySidecar) runCNITelemetryService(ctx context.Context) error {
	// Cleanup any existing CNI telemetry instances
	s.cleanupExistingInstances()

	// Start telemetry server on the expected socket for CNI integration
	if err := s.telemetryBuffer.StartServer(); err != nil {
		return fmt.Errorf("failed to start CNI telemetry server: %w", err)
	}

	logger.Printf("Azure VNet Telemetry server started successfully on socket: %s", telemetrySocket)

	// Start telemetry data collection in background (non-blocking)
	go s.telemetryBuffer.PushData(ctx)

	// Log readiness for CNI network event collection
	logger.Printf("CNI Telemetry sidecar ready to collect Azure network interface events")

	// Wait for context cancellation (graceful shutdown signal)
	<-ctx.Done()

	// Perform cleanup and graceful shutdown
	logger.Printf("Shutting down Azure CNI Telemetry service")
	return s.shutdownTelemetry()
}

// cleanupExistingInstances cleans up any leftover telemetry instances
func (s *TelemetrySidecar) cleanupExistingInstances() {
	// Create temporary buffer for cleanup operations
	tempBuffer := telemetry.NewTelemetryBuffer(nil)

	// Use the same FdName that azure-vnet-telemetry uses for consistency
	if err := tempBuffer.Cleanup(telemetry.FdName); err != nil {
		logger.Printf("Warning: Failed to cleanup existing CNI telemetry instances: %v", err)
	} else {
		logger.Printf("Successfully cleaned up existing CNI telemetry instances")
	}
}

// shutdownTelemetry handles graceful shutdown of telemetry resources
func (s *TelemetrySidecar) shutdownTelemetry() error {
	if s.telemetryBuffer != nil {
		// Close telemetry buffer (ensures data is flushed to Azure)
		s.telemetryBuffer.Close()
		logger.Printf("CNI Telemetry buffer closed and remaining data flushed to Azure")
	}
	return nil
}

// shouldRunTelemetry determines if CNI telemetry should be enabled
func (s *TelemetrySidecar) shouldRunTelemetry(config *configuration.CNSConfig) bool {
	// Check global telemetry disable flag in CNS configuration
	if config.TelemetrySettings.DisableAll {
		logger.Printf("CNI Telemetry disabled globally in CNS configuration")
		return false
	}

	// Check CNI telemetry specific enable flag (replaces old "ts" option)
	cniTelemetryEnabled := os.Getenv("CNI_TELEMETRY_ENABLED")
	if cniTelemetryEnabled != "true" {
		logger.Printf("CNI Telemetry not enabled via CNI_TELEMETRY_ENABLED environment variable")
		return false
	}

	// Validate Application Insights key availability
	if config.TelemetrySettings.AppInsightsInstrumentationKey == "" {
		logger.Printf("No Application Insights instrumentation key configured for CNI telemetry")
		return false
	}

	logger.Printf("CNI Telemetry enabled - will collect Azure network interface events")
	return true
}

// sleepUntilShutdown keeps the container running when telemetry is disabled
func (s *TelemetrySidecar) sleepUntilShutdown(ctx context.Context) error {
	logger.Printf("CNI Telemetry sidecar sleeping until shutdown signal received")
	<-ctx.Done()
	return ctx.Err()
}
