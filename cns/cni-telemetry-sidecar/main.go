package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/Azure/azure-container-networking/cns/logger/v2"
	cores "github.com/Azure/azure-container-networking/cns/logger/v2/cores"
	"go.uber.org/zap"
)

var (
	version    = "unknown"
	configPath = flag.String("config", "/etc/cns/cns-config.json", "Path to CNS configuration file")
	logLevel   = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
)

func main() {
	flag.Parse()

	// Initialize main logger with correct path for shared volume
	zapLogger, cleanup, err := logger.New(&logger.Config{
		Level: *logLevel,
		File: &cores.FileConfig{
			Filepath: "/var/log/azure-cni-telemetry-sidecar.log", // This will write to host's /var/log/azure-cns/
		},
	})
	if err != nil {
		panic("Failed to initialize logger: " + err.Error())
	}
	defer cleanup()

	zapLogger.Info("Starting Azure CNI Telemetry Sidecar",
		zap.String("version", version),
		zap.String("configPath", *configPath),
		zap.String("logLevel", *logLevel))

	// Create telemetry sidecar service and pass the logger
	sidecar := NewTelemetrySidecar(*configPath)

	// Set the logger for the sidecar to avoid nil pointer
	if err := sidecar.SetLogger(zapLogger); err != nil {
		zapLogger.Error("Failed to set logger for sidecar", zap.Error(err))
		os.Exit(1)
	}

	// Setup graceful shutdown context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle OS signals for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		zapLogger.Info("Received shutdown signal, initiating graceful shutdown",
			zap.String("signal", sig.String()))
		cancel()
	}()

	// Run the telemetry sidecar (using the Run method from sidecar.go)
	if err := sidecar.Run(ctx); err != nil {
		zapLogger.Error("Azure CNI Telemetry Sidecar failed",
			zap.Error(err))
		os.Exit(1)
	}

	zapLogger.Info("Azure CNI Telemetry Sidecar stopped gracefully")
}
