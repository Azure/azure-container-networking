package restserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/common"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
)

// Shared test fixtures. Kept as constants so the repeated literals don't trip goconst.
const (
	testMAC01    = "aa:aa:aa:aa:aa:01"
	testMAC02    = "aa:aa:aa:aa:aa:02"
	testMAC03    = "aa:aa:aa:aa:aa:03"
	testMAC04    = "aa:aa:aa:aa:aa:04"
	testMAC05    = "aa:aa:aa:aa:aa:05"
	testMAC07    = "AA:AA:AA:AA:AA:07"
	testMAC08    = "aa:aa:aa:aa:aa:08"
	testNodeName = "node1"
	testNet1     = "net1"
	testNet2     = "net2"
	testGUID1    = "guid1"
	testSubnet1  = "subnet1"
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
	m map[string]*cns.NICResourceSliceInfo
}

func (f *fakeNICNCClient) GetNICResourceSliceInfoByMAC(context.Context) (map[string]*cns.NICResourceSliceInfo, error) {
	return f.m, nil
}

type fakeMTPNCClient struct {
	m map[string]*cns.NICResourceSliceInfo
}

func (f *fakeMTPNCClient) GetMTPNCResourceSliceInfoByMAC(context.Context) (map[string]*cns.NICResourceSliceInfo, error) {
	return f.m, nil
}

func TestGetNICResources(t *testing.T) {
	nodeInfo := &v1alpha1.NodeInfo{
		Spec: v1alpha1.NodeInfoSpec{VMUniqueID: "vm-1"},
		Status: v1alpha1.NodeInfoStatus{DeviceInfos: []v1alpha1.DeviceInfo{
			{MacAddress: testMAC01}, // NICNC DRA                     → 16
			{MacAddress: testMAC02}, // NICNC non-DRA                 → 0
			{MacAddress: testMAC03}, // MTPNC dedicated DRA           → 1
			{MacAddress: testMAC04}, // MTPNC dedicated non-DRA       → 0
			{MacAddress: testMAC05}, // no NICNetworkConfig/MTPNC     → 0
			{MacAddress: testMAC07}, // NICNC via canonical MAC match → 16
			{MacAddress: testMAC08}, // in NICNC and MTPNC; NICNC wins → 16
		}},
	}
	nicNC := map[string]*cns.NICResourceSliceInfo{
		testMAC01: {NetworkID: testNet1, SubnetGUID: testGUID1, SubnetName: testSubnet1, Capacity: 16},
		testMAC02: {NetworkID: testNet2, SubnetGUID: "guid2", SubnetName: "subnet2", Capacity: 0},
		// CRD stores the canonical (lowercase) MAC; NodeInfo reports it uppercase.
		"aa:aa:aa:aa:aa:07": {NetworkID: "net7", SubnetGUID: "guid7", SubnetName: "subnet7", Capacity: 16},
		testMAC08:           {NetworkID: "net8", SubnetGUID: "guid8", SubnetName: "subnet8", Capacity: 16},
	}
	mtpnc := map[string]*cns.NICResourceSliceInfo{
		testMAC03: {NetworkID: "net3", SubnetGUID: "guid3", SubnetName: "subnet3", Capacity: 1},
		testMAC04: {NetworkID: "net4", SubnetGUID: "guid4", SubnetName: "subnet4", Capacity: 0},
		// Also present in NICNetworkConfig above; NICNetworkConfig must win over this.
		testMAC08: {NetworkID: "net8mtpnc", SubnetGUID: "guid8mtpnc", SubnetName: "subnet8mtpnc", Capacity: 1},
	}

	svc := &HTTPRestService{
		Service:        &cns.Service{Service: &common.Service{}},
		nodeName:       testNodeName,
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

	gotCap := map[string]string{}
	gotNet := map[string]string{}
	gotSubGUID := map[string]string{}
	gotSubName := map[string]string{}
	for _, nic := range resp.NICResources {
		gotCap[nic.MacAddress] = nic.Capacity
		gotNet[nic.MacAddress] = nic.NetworkID
		gotSubGUID[nic.MacAddress] = nic.SubnetGUID
		gotSubName[nic.MacAddress] = nic.SubnetName
	}
	wantCap := map[string]string{
		testMAC01: "16",
		testMAC02: "0",
		testMAC03: "1",  // MTPNC dedicated DRA
		testMAC04: "0",  // MTPNC dedicated non-DRA
		testMAC05: "0",  // no NICNetworkConfig/MTPNC → zero capacity
		testMAC07: "16", // NodeInfo reports uppercase; enriched via canonical NICNC key
		testMAC08: "16", // NICNetworkConfig wins over MTPNC
	}
	if len(resp.NICResources) != len(wantCap) {
		t.Fatalf("got %d NIC resources, want %d (%+v)", len(resp.NICResources), len(wantCap), gotCap)
	}
	for mac, want := range wantCap {
		if gotCap[mac] != want {
			t.Errorf("MAC %s capacity = %s, want %s", mac, gotCap[mac], want)
		}
	}
	if gotNet[testMAC01] != testNet1 {
		t.Errorf("MAC 01 networkID = %q, want %s", gotNet[testMAC01], testNet1)
	}
	if gotNet[testMAC03] != "net3" {
		t.Errorf("MAC 03 (MTPNC fallback) networkID = %q, want net3", gotNet[testMAC03])
	}
	if gotNet[testMAC08] != "net8" {
		t.Errorf("MAC 08 (NICNC wins over MTPNC) networkID = %q, want net8", gotNet[testMAC08])
	}
	if gotNet[testMAC07] != "net7" {
		t.Errorf("MAC 07 (uppercase, canonical match) networkID = %q, want net7", gotNet[testMAC07])
	}
	if gotSubGUID[testMAC01] != testGUID1 || gotSubName[testMAC01] != testSubnet1 {
		t.Errorf("MAC 01 subnet = (%q,%q), want (%s,%s)", gotSubGUID[testMAC01], gotSubName[testMAC01], testGUID1, testSubnet1)
	}
}
