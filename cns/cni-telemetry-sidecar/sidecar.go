// Copyright Microsoft. All rights reserved.
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
	// Sidecar-specific telemetry defaults - these may differ from CNS defaults
	// to optimize for CNI telemetry workloads (smaller, more frequent batches)
	defaultReportToHostIntervalInSecs = 30
	defaultRefreshTimeoutInSecs       = 15
	defaultBatchSizeInBytes           = 16384
	defaultBatchIntervalInSecs        = 15
	defaultGetEnvRetryCount           = 2
	defaultGetEnvRetryWaitTimeInSecs  = 3
	pluginName                        = "AzureCNI"
	maxServerStartRetries             = 10
)

// TelemetrySidecar implements the CNI telemetry service as a sidecar container,
// replacing the azure-vnet-telemetry binary fork process.
type TelemetrySidecar struct {
	configManager   *ConfigManager
	logger          *zap.Logger
	version         string
	telemetryBuffer *telemetry.TelemetryBuffer
}

// NewTelemetrySidecar creates a new TelemetrySidecar instance.
func NewTelemetrySidecar(configManager *ConfigManager, logger *zap.Logger, version string) *TelemetrySidecar {
	return &TelemetrySidecar{
		configManager: configManager,
		logger:        logger,
		version:       version,
	}
}

// Run starts the telemetry sidecar service.
func (s *TelemetrySidecar) Run(ctx context.Context) error {
	cnsConfig := s.configManager.LoadConfig()

	if !s.shouldRunTelemetry(cnsConfig) {
		s.logger.Info("CNI Telemetry disabled, entering idle mode")
		<-ctx.Done()
		return fmt.Errorf("CNI Telemetry disabled: %w", ctx.Err())
	}

	telemetryConfig := s.buildTelemetryConfig(cnsConfig)

	if err := s.startTelemetryService(ctx, telemetryConfig, cnsConfig); err != nil {
		return fmt.Errorf("failed to start telemetry service: %w", err)
	}

	<-ctx.Done()
	return s.cleanup()
}

func (s *TelemetrySidecar) buildTelemetryConfig(cnsConfig *configuration.CNSConfig) telemetry.TelemetryConfig {
	ts := cnsConfig.TelemetrySettings

	batchSize := ts.TelemetryBatchSizeBytes
	if batchSize == 0 {
		batchSize = defaultBatchSizeInBytes
	}
	batchInterval := ts.TelemetryBatchIntervalInSecs
	if batchInterval == 0 {
		batchInterval = defaultBatchIntervalInSecs
	}
	refreshTimeout := ts.RefreshIntervalInSecs
	if refreshTimeout == 0 {
		refreshTimeout = defaultRefreshTimeoutInSecs
	}

	return telemetry.TelemetryConfig{
		ReportToHostIntervalInSeconds: time.Duration(defaultReportToHostIntervalInSecs) * time.Second,
		DisableAll:                    ts.DisableAll,
		DisableTrace:                  ts.DisableTrace,
		DisableMetric:                 ts.DisableMetric,
		BatchSizeInBytes:              batchSize,
		BatchIntervalInSecs:           batchInterval,
		RefreshTimeoutInSecs:          refreshTimeout,
		DisableMetadataThread:         ts.DisableMetadataRefreshThread,
		DebugMode:                     ts.DebugMode,
		GetEnvRetryCount:              defaultGetEnvRetryCount,
		GetEnvRetryWaitTimeInSecs:     defaultGetEnvRetryWaitTimeInSecs,
	}
}

func (s *TelemetrySidecar) startTelemetryService(ctx context.Context, config telemetry.TelemetryConfig, cnsConfig *configuration.CNSConfig) error {
	// Set AI key from config or env var if not already set at build time
	if telemetry.GetAIMetadata() == "" {
		if key := cnsConfig.TelemetrySettings.AppInsightsInstrumentationKey; key != "" {
			telemetry.SetAIMetadata(key)
		} else if key := os.Getenv(appInsightsEnvVar); key != "" {
			telemetry.SetAIMetadata(key)
		}
	}

	// Clean up any orphan socket
	err := telemetry.NewTelemetryBuffer(s.logger).Cleanup(telemetry.FdName)
	if err != nil {
		s.logger.Warn("Failed to clean up orphan socket", zap.Error(err))
	}

	s.telemetryBuffer = telemetry.NewTelemetryBuffer(s.logger)

	// Retry starting server with bounded retries and context cancellation
	for attempt := 0; attempt < maxServerStartRetries; attempt++ {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled during server start: %w", ctx.Err())
		default:
		}

		err := s.telemetryBuffer.StartServer()
		if err == nil || s.telemetryBuffer.FdExists {
			break
		}

		s.logger.Error("Telemetry server start failed, retrying",
			zap.Error(err),
			zap.Int("attempt", attempt+1),
			zap.Int("maxRetries", maxServerStartRetries))

		if errL := s.telemetryBuffer.Cleanup(telemetry.FdName); errL != nil {
			s.logger.Warn("Failed to clean up orphan socket during retry", zap.Error(errL))
		}

		if attempt == maxServerStartRetries-1 {
			return fmt.Errorf("failed to start telemetry server after %d attempts: %w", maxServerStartRetries, err)
		}

		time.Sleep(200 * time.Millisecond)
	}

	if telemetry.GetAIMetadata() != "" {
		aiConfig := aitelemetry.AIConfig{
			AppName:                      pluginName,
			AppVersion:                   s.version,
			BatchSize:                    config.BatchSizeInBytes,
			BatchInterval:                config.BatchIntervalInSecs,
			RefreshTimeout:               config.RefreshTimeoutInSecs,
			DisableMetadataRefreshThread: config.DisableMetadataThread,
			DebugMode:                    config.DebugMode,
			GetEnvRetryCount:             config.GetEnvRetryCount,
			GetEnvRetryWaitTimeInSecs:    config.GetEnvRetryWaitTimeInSecs,
		}
		if err := s.telemetryBuffer.CreateAITelemetryHandle(aiConfig, config.DisableAll, config.DisableTrace, config.DisableMetric); err != nil {
			s.logger.Warn("AppInsights initialization failed, continuing without it", zap.Error(err))
		}
	}

	s.logger.Info("Telemetry service started",
		zap.Bool("appInsightsEnabled", telemetry.GetAIMetadata() != ""))

	go s.telemetryBuffer.PushData(ctx)
	return nil
}

func (s *TelemetrySidecar) shouldRunTelemetry(cnsConfig *configuration.CNSConfig) bool {
	if cnsConfig.TelemetrySettings.DisableAll {
		s.logger.Info("Telemetry disabled globally")
		return false
	}
	sidecarConfig := s.configManager.GetSidecarConfig()
	if sidecarConfig == nil || !sidecarConfig.TelemetrySettings.EnableCNITelemetry {
		s.logger.Info("CNI telemetry not enabled")
		return false
	}
	return true
}

func (s *TelemetrySidecar) cleanup() error {
	s.logger.Info("Shutting down telemetry service")
	if s.telemetryBuffer != nil {
		telemetry.CloseAITelemetryHandle()
		err := s.telemetryBuffer.Cleanup(telemetry.FdName)
		if err != nil {
			s.logger.Warn("Failed to clean up orphan socket during shutdown", zap.Error(err))
		}
	}
	return nil
}
