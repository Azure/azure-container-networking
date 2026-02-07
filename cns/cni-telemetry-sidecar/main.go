// Copyright Microsoft. All rights reserved.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Azure/azure-container-networking/telemetry"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var version = "1.0.0" // Set at build time via -ldflags

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var configPath string
	var logLevel string

	cmd := &cobra.Command{
		Use:   "azure-cni-telemetry-sidecar",
		Short: "Azure CNI Telemetry Sidecar",
		Long:  "Collects CNI telemetry from the unix socket and sends it to Application Insights",
		RunE: func(_ *cobra.Command, _ []string) error {
			return run(configPath, logLevel)
		},
	}

	// Use StringVarP to support both --config and -c shorthand
	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to CNS configuration file")
	cmd.Flags().StringVarP(&logLevel, "log-level", "l", "info", "Log level (debug, info, warn, error)")

	return cmd
}

func run(configPath, logLevel string) error {
	// Set up signal handling first, before any initialization that could hang
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		fmt.Fprintf(os.Stderr, "Received shutdown signal: %s\n", sig.String())
		cancel()
	}()

	logger, err := initializeLogger(logLevel)
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	defer logger.Sync() //nolint:errcheck // best effort

	configManager := NewConfigManager(configPath, logger)

	logger.Info("Starting Azure CNI Telemetry Sidecar",
		zap.String("version", version),
		zap.String("configPath", configManager.GetConfigPath()),
		zap.Bool("hasBuiltInAIKey", telemetry.GetAIMetadata() != ""))

	sidecar := NewTelemetrySidecar(configManager, logger, version)

	if err := sidecar.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("Sidecar execution failed", zap.Error(err))
		return fmt.Errorf("sidecar execution failed: %w", err)
	}

	logger.Info("Shutdown complete")
	return nil
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
