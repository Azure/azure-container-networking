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

// getNICResources must not panic when the NodeInfo client / node name are unset
// (e.g. AttachNodeInfoClient was never called); it should return a clear error
// response instead.
func TestGetNICResourcesNotConfigured(t *testing.T) {
	svc := &HTTPRestService{
		Service: &cns.Service{Service: &common.Service{}},
	} // nodeInfoCli nil, nodeName ""

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, cns.GetNICResources, nil)
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

func (f *fakeNICNCClient) GetNICNCInfoByMAC(context.Context) (map[string]*cns.NICResourceSliceInfo, error) {
	return f.m, nil
}

type fakeMTPNCClient struct {
	m map[string]*cns.NICResourceSliceInfo
}

func (f *fakeMTPNCClient) GetMTPNCInfoByMAC(context.Context) (map[string]*cns.NICResourceSliceInfo, error) {
	return f.m, nil
}

func TestGetNICResources(t *testing.T) {
	nodeInfo := &v1alpha1.NodeInfo{
		Spec: v1alpha1.NodeInfoSpec{VMUniqueID: "vm-1"},
		Status: v1alpha1.NodeInfoStatus{DeviceInfos: []v1alpha1.DeviceInfo{
			{MacAddress: "aa:aa:aa:aa:aa:01"}, // NICNC DRA                     → 16
			{MacAddress: "aa:aa:aa:aa:aa:02"}, // NICNC non-DRA                 → 0
			{MacAddress: "aa:aa:aa:aa:aa:03"}, // MTPNC dedicated DRA           → 1
			{MacAddress: "aa:aa:aa:aa:aa:04"}, // MTPNC dedicated non-DRA       → 0
			{MacAddress: "aa:aa:aa:aa:aa:05"}, // no NICNetworkConfig/MTPNC     → 0
			{MacAddress: "AA:AA:AA:AA:AA:07"}, // NICNC via canonical MAC match → 16
			{MacAddress: "aa:aa:aa:aa:aa:08"}, // in NICNC and MTPNC; NICNC wins → 16
		}},
	}
	nicNC := map[string]*cns.NICResourceSliceInfo{
		"aa:aa:aa:aa:aa:01": {NetworkID: "net1", SubnetGUID: "guid1", SubnetName: "subnet1", Capacity: 16},
		"aa:aa:aa:aa:aa:02": {NetworkID: "net2", SubnetGUID: "guid2", SubnetName: "subnet2", Capacity: 0},
		// CRD stores the canonical (lowercase) MAC; NodeInfo reports it uppercase.
		"aa:aa:aa:aa:aa:07": {NetworkID: "net7", SubnetGUID: "guid7", SubnetName: "subnet7", Capacity: 16},
		"aa:aa:aa:aa:aa:08": {NetworkID: "net8", SubnetGUID: "guid8", SubnetName: "subnet8", Capacity: 16},
	}
	mtpnc := map[string]*cns.NICResourceSliceInfo{
		"aa:aa:aa:aa:aa:03": {NetworkID: "net3", SubnetGUID: "guid3", SubnetName: "subnet3", Capacity: 1},
		"aa:aa:aa:aa:aa:04": {NetworkID: "net4", SubnetGUID: "guid4", SubnetName: "subnet4", Capacity: 0},
		// Also present in NICNetworkConfig above; NICNetworkConfig must win over this.
		"aa:aa:aa:aa:aa:08": {NetworkID: "net8mtpnc", SubnetGUID: "guid8mtpnc", SubnetName: "subnet8mtpnc", Capacity: 1},
	}

	svc := &HTTPRestService{
		Service:     &cns.Service{Service: &common.Service{}},
		nodeName:    "node1",
		nodeInfoCli: &fakeNodeInfoClient{nodeInfo: nodeInfo},
		nicNCClient: &fakeNICNCClient{m: nicNC},
		mtpncCli:    &fakeMTPNCClient{m: mtpnc},
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, cns.GetNICResources, nil)
	svc.getNICResources(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var resp cns.GetNICResourcesResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	gotCap := map[string]int{}
	gotNet := map[string]string{}
	gotSubGUID := map[string]string{}
	gotSubName := map[string]string{}
	for _, nic := range resp.NICResources {
		gotCap[nic.MacAddress] = nic.Capacity
		gotNet[nic.MacAddress] = nic.NetworkID
		gotSubGUID[nic.MacAddress] = nic.SubnetGUID
		gotSubName[nic.MacAddress] = nic.SubnetName
	}
	wantCap := map[string]int{
		"aa:aa:aa:aa:aa:01": 16,
		"aa:aa:aa:aa:aa:02": 0,
		"aa:aa:aa:aa:aa:03": 1,  // MTPNC dedicated DRA
		"aa:aa:aa:aa:aa:04": 0,  // MTPNC dedicated non-DRA
		"aa:aa:aa:aa:aa:05": 0,  // no NICNetworkConfig/MTPNC → zero capacity
		"AA:AA:AA:AA:AA:07": 16, // NodeInfo reports uppercase; enriched via canonical NICNC key
		"aa:aa:aa:aa:aa:08": 16, // NICNetworkConfig wins over MTPNC
	}
	if len(resp.NICResources) != len(wantCap) {
		t.Fatalf("got %d NIC resources, want %d (%+v)", len(resp.NICResources), len(wantCap), gotCap)
	}
	for mac, want := range wantCap {
		if gotCap[mac] != want {
			t.Errorf("MAC %s capacity = %d, want %d", mac, gotCap[mac], want)
		}
	}
	if gotNet["aa:aa:aa:aa:aa:01"] != "net1" {
		t.Errorf("MAC 01 networkID = %q, want net1", gotNet["aa:aa:aa:aa:aa:01"])
	}
	if gotNet["aa:aa:aa:aa:aa:03"] != "net3" {
		t.Errorf("MAC 03 (MTPNC fallback) networkID = %q, want net3", gotNet["aa:aa:aa:aa:aa:03"])
	}
	if gotNet["aa:aa:aa:aa:aa:08"] != "net8" {
		t.Errorf("MAC 08 (NICNC wins over MTPNC) networkID = %q, want net8", gotNet["aa:aa:aa:aa:aa:08"])
	}
	if gotNet["AA:AA:AA:AA:AA:07"] != "net7" {
		t.Errorf("MAC 07 (uppercase, canonical match) networkID = %q, want net7", gotNet["AA:AA:AA:AA:AA:07"])
	}
	if gotSubGUID["aa:aa:aa:aa:aa:01"] != "guid1" || gotSubName["aa:aa:aa:aa:aa:01"] != "subnet1" {
		t.Errorf("MAC 01 subnet = (%q,%q), want (guid1,subnet1)", gotSubGUID["aa:aa:aa:aa:aa:01"], gotSubName["aa:aa:aa:aa:aa:01"])
	}
}
