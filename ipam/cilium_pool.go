package ipam

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/store"
)

var errCiliumPoolState = errors.New("ipam: invalid cilium pool state")

// CiliumPoolIdentity fences reservations to a single node/VM incarnation.
type CiliumPoolIdentity struct {
	NodeUID       string
	CiliumNodeUID string
	InstanceID    string
	VMID          string
	NICID         string
	Interface     string
	Subnet        string
}

type CiliumPoolState struct {
	Identity    CiliumPoolIdentity
	ReadyUntil  time.Time
	BootID      string
	FenceReason string `json:",omitempty"`
}

type CiliumPoolUsage struct {
	Owner    string `json:"owner,omitempty"`
	Resource string `json:"resource,omitempty"`
}

type CiliumPoolAddress struct {
	Address  netip.Addr
	Resource string
}

// CiliumPoolTransaction must run under the same process lock as azure-vnet-ipam.
// A fresh store is required on every call; the JSON store caches previous reads.
func CiliumPoolTransaction(kvs store.KeyValueStore, identity CiliumPoolIdentity, supply []CiliumPoolAddress,
	initialize, admit bool,
) (used map[string]CiliumPoolUsage, quarantined int, err error) {
	am := &addressManager{store: kvs, AddrSpaces: make(map[string]*addressSpace)}
	if restoreErr := am.restore(false); restoreErr != nil {
		return nil, 0, restoreErr
	}
	if identity.NodeUID == "" || identity.CiliumNodeUID == "" || identity.VMID == "" ||
		identity.InstanceID == "" || identity.NICID == "" || identity.Interface == "" {
		return nil, 0, fmt.Errorf("%w: incomplete cilium pool identity", errCiliumPoolState)
	}
	prefix, err := netip.ParsePrefix(identity.Subnet)
	if err != nil || !prefix.Addr().Is4() || prefix != prefix.Masked() {
		return nil, 0, fmt.Errorf("%w: invalid cilium pool subnet", errCiliumPoolState)
	}
	ap, err := am.bindCiliumPool(identity, prefix, initialize)
	if err != nil {
		return nil, 0, err
	}
	for _, ar := range ap.Addresses {
		ar.Blocked = true
	}
	for _, entry := range supply {
		addr := entry.Address
		if !addr.Is4() || !prefix.Contains(addr) || !strings.EqualFold(entry.Resource, identity.NICID) {
			return nil, 0, fmt.Errorf("%w: invalid cilium pool address %s", errCiliumPoolState, addr)
		}
		ar := ap.Addresses[addr.String()]
		if ar == nil {
			ip := net.IP(addr.AsSlice())
			ar, err = ap.newAddressRecord(&ip)
			if err != nil {
				return nil, 0, err
			}
		}
		ar.Blocked = false
		ar.Resource = entry.Resource
	}
	used = make(map[string]CiliumPoolUsage)
	for ip, ar := range ap.Addresses {
		if ar.InUse || ar.ID != "" {
			used[ip] = CiliumPoolUsage{
				Owner:    "azure-vnet/" + ar.ID,
				Resource: ar.Resource,
			}
		} else if ar.Blocked {
			delete(ap.Addresses, ip)
		}
		if ar.Blocked && (ar.InUse || ar.ID != "") {
			quarantined++
		}
	}
	if admit {
		if am.CiliumPool.FenceReason != "" {
			return nil, 0, fmt.Errorf("%w: cilium pool is fenced: %s", errCiliumPoolState, am.CiliumPool.FenceReason)
		}
		// A bounded observation lease limits admissions during bridge/API outages.
		am.CiliumPool.ReadyUntil = time.Now().Add(30 * time.Second)
		am.CiliumPool.BootID, err = common.CiliumPoolBootID()
		if err != nil {
			return nil, 0, fmt.Errorf("reading pool admission boot ID: %w", err)
		}
	}
	if err := am.save(); err != nil {
		return nil, 0, err
	}
	return used, quarantined, nil
}

func (am *addressManager) bindCiliumPool(identity CiliumPoolIdentity, prefix netip.Prefix, initialize bool) (*addressPool, error) {
	if am.CiliumPool == nil {
		if !initialize || len(am.AddrSpaces) != 0 {
			return nil, fmt.Errorf("%w: missing cilium pool identity; fenced recovery required", errCiliumPoolState)
		}
		am.CiliumPool = &CiliumPoolState{Identity: identity}
	}
	if am.CiliumPool.Identity != identity {
		am.CiliumPool.ReadyUntil = time.Time{}
		am.CiliumPool.FenceReason = "identity changed"
		if err := am.save(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("%w: cilium pool identity changed; fenced recovery required", errCiliumPoolState)
	}
	as := am.AddrSpaces[LocalDefaultAddressSpaceId]
	if as == nil {
		var err error
		as, err = am.newAddressSpace(LocalDefaultAddressSpaceId, LocalScope)
		if err != nil {
			return nil, err
		}
		am.AddrSpaces[as.Id] = as
	}
	ap := as.Pools[identity.Subnet]
	if ap == nil {
		subnet := &net.IPNet{IP: net.IP(prefix.Addr().AsSlice()), Mask: net.CIDRMask(prefix.Bits(), 32)}
		var err error
		ap, err = as.newAddressPool(identity.Interface, 0, subnet)
		if err != nil {
			return nil, err
		}
	}
	ap.OperatorManaged = true
	if len(am.AddrSpaces) != 1 || len(as.Pools) != 1 || ap.IfName != identity.Interface {
		return nil, fmt.Errorf("%w: unexpected local pools in cilium mode", errCiliumPoolState)
	}
	return ap, nil
}

func (am *addressManager) checkCiliumPoolAdmission() error {
	if am.ciliumPoolMode || am.CiliumPool != nil {
		if am.CiliumPool == nil || am.CiliumPool.FenceReason != "" || time.Now().After(am.CiliumPool.ReadyUntil) {
			return fmt.Errorf("%w: cilium pool admissions paused; waiting for bridge synchronization", errCiliumPoolState)
		}
		bootID, err := common.CiliumPoolBootID()
		if err != nil {
			return fmt.Errorf("reading pool admission boot ID: %w", err)
		}
		if am.CiliumPool.BootID != bootID {
			return fmt.Errorf("%w: cilium pool awaits post-reboot synchronization", errCiliumPoolState)
		}
	}
	return nil
}

func ciliumPoolEnvironment(options map[string]interface{}) bool {
	return options[common.OptEnvironment] == common.OptEnvironmentCiliumPool
}

// FenceCiliumPool persists an admission stop without discarding reservations.
// The caller holds the IPAM process lock and uses a fresh store.
func FenceCiliumPool(kvs store.KeyValueStore, reason string) error {
	am := &addressManager{store: kvs, AddrSpaces: make(map[string]*addressSpace)}
	if err := am.restore(false); err != nil {
		return err
	}
	if am.CiliumPool == nil {
		return fmt.Errorf("%w: cannot fence missing pool state", errCiliumPoolState)
	}
	am.CiliumPool.FenceReason = reason
	am.CiliumPool.ReadyUntil = time.Time{}
	return am.save()
}

// CiliumPoolHasReservations reads ownership without changing state.
// The caller must hold the IPAM process lock and use a fresh store.
func CiliumPoolHasReservations(kvs store.KeyValueStore) (bool, error) {
	am := &addressManager{store: kvs, AddrSpaces: make(map[string]*addressSpace)}
	if err := am.restore(false); err != nil {
		return false, err
	}
	for _, as := range am.AddrSpaces {
		for _, ap := range as.Pools {
			if ap.IsAnyRecordInUse() {
				return true, nil
			}
		}
	}
	return false, nil
}

func (am *addressManager) validateCiliumPoolRecords() error {
	id := am.CiliumPool.Identity
	prefix, err := netip.ParsePrefix(id.Subnet)
	if err != nil || !prefix.Addr().Is4() || prefix != prefix.Masked() ||
		id.NodeUID == "" || id.CiliumNodeUID == "" || id.InstanceID == "" || id.VMID == "" ||
		id.NICID == "" || id.Interface == "" || len(am.AddrSpaces) != 1 {
		return fmt.Errorf("%w: invalid persisted cilium pool identity or address space", errCiliumPoolState)
	}
	as := am.AddrSpaces[LocalDefaultAddressSpaceId]
	if as == nil || as.Id != LocalDefaultAddressSpaceId || as.Scope != LocalScope || len(as.Pools) != 1 {
		return fmt.Errorf("%w: invalid persisted cilium pool address space", errCiliumPoolState)
	}
	ap := as.Pools[id.Subnet]
	if ap == nil || !ap.OperatorManaged || ap.Id != id.Subnet || ap.Subnet.String() != id.Subnet ||
		ap.IfName != id.Interface || ap.IsIPv6 || ap.Addresses == nil {
		return fmt.Errorf("%w: invalid persisted cilium pool", errCiliumPoolState)
	}
	owners := make(map[string]bool)
	for raw, ar := range ap.Addresses {
		if err := validateCiliumPoolAddress(raw, ar, prefix, id.NICID); err != nil {
			return err
		}
		if ar.ID != "" {
			if owners[ar.ID] {
				return fmt.Errorf("%w: duplicate persisted cilium pool owner", errCiliumPoolState)
			}
			owners[ar.ID] = true
		}
	}
	return nil
}

func validateCiliumPoolAddress(raw string, ar *addressRecord, prefix netip.Prefix, nicID string) error {
	addr, err := netip.ParseAddr(raw)
	if ar == nil || err != nil || !addr.Is4() || !prefix.Contains(addr) ||
		ar.Addr.String() != raw || !strings.EqualFold(ar.Resource, nicID) {
		return fmt.Errorf("%w: invalid persisted cilium pool address %q", errCiliumPoolState, raw)
	}
	return nil
}
