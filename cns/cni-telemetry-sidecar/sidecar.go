package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Azure/azure-container-networking/aitelemetry"
	"github.com/Azure/azure-container-networking/cns/configuration"
	"github.com/Azure/azure-container-networking/telemetry"
	"go.uber.org/zap"
)

const (
	// Constants matching telemetrymain.go
	defaultReportToHostIntervalInSecs = 30
	defaultRefreshTimeoutInSecs       = 15
	defaultBatchSizeInBytes           = 16384
	defaultBatchIntervalInSecs        = 15
	defaultGetEnvRetryCount           = 2
	defaultGetEnvRetryWaitTimeInSecs  = 3
	pluginName                        = "AzureCNI"
	cniTelemetryVersion               = "1.0.0"
)

// TelemetrySidecar replaces the azure-vnet-telemetry binary fork process
type TelemetrySidecar struct {
	configPath      string
	configManager   *ConfigManager
	logger          *zap.Logger
	telemetryBuffer *telemetry.TelemetryBuffer
	builtInAIKey    string // AppInsights key embedded at build time (like azure-vnet-telemetry)
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
	s.configManager.SetLogger(logger)
	return nil
}

// SetBuiltInAIKey sets the build-time embedded AppInsights key
func (s *TelemetrySidecar) SetBuiltInAIKey(key string) {
	s.builtInAIKey = key
}

// Run starts the telemetry sidecar (replaces main() in telemetrymain.go)
func (s *TelemetrySidecar) Run(ctx context.Context) error {
	if s.logger == nil {
		return fmt.Errorf("logger not initialized - call SetLogger() first")
	}

	s.logger.Info("Starting Azure CNI Telemetry Sidecar (replacing azure-vnet-telemetry binary)")

	// Load CNS configuration
	cnsConfig, err := s.configManager.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load CNS configuration: %w", err)
	}

	// Check if telemetry should run
	if !s.shouldRunTelemetry(cnsConfig) {
		s.logger.Info("CNI Telemetry disabled, entering sleep mode")
		return s.sleepUntilShutdown(ctx)
	}

	// Convert CNS config to telemetry config (like telemetrymain.go does)
	telemetryConfig := s.convertToTelemetryConfig(cnsConfig)
	s.setTelemetryDefaults(&telemetryConfig)

	s.logger.Info("Telemetry configuration", zap.Any("config", telemetryConfig))

	// Initialize and start telemetry service with both configs (like telemetrymain.go does)
	if err := s.startTelemetryService(ctx, telemetryConfig, cnsConfig); err != nil {
		return fmt.Errorf("failed to start telemetry service: %w", err)
	}

	// Keep running until context is cancelled
	<-ctx.Done()
	return s.cleanup()
}

// convertToTelemetryConfig converts CNS config to telemetry config
func (s *TelemetrySidecar) convertToTelemetryConfig(cnsConfig *configuration.CNSConfig) telemetry.TelemetryConfig {
	ts := cnsConfig.TelemetrySettings

	return telemetry.TelemetryConfig{
		ReportToHostIntervalInSeconds: time.Duration(defaultReportToHostIntervalInSecs) * time.Second,
		DisableAll:                    ts.DisableAll,
		DisableTrace:                  ts.DisableTrace,
		DisableMetric:                 ts.DisableMetric,
		BatchSizeInBytes:              ts.TelemetryBatchSizeBytes,
		BatchIntervalInSecs:           ts.TelemetryBatchIntervalInSecs,
		RefreshTimeoutInSecs:          ts.RefreshIntervalInSecs,
		DisableMetadataThread:         ts.DisableMetadataRefreshThread,
		DebugMode:                     ts.DebugMode,
		GetEnvRetryCount:              defaultGetEnvRetryCount,
		GetEnvRetryWaitTimeInSecs:     defaultGetEnvRetryWaitTimeInSecs,
	}
}

// setTelemetryDefaults sets default values (same as telemetrymain.go)
func (s *TelemetrySidecar) setTelemetryDefaults(config *telemetry.TelemetryConfig) {
	if config.ReportToHostIntervalInSeconds == 0 {
		config.ReportToHostIntervalInSeconds = time.Duration(defaultReportToHostIntervalInSecs) * time.Second
	}

	if config.RefreshTimeoutInSecs == 0 {
		config.RefreshTimeoutInSecs = defaultRefreshTimeoutInSecs
	}

	if config.BatchIntervalInSecs == 0 {
		config.BatchIntervalInSecs = defaultBatchIntervalInSecs
	}

	if config.BatchSizeInBytes == 0 {
		config.BatchSizeInBytes = defaultBatchSizeInBytes
	}

	if config.GetEnvRetryCount == 0 {
		config.GetEnvRetryCount = defaultGetEnvRetryCount
	}

	if config.GetEnvRetryWaitTimeInSecs == 0 {
		config.GetEnvRetryWaitTimeInSecs = defaultGetEnvRetryWaitTimeInSecs
	}
}

// getAppInsightsKey gets the AppInsights key with priority: build-time > config > env vars
func (s *TelemetrySidecar) getAppInsightsKey(cnsConfig *configuration.CNSConfig) string {
	// Priority 1: Build-time embedded key via telemetry.aiMetadata (like azure-vnet-telemetry)
	if buildTimeKey := telemetry.GetAIMetadata(); buildTimeKey != "" {
		s.logger.Debug("Using build-time embedded AppInsights key")
		return buildTimeKey
	}

	// Priority 2: CNS configuration
	if cnsConfig != nil && cnsConfig.TelemetrySettings.AppInsightsInstrumentationKey != "" {
		s.logger.Debug("Using AppInsights key from CNS configuration")
		return cnsConfig.TelemetrySettings.AppInsightsInstrumentationKey
	}

	// Priority 3: Environment variables (fallback)
	envKeys := []string{
		"APPINSIGHTS_INSTRUMENTATIONKEY",
		"APPLICATIONINSIGHTS_CONNECTION_STRING",
		"AI_INSTRUMENTATION_KEY",
	}

	for _, envKey := range envKeys {
		if envKey := os.Getenv(envKey); envKey != "" {
			s.logger.Debug("Using AppInsights key from environment variable", zap.String("envVar", envKey))
			return envKey
		}
	}

	// Only log warning if no key found from any source
	s.logger.Warn("No AppInsights instrumentation key found from any source (build-time, config, or environment)")
	return ""
}

// startTelemetryService starts the telemetry service (replicates telemetrymain.go logic)
func (s *TelemetrySidecar) startTelemetryService(ctx context.Context, config telemetry.TelemetryConfig, cnsConfig *configuration.CNSConfig) error {
	s.logger.Info("Initializing telemetry service")

	// Get AppInsights key with priority order (build-time aiMetadata has highest priority)
	aiKey := s.getAppInsightsKey(cnsConfig)

	// DEBUG: Only show detailed debug info in debug mode
	if s.logger.Level() == zap.DebugLevel {
		currentAIMetadata := telemetry.GetAIMetadata()
		s.logger.Debug("AI telemetry status",
			zap.String("buildTimeAIMetadata", maskAIKey(currentAIMetadata)),
			zap.String("resolvedAIKey", maskAIKey(aiKey)),
			zap.Bool("aiMetadataSet", currentAIMetadata != ""))
	}

	if aiKey != "" {
		// Set environment variable for compatibility with other telemetry components
		os.Setenv("APPINSIGHTS_INSTRUMENTATIONKEY", aiKey)

		// If aiMetadata wasn't set at build time, set it now for runtime scenarios
		if currentAIMetadata := telemetry.GetAIMetadata(); currentAIMetadata == "" {
			telemetry.SetAIMetadata(aiKey)
			s.logger.Debug("Set aiMetadata at runtime")
		}
	}

	// Clean up any orphan socket (same as telemetrymain.go)
	tbtemp := telemetry.NewTelemetryBuffer(s.logger)
	tbtemp.Cleanup(telemetry.FdName)

	// Create telemetry buffer (same as telemetrymain.go)
	s.telemetryBuffer = telemetry.NewTelemetryBuffer(s.logger)

	// Start telemetry server (same as telemetrymain.go)
	for {
		s.logger.Info("Starting telemetry server")
		err := s.telemetryBuffer.StartServer()
		if err == nil || s.telemetryBuffer.FdExists {
			break
		}

		s.logger.Error("Telemetry service starting failed", zap.Error(err))
		s.telemetryBuffer.Cleanup(telemetry.FdName)
		time.Sleep(time.Millisecond * 200)
	}

	// Only create AI telemetry handle if we have an AI key or aiMetadata is set
	finalAIMetadata := telemetry.GetAIMetadata()
	if finalAIMetadata != "" {
		// Configure AI settings (same as telemetrymain.go)
		aiConfig := aitelemetry.AIConfig{
			AppName:                      pluginName,
			AppVersion:                   cniTelemetryVersion,
			BatchSize:                    config.BatchSizeInBytes,
			BatchInterval:                config.BatchIntervalInSecs,
			RefreshTimeout:               config.RefreshTimeoutInSecs,
			DisableMetadataRefreshThread: config.DisableMetadataThread,
			DebugMode:                    config.DebugMode,
			GetEnvRetryCount:             config.GetEnvRetryCount,
			GetEnvRetryWaitTimeInSecs:    config.GetEnvRetryWaitTimeInSecs,
		}

		s.logger.Info("Initializing Azure Application Insights telemetry")

		// Create AI telemetry handle (same as telemetrymain.go)
		if err := s.telemetryBuffer.CreateAITelemetryHandle(aiConfig, config.DisableAll, config.DisableTrace, config.DisableMetric); err != nil {
			s.logger.Error("Failed to initialize Azure Application Insights", zap.Error(err))
			s.logger.Info("Continuing with local telemetry only")
		} else {
			s.logger.Info("Azure Application Insights telemetry initialized successfully")
		}
	} else {
		s.logger.Info("Running with local telemetry only (no Azure Application Insights key)")
	}

	s.logger.Info("Telemetry service started successfully",
		zap.Duration("reportInterval", config.ReportToHostIntervalInSeconds),
		zap.String("pluginName", pluginName),
		zap.String("version", cniTelemetryVersion),
		zap.Bool("azureIntegration", finalAIMetadata != ""))

	// Start the data push routine in background (same as telemetrymain.go)
	go s.telemetryBuffer.PushData(ctx)

	return nil
}

// Helper function to mask AI key for logging
func maskAIKey(aiKey string) string {
	if len(aiKey) <= 8 {
		return aiKey
	}
	return aiKey[:8] + "..."
}

// shouldRunTelemetry determines if CNI telemetry should be enabled
func (s *TelemetrySidecar) shouldRunTelemetry(cnsConfig *configuration.CNSConfig) bool {
	// Check if telemetry is disabled globally
	if cnsConfig.TelemetrySettings.DisableAll {
		s.logger.Info("Telemetry disabled via CNS configuration")
		return false
	}

	// Check if CNI telemetry is specifically enabled
	if !cnsConfig.TelemetrySettings.EnableCNITelemetry {
		s.logger.Info("CNI Telemetry disabled via CNS configuration")
		return false
	}

	// Check if we have an AI key from any source
	aiKey := s.getAppInsightsKey(cnsConfig)
	hasAIKey := aiKey != ""

	if hasAIKey {
		s.logger.Info("CNI Telemetry enabled with AppInsights integration",
			zap.Bool("enableCNITelemetry", cnsConfig.TelemetrySettings.EnableCNITelemetry),
			zap.Bool("hasAppInsightsKey", true))
	} else {
		s.logger.Info("CNI Telemetry enabled with local-only mode",
			zap.Bool("enableCNITelemetry", cnsConfig.TelemetrySettings.EnableCNITelemetry),
			zap.Bool("hasAppInsightsKey", false))
	}

	return true
}

// cleanup handles graceful shutdown (like telemetrymain.go)
func (s *TelemetrySidecar) cleanup() error {
	s.logger.Info("Shutting down CNI Telemetry service")

	if s.telemetryBuffer != nil {
		// Close AI telemetry handle (same as telemetrymain.go)
		telemetry.CloseAITelemetryHandle()

		// Cleanup socket and resources
		s.telemetryBuffer.Cleanup(telemetry.FdName)
		s.logger.Info("Telemetry service cleaned up successfully")
	}

	return nil
}

// sleepUntilShutdown keeps the container running when telemetry is disabled
func (s *TelemetrySidecar) sleepUntilShutdown(ctx context.Context) error {
	s.logger.Info("CNI Telemetry sidecar sleeping until shutdown signal received")
	<-ctx.Done()
	return ctx.Err()
}
