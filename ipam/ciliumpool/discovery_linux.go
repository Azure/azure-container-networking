package ciliumpool

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strings"

	"github.com/Azure/azure-container-networking/cns/imds"
	"github.com/Azure/azure-container-networking/nmagent"
	"github.com/google/uuid"
)

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
