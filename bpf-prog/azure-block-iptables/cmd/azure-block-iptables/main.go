//go:build linux
// +build linux

package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/Azure/azure-container-networking/bpf-prog/azure-block-iptables/pkg/bpfprogram"
)

// ProgramVersion is set during build
var version = "unknown"

// Config holds configuration for the application
type Config struct {
	Mode            string // "attach" or "detach"
	AttacherFactory bpfprogram.AttacherFactory
}

// parseArgs parses command line arguments and returns the configuration
func parseArgs() (*Config, error) {
	var (
		mode        = flag.String("mode", "", "Operation mode: 'attach' or 'detach' (required)")
		showVersion = flag.Bool("version", false, "Show version information")
		showHelp    = flag.Bool("help", false, "Show help information")
	)

	flag.Parse()

	if *showVersion {
		fmt.Printf("azure-block-iptables version %s\n", version)
		os.Exit(0)
	}

	if *showHelp {
		flag.PrintDefaults()
		os.Exit(0)
	}

	if *mode == "" {
		return nil, fmt.Errorf("mode is required. Use -mode=attach or -mode=detach")
	}

	if *mode != "attach" && *mode != "detach" {
		return nil, fmt.Errorf("invalid mode '%s'. Must be 'attach' or 'detach'", *mode)
	}

	return &Config{
		Mode:            *mode,
		AttacherFactory: bpfprogram.NewProgram,
	}, nil
}

// attachMode handles the attach operation
func attachMode(config *Config) error {
	log.Println("Starting attach mode...")

	// Initialize BPF program attacher using the factory
	bp := config.AttacherFactory()

	// Attach the BPF program
	if err := bp.Attach(); err != nil {
		return fmt.Errorf("failed to attach BPF program: %w", err)
	}

	log.Println("BPF program attached successfully")
	return nil
}

// detachMode handles the detach operation
func detachMode(config *Config) error {
	log.Println("Starting detach mode...")

	// Initialize BPF program attacher using the factory
	bp := config.AttacherFactory()
	bp.Detach()
	log.Println("BPF program detached successfully")
	return nil
}

// run is the main application logic
func run(config *Config) error {
	switch config.Mode {
	case "attach":
		return attachMode(config)
	case "detach":
		return detachMode(config)
	default:
		return fmt.Errorf("unsupported mode: %s", config.Mode)
	}
}

func main() {
	config, err := parseArgs()
	if err != nil {
		log.Printf("Error parsing arguments: %v", err)
		flag.PrintDefaults()
		os.Exit(1)
	}

	if err := run(config); err != nil {
		log.Fatalf("Application failed: %v", err)
	}
}
