package ciliumpool

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"time"

	"github.com/Azure/azure-container-networking/cns/imds"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/nmagent"
)

var errGuestObservation = errors.New("ciliumpool: invalid guest observation")

type Guest struct {
	InstanceID  string
	VMID        string
	MAC         net.HardwareAddr
	Primary     netip.Addr
	Subnet      netip.Prefix
	Secondaries map[netip.Addr]bool
}

// DiscoverGuest bypasses HTTP proxies for the link-local Azure services.
func DiscoverGuest(ctx context.Context, cfg *common.CiliumPoolConfig) (Guest, error) {
	client := &http.Client{
		Timeout:       10 * time.Second,
		Transport:     &http.Transport{Proxy: nil},
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
	defer client.CloseIdleConnections()
	local, err := net.InterfaceByName(cfg.Interface)
	if err != nil {
		return Guest{}, fmt.Errorf("finding selected NIC: %w", err)
	}
	config, err := nmagent.NewConfig("")
	if err != nil {
		return Guest{}, fmt.Errorf("configuring NMAgent: %w", err)
	}
	config.HTTPClient = client
	agent, err := nmagent.NewClient(config)
	if err != nil {
		return Guest{}, fmt.Errorf("creating NMAgent client: %w", err)
	}
	g, err := observeGuest(ctx, imds.NewClient(imds.HTTPClient(client)), agent, local.HardwareAddr, cfg.Subnet)
	if err != nil {
		return Guest{}, err
	}
	addrs, err := local.Addrs()
	if err != nil {
		return Guest{}, fmt.Errorf("reading selected NIC addresses: %w", err)
	}
	for _, raw := range addrs {
		p, err := netip.ParsePrefix(raw.String())
		if err == nil && p.Addr() == g.Primary {
			return g, nil
		}
	}
	return Guest{}, fmt.Errorf("%w: NMAgent primary IP is absent from selected guest NIC", errGuestObservation)
}

func observeGuest(ctx context.Context, metadata *imds.Client, agent *nmagent.Client, mac net.HardwareAddr, subnet string) (Guest, error) {
	compute, err := metadata.GetComputeMetadata(ctx)
	if err != nil {
		return Guest{}, fmt.Errorf("reading IMDS: %w", err)
	}
	interfaces, err := agent.GetInterfaceIPInfo(ctx)
	if err != nil {
		return Guest{}, fmt.Errorf("reading NMAgent: %w", err)
	}
	g, err := parseGuest(interfaces, mac, subnet)
	if err != nil {
		return Guest{}, err
	}
	g.InstanceID, g.VMID = compute.ResourceID, compute.VMID
	return g, nil
}

func parseGuest(doc nmagent.Interfaces, mac net.HardwareAddr, subnet string) (Guest, error) {
	prefix, err := netip.ParsePrefix(subnet)
	if err != nil || !prefix.Addr().Is4() || prefix != prefix.Masked() {
		return Guest{}, fmt.Errorf("%w: invalid guest subnet", errGuestObservation)
	}
	g := Guest{MAC: mac, Subnet: prefix, Secondaries: make(map[netip.Addr]bool)}
	found := false
	for _, iface := range doc.Entries {
		if !bytes.Equal(net.HardwareAddr(iface.MacAddress), mac) {
			continue
		}
		if found || !iface.IsPrimary {
			return Guest{}, fmt.Errorf("%w: selected NIC is not the unique primary", errGuestObservation)
		}
		found = true
		for _, sub := range iface.InterfaceSubnets {
			if sub.Prefix != subnet {
				continue
			}
			for _, raw := range sub.IPAddress {
				addr := netip.Addr(raw.Address)
				if !addr.Is4() || !prefix.Contains(addr) {
					return Guest{}, fmt.Errorf("%w: invalid NMAgent address", errGuestObservation)
				}
				if raw.IsPrimary {
					if g.Primary.IsValid() {
						return Guest{}, fmt.Errorf("%w: multiple primary IPv4 addresses", errGuestObservation)
					}
					g.Primary = addr
				} else {
					g.Secondaries[addr] = true
				}
			}
		}
	}
	if !found || !g.Primary.IsValid() || len(mac) != 6 ||
		bytes.Equal(mac, make([]byte, 6)) {
		return Guest{}, fmt.Errorf("%w: selected primary NIC/subnet absent from NMAgent", errGuestObservation)
	}
	delete(g.Secondaries, g.Primary)
	return g, nil
}
