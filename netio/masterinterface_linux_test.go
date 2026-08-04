package netio

import (
	"net"
	"testing"

	vishnetlink "github.com/vishvananda/netlink"
)

type mockLinkResolver struct {
	byName  map[string]vishnetlink.Link
	byIndex map[int]vishnetlink.Link
}

func (m *mockLinkResolver) LinkByName(name string) (vishnetlink.Link, error) {
	l, ok := m.byName[name]
	if !ok {
		return nil, ErrInterfaceNotFound
	}
	return l, nil
}

func (m *mockLinkResolver) LinkByIndex(index int) (vishnetlink.Link, error) {
	l, ok := m.byIndex[index]
	if !ok {
		return nil, ErrInterfaceNotFound
	}
	return l, nil
}

func dummy(index, masterIndex int, name string) vishnetlink.Link {
	return &vishnetlink.Dummy{LinkAttrs: vishnetlink.LinkAttrs{Index: index, MasterIndex: masterIndex, Name: name}}
}

const (
	masterName = "eth2"
	vfName     = "enP55951s3"
)

func TestResolveMasterInterface(t *testing.T) {
	tests := []struct {
		name      string
		iface     *net.Interface
		byName    map[string]vishnetlink.Link
		byIndex   map[int]vishnetlink.Link
		wantName  string
		wantIndex int
		wantErr   bool
	}{
		{
			// The netvsc synthetic is already the master: returned unchanged.
			name:      "not enslaved returns input unchanged",
			iface:     &net.Interface{Index: 4, Name: masterName},
			byName:    map[string]vishnetlink.Link{masterName: dummy(4, 0, masterName)},
			wantName:  masterName,
			wantIndex: 4,
		},
		{
			// A VF matched by MAC must resolve up to its netvsc master.
			name:      "VF resolves to its netvsc master",
			iface:     &net.Interface{Index: 376, Name: vfName},
			byName:    map[string]vishnetlink.Link{vfName: dummy(376, 4, vfName)},
			byIndex:   map[int]vishnetlink.Link{4: dummy(4, 0, masterName)},
			wantName:  masterName,
			wantIndex: 4,
		},
		{
			// Production shape dev257 -> dev4. The VF has the HIGHER ifindex but the
			// LOWER hash bucket (257%256 = 1 < 4), so the MAC lookup returns it first.
			// Resolution must still reach the master.
			name:      "high-ifindex VF in a low hash bucket resolves to master",
			iface:     &net.Interface{Index: 257, Name: "dev257"},
			byName:    map[string]vishnetlink.Link{"dev257": dummy(257, 4, "dev257")},
			byIndex:   map[int]vishnetlink.Link{4: dummy(4, 0, "dev4")},
			wantName:  "dev4",
			wantIndex: 4,
		},
		{
			name:    "nil interface errors",
			iface:   nil,
			wantErr: true,
		},
		{
			name:    "link lookup failure propagates",
			iface:   &net.Interface{Index: 9, Name: "eth9"},
			byName:  map[string]vishnetlink.Link{},
			wantErr: true,
		},
		{
			name:    "master lookup failure propagates",
			iface:   &net.Interface{Index: 376, Name: vfName},
			byName:  map[string]vishnetlink.Link{vfName: dummy(376, 4, vfName)},
			byIndex: map[int]vishnetlink.Link{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orig := masterLinkResolver
			masterLinkResolver = &mockLinkResolver{byName: tt.byName, byIndex: tt.byIndex}
			t.Cleanup(func() { masterLinkResolver = orig })

			got, err := resolveMasterInterface(tt.iface)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Name != tt.wantName || got.Index != tt.wantIndex {
				t.Fatalf("got %s/%d, want %s/%d", got.Name, got.Index, tt.wantName, tt.wantIndex)
			}
		})
	}
}
