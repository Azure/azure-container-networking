package ciliumpool

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/common"
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
	get := func(url string, metadata bool) ([]byte, error) {
		req, requestErr := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
		if requestErr != nil {
			return nil, fmt.Errorf("creating guest discovery request: %w", requestErr)
		}
		if metadata {
			req.Header.Set("Metadata", "true")
		}
		resp, responseErr := client.Do(req)
		if responseErr != nil {
			return nil, fmt.Errorf("sending guest discovery request: %w", responseErr)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("%w: guest discovery HTTP status %d", errGuestObservation, resp.StatusCode)
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if readErr != nil {
			return nil, fmt.Errorf("reading guest discovery response: %w", readErr)
		}
		return body, nil
	}
	data, err := get("http://169.254.169.254/metadata/instance/compute?api-version=2021-02-01", true)
	if err != nil {
		return Guest{}, fmt.Errorf("reading IMDS: %w", err)
	}
	var compute struct {
		ResourceID string `json:"resourceId"`
		VMID       string `json:"vmId"`
	}
	if err = json.Unmarshal(data, &compute); err != nil {
		return Guest{}, fmt.Errorf("decoding IMDS: %w", err)
	}
	data, err = get("http://168.63.129.16/machine/plugins?comp=nmagent&type=getinterfaceinfov1", false)
	if err != nil {
		return Guest{}, fmt.Errorf("reading NMAgent: %w", err)
	}
	g, err := parseGuest(data, local.HardwareAddr, cfg.Subnet)
	if err != nil {
		return Guest{}, err
	}
	g.InstanceID, g.VMID = compute.ResourceID, compute.VMID
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

func parseGuest(data []byte, mac net.HardwareAddr, subnet string) (Guest, error) {
	prefix, err := netip.ParsePrefix(subnet)
	if err != nil || !prefix.Addr().Is4() || prefix != prefix.Masked() {
		return Guest{}, fmt.Errorf("%w: invalid guest subnet", errGuestObservation)
	}
	var doc common.XmlDocument
	if err := xml.Unmarshal(data, &doc); err != nil {
		return Guest{}, fmt.Errorf("decoding NMAgent interfaces: %w", err)
	}
	g := Guest{MAC: mac, Subnet: prefix, Secondaries: make(map[netip.Addr]bool)}
	found := false
	for _, iface := range doc.Interface {
		raw := strings.ReplaceAll(strings.ReplaceAll(iface.MacAddress, ":", ""), "-", "")
		if !strings.EqualFold(raw, strings.ReplaceAll(mac.String(), ":", "")) {
			continue
		}
		if found || !iface.IsPrimary {
			return Guest{}, fmt.Errorf("%w: selected NIC is not the unique primary", errGuestObservation)
		}
		found = true
		for _, sub := range iface.IPSubnet {
			if sub.Prefix != subnet {
				continue
			}
			for _, raw := range sub.IPAddress {
				addr, err := netip.ParseAddr(raw.Address)
				if err != nil || !addr.Is4() || !prefix.Contains(addr) {
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
