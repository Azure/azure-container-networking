package restserver

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/common"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
)

// getNICResources must not panic when the NodeInfo client / node name are unset
// (e.g. AttachNodeInfoClient was never called); it should return a clear error
// response instead.
func TestGetNICResourcesNotConfigured(t *testing.T) {
	svc := &HTTPRestService{
		Service: &cns.Service{Service: &common.Service{}},
	} // nodeinfoClient nil, nodeName ""

	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, cns.GetNICResources, http.NoBody)
	svc.getNICResources(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}

	var resp cns.GetNICResourcesResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Response.ReturnCode != types.UnexpectedError {
		t.Errorf("ReturnCode = %v, want %v", resp.Response.ReturnCode, types.UnexpectedError)
	}
}

type fakeNodeInfoClient struct{ nodeInfo *v1alpha1.NodeInfo }

func (f *fakeNodeInfoClient) Get(context.Context, string) (*v1alpha1.NodeInfo, error) {
	return f.nodeInfo, nil
}

type fakeNICNCClient struct {
	m map[string]*cns.NICResourceNetworkInfo
}

func (f *fakeNICNCClient) GetNICResourceNetworkInfoFromNICNC(context.Context) (map[string]*cns.NICResourceNetworkInfo, error) {
	return f.m, nil
}

type fakeMTPNCClient struct {
	m map[string]*cns.NICResourceNetworkInfo
}

func (f *fakeMTPNCClient) GetNICResourceNetworkInfoFromMTPNC(context.Context) (map[string]*cns.NICResourceNetworkInfo, error) {
	return f.m, nil
}

func TestGetNICResources(t *testing.T) {
	tests := []struct {
		name       string
		deviceMAC  string              // MAC as reported by NodeInfo (may be uppercase)
		deviceType v1alpha1.DeviceType // defaults to Vnet NIC when empty
		nicnc      *cns.NICResourceNetworkInfo
		mtpnc      *cns.NICResourceNetworkInfo
		wantCap    string
		wantNet    string
		wantGUID   string
		wantSubnet string
	}{
		{
			name:      "NICNetworkConfig DRA NIC advertises shared capacity",
			deviceMAC: "aa:aa:aa:aa:aa:01",
			nicnc:     &cns.NICResourceNetworkInfo{NetworkID: "net1", SubnetGUID: "guid1", SubnetName: "subnet1", Capacity: 16},
			wantCap:   "16", wantNet: "net1", wantGUID: "guid1", wantSubnet: "subnet1",
		},
		{
			name:      "NICNetworkConfig non-DRA NIC has zero capacity",
			deviceMAC: "aa:aa:aa:aa:aa:02",
			nicnc:     &cns.NICResourceNetworkInfo{NetworkID: "net2", SubnetGUID: "guid2", SubnetName: "subnet2", Capacity: 0},
			wantCap:   "0", wantNet: "net2", wantGUID: "guid2", wantSubnet: "subnet2",
		},
		{
			name:      "MTPNC dedicated DRA NIC fallback advertises capacity 1",
			deviceMAC: "aa:aa:aa:aa:aa:03",
			mtpnc:     &cns.NICResourceNetworkInfo{NetworkID: "net3", SubnetGUID: "guid3", SubnetName: "subnet3", Capacity: 1},
			wantCap:   "1", wantNet: "net3", wantGUID: "guid3", wantSubnet: "subnet3",
		},
		{
			name:      "MTPNC dedicated non-DRA NIC has zero capacity",
			deviceMAC: "aa:aa:aa:aa:aa:04",
			mtpnc:     &cns.NICResourceNetworkInfo{NetworkID: "net4", SubnetGUID: "guid4", SubnetName: "subnet4", Capacity: 0},
			wantCap:   "0", wantNet: "net4", wantGUID: "guid4", wantSubnet: "subnet4",
		},
		{
			name:      "free NIC in neither NICNetworkConfig nor MTPNC has no capacity set",
			deviceMAC: "aa:aa:aa:aa:aa:05",
			wantCap:   "",
		},
		{
			name:      "uppercase NodeInfo MAC matches canonical NICNetworkConfig key",
			deviceMAC: "AA:AA:AA:AA:AA:07",
			nicnc:     &cns.NICResourceNetworkInfo{NetworkID: "net7", SubnetGUID: "guid7", SubnetName: "subnet7", Capacity: 16},
			wantCap:   "16", wantNet: "net7", wantGUID: "guid7", wantSubnet: "subnet7",
		},
		{
			name:      "NIC in both NICNetworkConfig and MTPNC: NICNetworkConfig wins",
			deviceMAC: "aa:aa:aa:aa:aa:08",
			nicnc:     &cns.NICResourceNetworkInfo{NetworkID: "net8", SubnetGUID: "guid8", SubnetName: "subnet8", Capacity: 16},
			mtpnc:     &cns.NICResourceNetworkInfo{NetworkID: "net8mtpnc", SubnetGUID: "guid8mtpnc", SubnetName: "subnet8mtpnc", Capacity: 1},
			wantCap:   "16", wantNet: "net8", wantGUID: "guid8", wantSubnet: "subnet8",
		},
		{
			name:       "InfiniBand NIC is skipped (DRA NIC sharing is vnet-only)",
			deviceMAC:  "aa:aa:aa:aa:aa:06",
			deviceType: v1alpha1.DeviceTypeInfiniBandNIC,
		},
	}

	nodeInfo := &v1alpha1.NodeInfo{Spec: v1alpha1.NodeInfoSpec{VMUniqueID: "vm-1"}}
	nicNC := map[string]*cns.NICResourceNetworkInfo{}
	mtpnc := map[string]*cns.NICResourceNetworkInfo{}
	wantCount := 0
	for _, tc := range tests {
		deviceType := tc.deviceType
		if deviceType == "" {
			deviceType = v1alpha1.DeviceTypeVnetNIC
		}
		nodeInfo.Status.DeviceInfos = append(nodeInfo.Status.DeviceInfos, v1alpha1.DeviceInfo{MacAddress: tc.deviceMAC, DeviceType: deviceType})
		if deviceType == v1alpha1.DeviceTypeVnetNIC {
			wantCount++
		}
		hw, err := net.ParseMAC(tc.deviceMAC)
		if err != nil {
			t.Fatalf("invalid test MAC %q: %v", tc.deviceMAC, err)
		}
		key := hw.String()
		if tc.nicnc != nil {
			nicNC[key] = tc.nicnc
		}
		if tc.mtpnc != nil {
			mtpnc[key] = tc.mtpnc
		}
	}

	svc := &HTTPRestService{
		Service:        &cns.Service{Service: &common.Service{}},
		nodeName:       "nicresnode",
		nodeinfoClient: &fakeNodeInfoClient{nodeInfo: nodeInfo},
		nicncClient:    &fakeNICNCClient{m: nicNC},
		mtpncClient:    &fakeMTPNCClient{m: mtpnc},
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, cns.GetNICResources, http.NoBody)
	svc.getNICResources(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var resp cns.GetNICResourcesResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.NICResources) != wantCount {
		t.Fatalf("got %d NIC resources, want %d", len(resp.NICResources), wantCount)
	}
	got := make(map[string]cns.NICResource, len(resp.NICResources))
	for _, nic := range resp.NICResources {
		got[nic.MacAddress] = nic
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			nic, ok := got[tc.deviceMAC]
			if tc.deviceType != "" && tc.deviceType != v1alpha1.DeviceTypeVnetNIC {
				if ok {
					t.Errorf("non-vnet NIC %s should be excluded, got %+v", tc.deviceMAC, nic)
				}
				return
			}
			if !ok {
				t.Fatalf("MAC %s missing from response", tc.deviceMAC)
			}
			if nic.Capacity != tc.wantCap {
				t.Errorf("capacity = %s, want %s", nic.Capacity, tc.wantCap)
			}
			if nic.NetworkID != tc.wantNet {
				t.Errorf("networkID = %q, want %q", nic.NetworkID, tc.wantNet)
			}
			if nic.SubnetGUID != tc.wantGUID {
				t.Errorf("subnetGUID = %q, want %q", nic.SubnetGUID, tc.wantGUID)
			}
			if nic.SubnetName != tc.wantSubnet {
				t.Errorf("subnetName = %q, want %q", nic.SubnetName, tc.wantSubnet)
			}
		})
	}
}
