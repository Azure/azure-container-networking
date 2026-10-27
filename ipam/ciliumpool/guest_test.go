package ciliumpool

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/Azure/azure-container-networking/cns/imds"
	"github.com/Azure/azure-container-networking/nmagent"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestGuestObservationUsesSharedClients(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body string
		switch r.URL.Path {
		case "/metadata/instance/compute":
			if r.Header.Get("Metadata") != "true" {
				t.Error("metadata request is missing its required header")
			}
			body = `{"resourceId":"vm-resource","vmId":"incarnation"}`
		case "/machine/plugins":
			if r.URL.Query().Get("comp") != "nmagent" {
				t.Error("NMAgent request did not use the wireserver transport")
			}
			body = `<Interfaces><Interface MacAddress="001122334455" IsPrimary="true"><IPSubnet Prefix="10.20.16.0/20">` +
				`<IPAddress Address="10.20.16.4" IsPrimary="true"/><IPAddress Address="10.20.16.10" IsPrimary="false"/>` +
				`</IPSubnet></Interface></Interfaces>`
		default:
			t.Errorf("unexpected guest request %s", r.URL)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	cfg, err := nmagent.NewConfig(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.HTTPClient = server.Client()
	agent, err := nmagent.NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	metadata := imds.NewClient(imds.Endpoint(server.URL), imds.HTTPClient(server.Client()))
	mac := net.HardwareAddr{0, 0x11, 0x22, 0x33, 0x44, 0x55}
	got, err := observeGuest(t.Context(), metadata, agent, mac, "10.20.16.0/20")
	if err != nil {
		t.Fatal(err)
	}
	want := Guest{
		InstanceID: "vm-resource", VMID: "incarnation", MAC: mac,
		Primary: netip.MustParseAddr("10.20.16.4"), Subnet: netip.MustParsePrefix("10.20.16.0/20"),
		Secondaries: map[netip.Addr]bool{netip.MustParseAddr("10.20.16.10"): true},
	}
	if diff := cmp.Diff(want, got, cmpopts.EquateComparable(netip.Addr{}, netip.Prefix{})); diff != "" {
		t.Errorf("guest observation (-want +got):\n%s", diff)
	}
}
