//go:build linux
// +build linux

package bpfprogram

import (
	"log"
	"os"
	"path/filepath"
	"syscall"

	blockservice "github.com/Azure/azure-container-networking/bpf-prog/azure-block-iptables/pkg/blockservice"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
	"github.com/pkg/errors"
)

const (
	// BPFMapPinPath is the directory where BPF maps are pinned
	BPFMapPinPath = "/sys/fs/bpf/block-iptables"
	// EventCounterMapName is the name used for pinning the event counter map
	EventCounterMapName = "iptables_block_event_counter"
	// IptablesLegacyBlockProgramName is the name used for pinning the legacy iptables block program
	IptablesLegacyBlockProgramName = "iptables_legacy_block"
	// IptablesNftablesBlockProgramName is the name used for pinning the nftables block program
	IptablesNftablesBlockProgramName = "iptables_nftables_block"
	// NetNSPath is the path to the host network namespace
	NetNSPath = "/proc/self/ns/net"
)

var ErrEventCounterMapNotLoaded = errors.New("event counter map not loaded")

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
		return errors.Wrap(err, "failed to create BPF map pin directory")
	}
	return nil
}

// pinEventCounterMap pins the event counter map to the filesystem
func (p *Program) pinEventCounterMap() error {
	if p.objs == nil || p.objs.IptablesBlockEventCounter == nil {
		return ErrEventCounterMapNotLoaded
	}

	pinPath := filepath.Join(BPFMapPinPath, EventCounterMapName)

	if err := p.objs.IptablesBlockEventCounter.Pin(pinPath); err != nil {
		return errors.Wrapf(err, "failed to pin event counter map to %s", pinPath)
	}

	log.Printf("Event counter map pinned to %s", pinPath)
	return nil
}

// unpinEventCounterMap unpins the event counter map from the filesystem
func (p *Program) unpinEventCounterMap() error {
	pinPath := filepath.Join(BPFMapPinPath, EventCounterMapName)

	if err := os.Remove(pinPath); err != nil && !os.IsNotExist(err) {
		return errors.Wrapf(err, "failed to remove pinned map %s", pinPath)
	}

	log.Printf("Event counter map unpinned from %s", pinPath)
	return nil
}

// unpinLinks unpins the links to BPF programs from the filesystem
func (p *Program) unpinLinks() error {
	var errs []error

	// Unpin the legacy iptables block program
	legacyPinPath := filepath.Join(BPFMapPinPath, IptablesLegacyBlockProgramName)
	if err := os.Remove(legacyPinPath); err != nil && !os.IsNotExist(err) {
		errs = append(errs, errors.Wrapf(err, "failed to remove pinned legacy program %s", legacyPinPath))
	} else {
		log.Printf("Legacy iptables block program unpinned from %s", legacyPinPath)
	}

	// Unpin the nftables block program
	nftablesPinPath := filepath.Join(BPFMapPinPath, IptablesNftablesBlockProgramName)
	if err := os.Remove(nftablesPinPath); err != nil && !os.IsNotExist(err) {
		errs = append(errs, errors.Wrapf(err, "failed to remove pinned nftables program %s", nftablesPinPath))
	} else {
		log.Printf("Nftables block program unpinned from %s", nftablesPinPath)
	}

	if len(errs) > 0 {
		return errors.Errorf("failed to unpin programs: %v", errs)
	}

	return nil
}

func getHostNetnsInode() (uint64, error) {
	var stat syscall.Stat_t
	err := syscall.Stat(NetNSPath, &stat)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to stat %s", NetNSPath)
	}

	log.Printf("Host network namespace inode: %d", stat.Ino)
	return stat.Ino, nil
}

// Attach attaches the BPF program to LSM hooks.
func (p *Program) Attach() error {
	if p.attached {
		log.Println("BPF program already attached")
		return nil
	}

	// Remove memory limit for eBPF
	if err := rlimit.RemoveMemlock(); err != nil {
		return errors.Wrapf(err, "failed to remove memlock rlimit")
	}

	log.Println("Attaching BPF program...")

	// Get the host network namespace inode
	hostNetnsInode, err := getHostNetnsInode()
	if err != nil {
		return errors.Wrap(err, "failed to get host network namespace inode")
	}

	if err = p.CreatePinPath(); err != nil {
		return errors.Wrap(err, "failed to create BPF map pin directory")
	}

	// Load BPF objects with the host namespace inode set
	spec, err := blockservice.LoadBlockIptables()
	if err != nil {
		return errors.Wrap(err, "failed to load BPF spec")
	}

	// Set the host_netns_inode variable in the BPF program before loading
	if err = spec.RewriteConstants(map[string]interface{}{
		"host_netns_inode": hostNetnsInode,
	}); err != nil {
		return errors.Wrap(err, "failed to rewrite constants")
	}

	// Load the objects
	objs := &blockservice.BlockIptablesObjects{}
	options := &ebpf.CollectionOptions{
		Maps: ebpf.MapOptions{
			PinPath:        BPFMapPinPath,
			LoadPinOptions: ebpf.LoadPinOptions{},
		},
	}
	if err = spec.LoadAndAssign(objs, options); err != nil {
		return errors.Wrap(err, "failed to load BPF objects")
	}
	p.objs = objs

	// Pin the event counter map to filesystem
	if err = p.pinEventCounterMap(); err != nil {
		return errors.Wrap(err, "failed to pin event counter map")
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
			p.objs = nil
			return errors.Wrap(err, "failed to attach iptables_legacy_block LSM")
		}

		pinPath := filepath.Join(BPFMapPinPath, IptablesLegacyBlockProgramName)
		err = l.Pin(pinPath)
		if err != nil {
			l.Close()
			return errors.Wrap(err, "failed to pin iptables_legacy_block LSM")
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
				link = nil
			}
			p.objs.Close()
			p.objs = nil
			return errors.Wrap(err, "failed to attach block_nf_netlink LSM")
		}
		pinPath := filepath.Join(BPFMapPinPath, IptablesNftablesBlockProgramName)
		err = l.Pin(pinPath)
		if err != nil {
			for _, link := range links {
				link.Close()
			}

			l.Close()
			return errors.Wrap(err, "failed to pin iptables_nftables_block LSM")
		}

		links = append(links, l)
	}

	p.links = links
	p.attached = true

	log.Printf("BPF program attached successfully with host_netns_inode=%d", hostNetnsInode)
	return nil
}

func (p *Program) Detach() error {
	return p.cleanupPinnedResources()
}

// cleanupPinnedResources removes pinned resources even when the program is not currently attached
func (p *Program) cleanupPinnedResources() error {
	log.Println("Cleaning up pinned resources...")

	// Try to unpin links
	if err := p.unpinLinks(); err != nil {
		log.Printf("Warning: failed to unpin links: %v", err)
	}

	// Try to unpin the event counter map
	if err := p.unpinEventCounterMap(); err != nil {
		log.Printf("Warning: failed to unpin event counter map: %v", err)
	}

	log.Println("Pinned resources cleanup completed")
	return nil
}

// IsAttached returns true if the BPF program is currently attached.
func (p *Program) IsAttached() bool {
	return p.attached
}

// Close cleans up all resources.
func (p *Program) Close() {
	if err := p.Detach(); err != nil {
		log.Println("Warning: failed to detach BPF program:", err)
	}
}
