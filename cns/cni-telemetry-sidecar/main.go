package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/Azure/azure-container-networking/cns/logger"
)

var (
	version    = "unknown"
	configPath = flag.String("config", "/etc/cns/cns-config.json", "Path to CNS configuration file")
)

func main() {
	flag.Parse()

	// Initialize logging for the CNI telemetry sidecar
	logger.InitLogger("azure-cns-cni-telemetry-sidecar", 1, 1, "/var/log/azure-cns-telemetry")
	defer logger.Close()

	logger.Printf("Starting Azure CNI Telemetry Sidecar v%s", version)

	// Create telemetry sidecar service
	sidecar := NewTelemetrySidecar(*configPath)

	// Setup graceful shutdown context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle OS signals for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		logger.Printf("Received shutdown signal %v, initiating graceful shutdown", sig)
		cancel()
	}()

	// Run the telemetry sidecar
	if err := sidecar.Run(ctx); err != nil {
		logger.Errorf("Azure CNI Telemetry Sidecar failed: %v", err)
		os.Exit(1)
	}

	logger.Printf("Azure CNI Telemetry Sidecar stopped gracefully")
}
