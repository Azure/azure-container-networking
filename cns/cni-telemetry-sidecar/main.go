// Copyright Microsoft. All rights reserved.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Azure/azure-container-networking/telemetry"
	"go.uber.org/zap"
)

var (
	configPath = flag.String("config", "", "Path to CNS configuration file")
	logLevel   = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	version    = "1.0.0" // Set at build time via -ldflags
)

func main() {
	flag.Parse()
	os.Exit(run())
}

func run() int {
	logger, err := initializeLogger(*logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize logger: %v\n", err)
		return 1
	}
	defer logger.Sync() //nolint:errcheck // best effort

	configManager := NewConfigManager(*configPath)
	configManager.SetLogger(logger)

	logger.Info("Starting Azure CNI Telemetry Sidecar",
		zap.String("version", version),
		zap.String("configPath", configManager.GetConfigPath()),
		zap.Bool("hasBuiltInAIKey", telemetry.GetAIMetadata() != ""))

	sidecar := NewTelemetrySidecar(configManager)
	sidecar.SetLogger(logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		logger.Info("Received shutdown signal", zap.String("signal", sig.String()))
		cancel()
	}()

	if err := sidecar.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("Sidecar execution failed", zap.Error(err))
		return 1
	}

	logger.Info("Shutdown complete")
	return 0
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

	logger, err := config.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build logger: %w", err)
	}
	return logger, nil
}
