package ciliumpool

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/netip"
	"os"
	"slices"
	"strings"

	"github.com/Azure/azure-container-networking/cns/imds"
	"github.com/Azure/azure-container-networking/nmagent"
	"github.com/google/uuid"
)

type Guest struct {
	VMID       string
	InstanceID string
	BootID     string
	Primary    netip.Addr
	MAC        net.HardwareAddr
	Supply     map[string]bool
}

func parseInterfaces(doc nmagent.Interfaces, mac net.HardwareAddr, subnet netip.Prefix, host []netip.Addr) (Guest, error) {
	result := Guest{MAC: mac, Supply: map[string]bool{}}
	found := false
	for _, iface := range doc.Entries {
		if !bytes.Equal([]byte(iface.MacAddress), mac) {
			continue
		}
		if found || !iface.IsPrimary {
			return Guest{}, fmt.Errorf("%w: selected NIC is not unique primary", ErrConfig)
		}
		found = true
		for _, sub := range iface.InterfaceSubnets {
			if sub.Prefix != subnet.String() {
				continue
			}
			for _, entry := range sub.IPAddress {
				ip := netip.Addr(entry.Address)
				if !ip.Is4() || !subnet.Contains(ip) {
					return Guest{}, fmt.Errorf("%w: invalid guest address", ErrConfig)
				}
				if entry.IsPrimary {
					if result.Primary.IsValid() {
						return Guest{}, fmt.Errorf("%w: multiple guest primary addresses", ErrConfig)
					}
					result.Primary = ip
				} else {
					if slices.Contains(host, ip) {
						return Guest{}, fmt.Errorf("%w: secondary address %s is configured on the host", ErrConfig, ip)
					}
					result.Supply[ip.String()] = true
				}
			}
		}
	}
	if !found || !result.Primary.IsValid() || !slices.Contains(host, result.Primary) {
		return Guest{}, fmt.Errorf("%w: primary NIC or subnet is absent", ErrConfig)
	}
	delete(result.Supply, result.Primary.String())
	return result, nil
}

func parseBootID(data []byte) (string, error) {
	id, err := uuid.Parse(strings.TrimSpace(string(data)))
	if err != nil || id == uuid.Nil {
		return "", fmt.Errorf("%w: invalid kernel boot identity", ErrOwnership)
	}
	return id.String(), nil
}

// Discover omits supply discovery on DEL, which must release withdrawn addresses.
func Discover(ctx context.Context, cfg Config, observeSupply bool) (Guest, error) {
	compute, err := imds.NewClient().GetComputeMetadata(ctx)
	if err != nil {
		return Guest{}, fmt.Errorf("reading guest identity: %w", err)
	}
	raw, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return Guest{}, fmt.Errorf("reading kernel boot ID: %w", err)
	}
	boot, err := parseBootID(raw)
	if err != nil {
		return Guest{}, err
	}
	g := Guest{VMID: compute.VMID, InstanceID: compute.ResourceID, BootID: boot}
	if !observeSupply {
		return g, nil
	}
	local, err := net.InterfaceByName(cfg.Interface)
	if err != nil {
		return Guest{}, fmt.Errorf("finding guest NIC: %w", err)
	}
	addresses, err := local.Addrs()
	if err != nil {
		return Guest{}, fmt.Errorf("reading guest NIC addresses: %w", err)
	}
	host := make([]netip.Addr, 0, len(addresses))
	for _, address := range addresses {
		prefix, callErr := netip.ParsePrefix(address.String())
		if callErr != nil {
			return Guest{}, fmt.Errorf("parsing host address: %w", callErr)
		}
		host = append(host, prefix.Addr())
	}
	config, err := nmagent.NewConfig("")
	if err != nil {
		return Guest{}, fmt.Errorf("configuring NMAgent: %w", err)
	}
	agent, err := nmagent.NewClient(config)
	if err != nil {
		return Guest{}, fmt.Errorf("creating NMAgent client: %w", err)
	}
	interfaces, err := agent.GetInterfaceIPInfo(ctx)
	if err != nil {
		return Guest{}, fmt.Errorf("reading NMAgent supply: %w", err)
	}
	subnet, err := netip.ParsePrefix(cfg.Subnet)
	if err != nil {
		return Guest{}, fmt.Errorf("parsing guest subnet: %w", err)
	}
	observed, err := parseInterfaces(interfaces, local.HardwareAddr, subnet, host)
	if err != nil {
		return Guest{}, err
	}
	g.Primary, g.MAC, g.Supply = observed.Primary, observed.MAC, observed.Supply
	return g, nil
}
