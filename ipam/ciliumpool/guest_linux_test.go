package ciliumpool

import (
	"encoding/xml"
	"net"
	"net/netip"
	"testing"

	"github.com/Azure/azure-container-networking/nmagent"
)

func TestGuestSupplyRequiresPrimaryNICAndNoHostOwnedSecondary(t *testing.T) {
	var doc nmagent.Interfaces
	if err := xml.Unmarshal([]byte(`<Interfaces><Interface MacAddress="001122334455" IsPrimary="true">
		<IPSubnet Prefix="10.0.0.0/24"><IPAddress Address="10.0.0.4" IsPrimary="true"/>
		<IPAddress Address="10.0.0.10" IsPrimary="false"/></IPSubnet></Interface></Interfaces>`), &doc); err != nil {
		t.Fatal(err)
	}
	mac, err := net.ParseMAC(testMAC)
	if err != nil {
		t.Fatal(err)
	}
	primary := netip.MustParseAddr(testPrimary)
	subnet := netip.MustParsePrefix(testSubnet)
	guest, err := parseInterfaces(doc, mac, subnet, []netip.Addr{primary})
	if err != nil || guest.Primary != primary || !guest.Supply[testIP] || guest.Supply[primary.String()] {
		t.Fatalf("guest = %+v, %v", guest, err)
	}
	for _, tt := range []struct {
		name  string
		mac   net.HardwareAddr
		hosts []netip.Addr
	}{
		{"foreign NIC", net.HardwareAddr{1, 2, 3, 4, 5, 6}, []netip.Addr{primary}},
		{"missing primary", mac, nil},
		{"host owns secondary", mac, []netip.Addr{primary, netip.MustParseAddr(testIP)}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseInterfaces(doc, tt.mac, subnet, tt.hosts); err == nil {
				t.Fatal("invalid guest observation admitted")
			}
		})
	}
}
