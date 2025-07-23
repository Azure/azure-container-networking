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

	blockservice "github.com/Azure/azure-container-networking/bpf-prog/block-iptables/pkg/blockservice"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
	"github.com/fsnotify/fsnotify"
)

const (
	DefaultConfigFile = "/etc/cni/net.d/iptables-allow-list"
)

// BPFProgram wraps the eBPF program and its links
type BPFProgram struct {
	objs     *blockservice.BlockIptablesObjects
	links    []link.Link
	attached bool
}

// getHostNetnsInode gets the network namespace inode of the current process (host namespace)
func getHostNetnsInode() (uint32, error) {
	var stat syscall.Stat_t
	err := syscall.Stat("/proc/self/ns/net", &stat)
	if err != nil {
		return 0, fmt.Errorf("failed to stat /proc/self/ns/net: %w", err)
	}

	inode := uint32(stat.Ino)
	log.Printf("Host network namespace inode: %d", inode)
	return inode, nil
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

// attachBPFProgram attaches the BPF program to LSM hooks
func (bp *BPFProgram) attachBPFProgram() error {
	if bp.attached {
		log.Println("BPF program already attached")
		return nil
	}

	log.Println("Attaching BPF program...")

	// Get the host network namespace inode
	hostNetnsInode, err := getHostNetnsInode()
	if err != nil {
		return fmt.Errorf("failed to get host network namespace inode: %w", err)
	}

	// Load BPF objects with the host namespace inode set
	spec, err := blockservice.LoadBlockIptables()
	if err != nil {
		return fmt.Errorf("failed to load BPF spec: %w", err)
	}

	// Set the host_netns_inode variable in the BPF program before loading
	// Note: The C program sets it to hostNetnsInode + 1, so we do the same
	if err := spec.RewriteConstants(map[string]interface{}{
		"host_netns_inode": hostNetnsInode,
	}); err != nil {
		return fmt.Errorf("failed to rewrite constants: %w", err)
	}

	// Load the objects
	objs := &blockservice.BlockIptablesObjects{}
	if err := spec.LoadAndAssign(objs, nil); err != nil {
		return fmt.Errorf("failed to load BPF objects: %w", err)
	}
	bp.objs = objs

	// Attach LSM programs
	var links []link.Link

	// Attach socket_setsockopt LSM hook
	if bp.objs.IptablesLegacyBlock != nil {
		l, err := link.AttachLSM(link.LSMOptions{
			Program: bp.objs.IptablesLegacyBlock,
		})
		if err != nil {
			bp.objs.Close()
			return fmt.Errorf("failed to attach iptables_legacy_block LSM: %w", err)
		}
		links = append(links, l)
	}

	// Attach netlink_send LSM hook
	if bp.objs.IptablesNftablesBlock != nil {
		l, err := link.AttachLSM(link.LSMOptions{
			Program: bp.objs.IptablesNftablesBlock,
		})
		if err != nil {
			// Clean up previous links
			for _, link := range links {
				link.Close()
			}
			bp.objs.Close()
			return fmt.Errorf("failed to attach block_nf_netlink LSM: %w", err)
		}
		links = append(links, l)
	}

	bp.links = links
	bp.attached = true

	log.Printf("BPF program attached successfully with host_netns_inode=%d", hostNetnsInode)
	return nil
}

// detachBPFProgram detaches the BPF program
func (bp *BPFProgram) detachBPFProgram() error {
	if !bp.attached {
		log.Println("BPF program already detached")
		return nil
	}

	log.Println("Detaching BPF program...")

	// Close all links
	for _, l := range bp.links {
		if err := l.Close(); err != nil {
			log.Printf("Warning: failed to close link: %v", err)
		}
	}
	bp.links = nil

	// Close objects
	if bp.objs != nil {
		bp.objs.Close()
		bp.objs = nil
	}

	bp.attached = false
	log.Println("BPF program detached successfully")
	return nil
}

// Close cleans up all resources
func (bp *BPFProgram) Close() {
	bp.detachBPFProgram()
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

// handleFileEvent processes file system events
func handleFileEvent(event fsnotify.Event, configFile string, bp *BPFProgram) {
	// Check if the event is for our config file
	if filepath.Base(event.Name) != filepath.Base(configFile) {
		return
	}

	log.Printf("Config file changed: %s (operation: %s)", event.Name, event.Op)

	// Small delay to handle rapid successive events
	time.Sleep(100 * time.Millisecond)

	// Check current state and take action
	fileState := isFileEmptyOrMissing(configFile)
	switch fileState {
	case 1: // File is empty
		log.Println("File is empty, attaching BPF program")
		if err := bp.attachBPFProgram(); err != nil {
			log.Printf("Failed to attach BPF program: %v", err)
		}
	case 0: // File has content
		log.Println("File has content, detaching BPF program")
		if err := bp.detachBPFProgram(); err != nil {
			log.Printf("Failed to detach BPF program: %v", err)
		}
	case -1: // File is missing
		log.Println("Config file was deleted, detaching BPF program")
		if err := bp.detachBPFProgram(); err != nil {
			log.Printf("Failed to detach BPF program: %v", err)
		}
	}
}

func main() {
	configFile := DefaultConfigFile

	// Parse command line arguments
	if len(os.Args) > 1 {
		configFile = os.Args[1]
	}

	log.Printf("Using config file: %s", configFile)

	// Remove memory limit for eBPF
	if err := rlimit.RemoveMemlock(); err != nil {
		log.Fatalf("Failed to remove memlock rlimit: %v", err)
	}

	// Initialize BPF program wrapper
	bp := &BPFProgram{}
	defer bp.Close()

	// Initial state check
	fileState := isFileEmptyOrMissing(configFile)
	switch fileState {
	case 1: // File is empty
		log.Println("File is empty, attaching BPF program")
		if err := bp.attachBPFProgram(); err != nil {
			log.Fatalf("Failed to attach BPF program: %v", err)
		}
	case 0: // File has content
		log.Println("Config file has content, BPF program will remain detached")
	case -1: // File is missing
		log.Println("Config file is missing, waiting for file to be created...")
	}

	// Setup file watcher
	watcher, err := setupFileWatcher(configFile)
	if err != nil {
		log.Fatalf("Failed to setup file watcher: %v", err)
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
				return
			}
			handleFileEvent(event, configFile, bp)

		case err, ok := <-watcher.Errors:
			if !ok {
				log.Println("Watcher errors channel closed")
				return
			}
			log.Printf("Watcher error: %v", err)

		case sig := <-sigChan:
			log.Printf("Received signal: %v", sig)
			cancel()
			return

		case <-ctx.Done():
			log.Println("Context cancelled, exiting")
			return
		}
	}
}
