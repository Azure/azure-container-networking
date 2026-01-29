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
	defaultReportToHostIntervalInSecs = 30
	defaultRefreshTimeoutInSecs       = 15
	defaultBatchSizeInBytes           = 16384
	defaultBatchIntervalInSecs        = 15
	defaultGetEnvRetryCount           = 2
	defaultGetEnvRetryWaitTimeInSecs  = 3
	pluginName                        = "AzureCNI"
	cniTelemetryVersion               = "1.0.0"
)

// TelemetrySidecar implements the CNI telemetry service as a sidecar container,
// replacing the azure-vnet-telemetry binary fork process.
type TelemetrySidecar struct {
	configManager   *ConfigManager
	logger          *zap.Logger
	telemetryBuffer *telemetry.TelemetryBuffer
}

// NewTelemetrySidecar creates a new TelemetrySidecar instance.
func NewTelemetrySidecar(configManager *ConfigManager) *TelemetrySidecar {
	return &TelemetrySidecar{
		configManager: configManager,
	}
}

// SetLogger sets the logger for the sidecar.
func (s *TelemetrySidecar) SetLogger(logger *zap.Logger) {
	s.logger = logger
	s.configManager.SetLogger(logger)
}

// Run starts the telemetry sidecar service.
func (s *TelemetrySidecar) Run(ctx context.Context) error {
	cnsConfig, err := s.configManager.LoadConfig()
	if err != nil {
		return err
	}

	if !s.shouldRunTelemetry(cnsConfig) {
		s.logger.Info("CNI Telemetry disabled, entering idle mode")
		<-ctx.Done()
		return fmt.Errorf("CNI Telemetry disabled: %w", ctx.Err())
	}

	telemetryConfig := s.buildTelemetryConfig(cnsConfig)

	if err := s.startTelemetryService(ctx, telemetryConfig, cnsConfig); err != nil {
		return err
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

// getAppInsightsKey returns the AppInsights key with priority: build-time > config > env vars.
func (s *TelemetrySidecar) getAppInsightsKey(cnsConfig *configuration.CNSConfig) string {
	if key := telemetry.GetAIMetadata(); key != "" {
		return key
	}
	if cnsConfig != nil && cnsConfig.TelemetrySettings.AppInsightsInstrumentationKey != "" {
		return cnsConfig.TelemetrySettings.AppInsightsInstrumentationKey
	}
	for _, env := range appInsightsEnvVars {
		if key := os.Getenv(env); key != "" {
			return key
		}
	}
	return ""
}

func (s *TelemetrySidecar) startTelemetryService(ctx context.Context, config telemetry.TelemetryConfig, cnsConfig *configuration.CNSConfig) error {
	aiKey := s.getAppInsightsKey(cnsConfig)
	if aiKey != "" {
		os.Setenv("APPINSIGHTS_INSTRUMENTATIONKEY", aiKey)
		if telemetry.GetAIMetadata() == "" {
			telemetry.SetAIMetadata(aiKey)
		}
	}

	// Clean up any orphan socket
	err := telemetry.NewTelemetryBuffer(s.logger).Cleanup(telemetry.FdName)
	if err != nil {
		s.logger.Warn("Failed to clean up orphan socket", zap.Error(err))
	}

	s.telemetryBuffer = telemetry.NewTelemetryBuffer(s.logger)

	// Retry starting server until successful
	for {
		err := s.telemetryBuffer.StartServer()
		if err == nil || s.telemetryBuffer.FdExists {
			break
		}
		s.logger.Error("Telemetry server start failed, retrying", zap.Error(err))
		errL := s.telemetryBuffer.Cleanup(telemetry.FdName)
		if errL != nil {
			s.logger.Warn("Failed to clean up orphan socket during retry", zap.Error(errL))
		}
		time.Sleep(200 * time.Millisecond)
	}

	if telemetry.GetAIMetadata() != "" {
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
	if !cnsConfig.TelemetrySettings.EnableCNITelemetry {
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
