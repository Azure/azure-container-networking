package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/Azure/azure-container-networking/cns/configuration"
	"go.uber.org/zap"
)

const (
	// CNI telemetry constants
	cniTelemetryAppName = "azure-vnet-telemetry"
	cniTelemetryVersion = "1.0.0"
	telemetrySocketPath = "/var/run/azure-vnet-telemetry.sock" // Socket path that azure-vnet expects
)

// TelemetrySidecar manages the lifecycle of the CNI telemetry service
type TelemetrySidecar struct {
	configPath     string
	configManager  *ConfigManager
	logger         *zap.Logger
	socketListener net.Listener
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

	// Create the telemetry socket that azure-vnet CNI expects
	if err := s.createTelemetrySocket(); err != nil {
		return fmt.Errorf("failed to create telemetry socket: %w", err)
	}
	defer s.cleanupSocket()

	s.logger.Info("Starting Azure CNI Telemetry collection with socket server")
	return s.runTelemetryService(ctx)
}

// createTelemetrySocket creates the Unix socket that azure-vnet CNI connects to
func (s *TelemetrySidecar) createTelemetrySocket() error {
	// Remove any existing socket file
	if err := os.RemoveAll(telemetrySocketPath); err != nil {
		s.logger.Warn("Failed to remove existing socket file", zap.Error(err))
	}

	// Create the directory if it doesn't exist
	if err := os.MkdirAll("/var/run", 0755); err != nil {
		return fmt.Errorf("failed to create /var/run directory: %w", err)
	}

	// Create Unix socket listener
	listener, err := net.Listen("unix", telemetrySocketPath)
	if err != nil {
		return fmt.Errorf("failed to create Unix socket at %s: %w", telemetrySocketPath, err)
	}

	s.socketListener = listener
	s.logger.Info("Created telemetry socket", zap.String("path", telemetrySocketPath))

	// Set socket permissions so azure-vnet can access it
	if err := os.Chmod(telemetrySocketPath, 0666); err != nil {
		s.logger.Warn("Failed to set socket permissions", zap.Error(err))
	}

	return nil
}

// runTelemetryService runs both the socket server and telemetry collection
func (s *TelemetrySidecar) runTelemetryService(ctx context.Context) error {
	// Start socket server in background
	go s.handleSocketConnections(ctx)

	// Start telemetry collection loop
	return s.runTelemetryLoop(ctx)
}

// handleSocketConnections handles incoming connections from azure-vnet CNI
func (s *TelemetrySidecar) handleSocketConnections(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Accept connection with timeout
			if conn, err := s.socketListener.Accept(); err == nil {
				go s.handleConnection(conn)
			}
		}
	}
}

// handleConnection handles a single connection from azure-vnet CNI
func (s *TelemetrySidecar) handleConnection(conn net.Conn) {
	defer conn.Close()

	s.logger.Debug("Azure CNI telemetry connection established")

	// Read telemetry data from azure-vnet CNI
	buffer := make([]byte, 4096)
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			s.logger.Debug("Connection closed", zap.Error(err))
			break
		}

		if n > 0 {
			// Process telemetry data received from azure-vnet
			s.processTelemetryData(buffer[:n])
		}
	}
}

// processTelemetryData processes telemetry data received from azure-vnet CNI
func (s *TelemetrySidecar) processTelemetryData(data []byte) {
	s.logger.Debug("Received CNI telemetry data",
		zap.Int("bytes", len(data)),
		zap.String("data", string(data)))

	// TODO: Parse and process the actual telemetry data
	// This could include:
	// - JSON parsing of CNI events
	// - Metrics extraction
	// - Forwarding to Azure Monitor/Application Insights
}

// runTelemetryLoop runs the main telemetry collection loop
func (s *TelemetrySidecar) runTelemetryLoop(ctx context.Context) error {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	s.logger.Info("CNI Telemetry collection started with socket server")

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

// cleanupSocket removes the telemetry socket file
func (s *TelemetrySidecar) cleanupSocket() {
	if s.socketListener != nil {
		s.socketListener.Close()
	}
	if err := os.RemoveAll(telemetrySocketPath); err != nil {
		s.logger.Warn("Failed to cleanup socket file", zap.Error(err))
	} else {
		s.logger.Info("Telemetry socket cleaned up")
	}
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
