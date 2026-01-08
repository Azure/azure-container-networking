package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/Azure/azure-container-networking/telemetry"
	"go.uber.org/zap"
)

var (
	configPath = flag.String("config", "/etc/azure-cns/cns_config.json", "Path to CNS configuration file")
	logLevel   = flag.String("log-level", "info", "Log level (debug, info, warn, error)")

	// This variable is set at build time via ldflags from Makefile
	version = "1.0.0" // -X main.version=$(CNI_TELEMETRY_SIDECAR_VERSION)
)

func main() {
	flag.Parse()

	// Initialize logger
	logger, err := initializeLogger(*logLevel)
	if err != nil {
		panic(err)
	}
	defer logger.Sync()

	// DEBUG: Check if aiMetadata was set at build time via ldflags
	currentAIMetadata := telemetry.GetAIMetadata()
	logger.Info("Starting Azure CNI Telemetry Sidecar",
		zap.String("version", version),
		zap.String("configPath", *configPath),
		zap.String("logLevel", *logLevel),
		zap.Bool("hasBuiltInAIKey", currentAIMetadata != ""),
		zap.String("aiKeyPrefix", MaskAIKey(currentAIMetadata)))

	// Create and configure telemetry sidecar
	// Pass the configPath to NewTelemetrySidecar (it expects a string parameter)
	sidecar := NewTelemetrySidecar(*configPath)
	if err := sidecar.SetLogger(logger); err != nil {
		logger.Error("Failed to set logger", zap.Error(err))
		os.Exit(1)
	}

	// Log which AI key source we're using
	if currentAIMetadata != "" {
		logger.Info("Using build-time embedded AppInsights key (from Makefile)")
	} else {
		logger.Info("No build-time AppInsights key found - will check config/environment")
	}

	// Create context with cancellation for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		logger.Info("Received shutdown signal", zap.String("signal", sig.String()))
		cancel()
	}()

	// Run the sidecar
	if err := sidecar.Run(ctx); err != nil && err != context.Canceled {
		logger.Error("Sidecar execution failed", zap.Error(err))
		os.Exit(1)
	}

	logger.Info("Azure CNI Telemetry Sidecar shutdown complete")
}

// initializeLogger creates a zap logger with the specified level
func initializeLogger(level string) (*zap.Logger, error) {
	var zapLevel zap.AtomicLevel
	switch level {
	case "debug":
		zapLevel = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		zapLevel = zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn":
		zapLevel = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		zapLevel = zap.NewAtomicLevelAt(zap.ErrorLevel)
	default:
		zapLevel = zap.NewAtomicLevelAt(zap.InfoLevel)
	}

	config := zap.NewProductionConfig()
	config.Level = zapLevel
	config.DisableStacktrace = true
	config.DisableCaller = false

	return config.Build()
}
