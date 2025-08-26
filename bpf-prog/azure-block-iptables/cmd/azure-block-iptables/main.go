//go:build linux
// +build linux

package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/Azure/azure-container-networking/bpf-prog/azure-block-iptables/pkg/bpfprogram"
	"github.com/pkg/errors"
)

// ProgramVersion is set during build
var (
	version         = "unknown"
	ErrModeRequired = errors.New("mode is required")
	ErrInvalidMode  = errors.New("invalid mode. Use -mode=attach or -mode=detach")
)

// Config holds configuration for the application
type Config struct {
	Mode            string // "attach" or "detach"
	Overwrite       bool   // force detach before attach
	AttacherFactory bpfprogram.AttacherFactory
}

// parseArgs parses command line arguments and returns the configuration
func parseArgs() (*Config, error) {
	var (
		mode        = flag.String("mode", "", "Operation mode: 'attach' or 'detach' (required)")
		overwrite   = flag.Bool("overwrite", false, "Force detach before attach (only applies to attach mode)")
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
		return nil, ErrModeRequired
	}

	if *mode != "attach" && *mode != "detach" {
		return nil, ErrInvalidMode
	}

	return &Config{
		Mode:            *mode,
		Overwrite:       *overwrite,
		AttacherFactory: bpfprogram.NewProgram,
	}, nil
}

// attachMode handles the attach operation
func attachMode(config *Config) error {
	log.Println("Starting attach mode...")

	// Initialize BPF program attacher using the factory
	bp := config.AttacherFactory()

	// If overwrite is enabled, first detach any existing programs
	if config.Overwrite {
		log.Println("Overwrite mode enabled, detaching any existing programs first...")
		if err := bp.Detach(); err != nil {
			log.Printf("Warning: failed to detach existing programs: %v", err)
		}
	}

	// Attach the BPF program
	if err := bp.Attach(); err != nil {
		return errors.Wrap(err, "failed to attach BPF program")
	}

	log.Println("BPF program attached successfully")
	return nil
}

// detachMode handles the detach operation
func detachMode(config *Config) error {
	log.Println("Starting detach mode...")

	// Initialize BPF program attacher using the factory
	bp := config.AttacherFactory()

	// Detach the BPF program
	if err := bp.Detach(); err != nil {
		return errors.Wrap(err, "failed to detach BPF program")
	}

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
		return ErrInvalidMode
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
