//go:build linux
// +build linux

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Azure/azure-container-networking/bpf-prog/azure-block-iptables/pkg/bpfprogram"
	"github.com/cilium/ebpf/rlimit"
	"github.com/fsnotify/fsnotify"
)

const (
	DefaultConfigFile = "/etc/cni/net.d/iptables-allow-list"
)

// BlockConfig holds configuration for the application
type BlockConfig struct {
	ConfigFile      string
	AttacherFactory bpfprogram.AttacherFactory
}

// NewDefaultBlockConfig creates a new BlockConfig with default values
func NewDefaultBlockConfig() *BlockConfig {
	return &BlockConfig{
		ConfigFile:      DefaultConfigFile,
		AttacherFactory: bpfprogram.NewProgram,
	}
}

// isFileEmptyOrMissing checks if the config file exists and has content
// Returns: 1 if empty, 0 if has content, -1 if missing/error
func isFileEmptyOrMissing(filename string) int {
	stat, err := os.Stat(filename)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("Config file %s does not exist", filename)
			return -1 // File missing
		}
		log.Printf("Error checking file %s: %v", filename, err)
		return -1 // Treat errors as missing
	}

	if stat.Size() == 0 {
		log.Printf("Config file %s is empty", filename)
		return 1 // File empty
	}

	log.Printf("Config file %s has content (size: %d bytes)", filename, stat.Size())
	return 0 // File exists and has content
}

// setupFileWatcher sets up a file watcher for the config file
func setupFileWatcher(configFile string) (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	// Watch the directory containing the config file
	dir := filepath.Dir(configFile)
	err = watcher.Add(dir)
	if err != nil {
		watcher.Close()
		return nil, fmt.Errorf("failed to add watch for directory %s: %w", dir, err)
	}

	log.Printf("Watching directory %s for changes to %s", dir, configFile)
	return watcher, nil
}

func checkFileStatusAndUpdateBPF(configFile string, bp bpfprogram.Attacher) {
	// Check current state and take action
	fileState := isFileEmptyOrMissing(configFile)
	switch fileState {
	case 1: // File is empty
		log.Println("File is empty, attaching BPF program")
		if err := bp.Attach(); err != nil { // No-op if already attached
			log.Printf("Failed to attach BPF program: %v", err)
		}
	case 0: // File has content
		log.Println("File has content, detaching BPF program")
		if err := bp.Detach(); err != nil { // No-op if already detached
			log.Printf("Failed to detach BPF program: %v", err)
		}
	case -1: // File is missing
		log.Println("Config file was deleted, detaching BPF program")
		if err := bp.Detach(); err != nil { // No-op if already detached
			log.Printf("Failed to detach BPF program: %v", err)
		}
	}
}

// handleFileEvent processes file system events
func handleFileEvent(event fsnotify.Event, configFile string, bp bpfprogram.Attacher) {
	// Check if the event is for our config file
	if filepath.Base(event.Name) != filepath.Base(configFile) {
		return
	}

	log.Printf("Config file changed: %s (operation: %s)", event.Name, event.Op)

	// Small delay to handle rapid successive events
	time.Sleep(100 * time.Millisecond)
	checkFileStatusAndUpdateBPF(configFile, bp)
}

// run is the main application logic, separated for easier testing
func run(config *BlockConfig) error {
	log.Printf("Using config file: %s", config.ConfigFile)

	// Remove memory limit for eBPF
	if err := rlimit.RemoveMemlock(); err != nil {
		return fmt.Errorf("failed to remove memlock rlimit: %w", err)
	}

	// Initialize BPF program attacher using the factory
	bp := config.AttacherFactory()
	defer bp.Close()

	// Check initial state of the config file
	checkFileStatusAndUpdateBPF(config.ConfigFile, bp)

	// Setup file watcher
	watcher, err := setupFileWatcher(config.ConfigFile)
	if err != nil {
		return fmt.Errorf("failed to setup file watcher: %w", err)
	}
	defer watcher.Close()

	// Setup signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Println("Starting file watch loop...")

	// Main event loop
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				log.Println("Watcher events channel closed")
				return nil
			}
			handleFileEvent(event, config.ConfigFile, bp)

		case err, ok := <-watcher.Errors:
			if !ok {
				log.Println("Watcher errors channel closed")
				return nil
			}
			log.Printf("Watcher error: %v", err)

		case sig := <-sigChan:
			log.Printf("Received signal: %v", sig)
			cancel()
			return nil

		case <-ctx.Done():
			log.Println("Context cancelled, exiting")
			return nil
		}
	}
}

func main() {
	config := NewDefaultBlockConfig()

	// Parse command line arguments
	if len(os.Args) > 1 {
		config.ConfigFile = os.Args[1]
	}

	if err := run(config); err != nil {
		log.Fatalf("Application failed: %v", err)
	}
}
