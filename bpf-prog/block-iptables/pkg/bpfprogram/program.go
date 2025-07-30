package bpfprogram

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"syscall"

	blockservice "github.com/Azure/azure-container-networking/bpf-prog/block-iptables/pkg/blockservice"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
)

const (
	// BPFMapPinPath is the directory where BPF maps are pinned
	BPFMapPinPath = "/sys/fs/bpf/block-iptables"
	// EventCounterMapName is the name used for pinning the event counter map
	EventCounterMapName = "event_counter"
)

// Program implements the Manager interface for real BPF program operations.
type Program struct {
	objs     *blockservice.BlockIptablesObjects
	links    []link.Link
	attached bool
}

// NewProgram creates a new BPF program manager instance.
func NewProgram() Attacher {
	return &Program{}
}

// CreatePinPath ensures the BPF map pin directory exists.
func (p *Program) CreatePinPath() error {
	// Ensure the BPF map pin directory exists with correct permissions (drwxr-xr-x)
	if err := os.MkdirAll(BPFMapPinPath, 0o755); err != nil {
		return fmt.Errorf("failed to create BPF map pin directory: %w", err)
	}
	return nil
}

// pinEventCounterMap pins the event counter map to the filesystem
func (p *Program) pinEventCounterMap() error {
	if p.objs == nil || p.objs.EventCounter == nil {
		return fmt.Errorf("event counter map not loaded")
	}

	pinPath := filepath.Join(BPFMapPinPath, EventCounterMapName)

	if err := p.objs.EventCounter.Pin(pinPath); err != nil {
		return fmt.Errorf("failed to pin event counter map to %s: %w", pinPath, err)
	}

	log.Printf("Event counter map pinned to %s", pinPath)
	return nil
}

// unpinEventCounterMap unpins the event counter map from the filesystem
func (p *Program) unpinEventCounterMap() error {
	pinPath := filepath.Join(BPFMapPinPath, EventCounterMapName)

	if err := os.Remove(pinPath); err != nil && !os.IsNotExist(err) {
		log.Printf("Warning: failed to remove pinned map %s: %v", pinPath, err)
	} else {
		log.Printf("Event counter map unpinned from %s", pinPath)
	}

	return nil
}

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

// Attach attaches the BPF program to LSM hooks.
func (p *Program) Attach() error {
	if p.attached {
		log.Println("BPF program already attached")
		return nil
	}

	log.Println("Attaching BPF program...")

	// Get the host network namespace inode
	hostNetnsInode, err := getHostNetnsInode()
	if err != nil {
		return fmt.Errorf("failed to get host network namespace inode: %w", err)
	}

	if err := p.CreatePinPath(); err != nil {
		return fmt.Errorf("failed to create BPF map pin directory: %w", err)
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
	options := &ebpf.CollectionOptions{
		Maps: ebpf.MapOptions{
			PinPath:        BPFMapPinPath,
			LoadPinOptions: ebpf.LoadPinOptions{},
		},
	}
	if err := spec.LoadAndAssign(objs, options); err != nil {
		return fmt.Errorf("failed to load BPF objects: %w", err)
	}
	p.objs = objs

	// Pin the event counter map to filesystem
	if err := p.pinEventCounterMap(); err != nil {
		return fmt.Errorf("failed to pin event counter map: %w", err)
	}

	// Attach LSM programs
	var links []link.Link

	// Attach socket_setsockopt LSM hook
	if p.objs.IptablesLegacyBlock != nil {
		l, err := link.AttachLSM(link.LSMOptions{
			Program: p.objs.IptablesLegacyBlock,
		})
		if err != nil {
			p.objs.Close()
			return fmt.Errorf("failed to attach iptables_legacy_block LSM: %w", err)
		}
		links = append(links, l)
	}

	// Attach netlink_send LSM hook
	if p.objs.IptablesNftablesBlock != nil {
		l, err := link.AttachLSM(link.LSMOptions{
			Program: p.objs.IptablesNftablesBlock,
		})
		if err != nil {
			// Clean up previous links
			for _, link := range links {
				link.Close()
			}
			p.objs.Close()
			return fmt.Errorf("failed to attach block_nf_netlink LSM: %w", err)
		}
		links = append(links, l)
	}

	p.links = links
	p.attached = true

	log.Printf("BPF program attached successfully with host_netns_inode=%d", hostNetnsInode)
	return nil
}

// Detach detaches the BPF program from LSM hooks.
func (p *Program) Detach() error {
	if !p.attached {
		log.Println("BPF program already detached")
		return nil
	}

	log.Println("Detaching BPF program...")

	// Unpin the event counter map from filesystem
	if err := p.unpinEventCounterMap(); err != nil {
		log.Printf("Warning: failed to unpin event counter map: %v", err)
	}

	// Close all links
	for _, l := range p.links {
		if err := l.Close(); err != nil {
			log.Printf("Warning: failed to close link: %v", err)
		}
	}
	p.links = nil

	// Close objects
	if p.objs != nil {
		p.objs.Close()
		p.objs = nil
	}

	p.attached = false
	log.Println("BPF program detached successfully")
	return nil
}

// IsAttached returns true if the BPF program is currently attached.
func (p *Program) IsAttached() bool {
	return p.attached
}

// Close cleans up all resources.
func (p *Program) Close() {
	p.Detach()
}
