package restserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/common"
	"github.com/Azure/azure-container-networking/cns/types"
	"go.uber.org/zap"
)

type fakeSwiftV2NICMiddleware struct{ macs []string }

func (f *fakeSwiftV2NICMiddleware) GetPodInfoForIPConfigsRequest(context.Context, *cns.IPConfigsRequest) (cns.PodInfo, types.ResponseCode, string) {
	return nil, types.Success, ""
}

func (f *fakeSwiftV2NICMiddleware) GetAllIPConfigs(context.Context, cns.PodInfo) ([]cns.PodIpInfo, error) {
	return nil, nil
}

func (f *fakeSwiftV2NICMiddleware) GetPodNICMACs(context.Context, cns.PodInfo) ([]string, error) {
	return f.macs, nil
}

// podNICResources enriches each of the pod's NICs from NICNetworkConfig, falling back to
// MTPNC for dedicated NICs; the pod's NIC MACs come from its MTPNC via the middleware.
func TestPodNICResources(t *testing.T) {
	svc := &HTTPRestService{
		Service: &cns.Service{Service: &common.Service{}},
		nicNCClient: &fakeNICNCClient{m: map[string]*cns.NICResourceSliceInfo{
			"aa:aa:aa:aa:aa:01": {NetworkID: "net1", SubnetGUID: "guid1", SubnetName: "subnet1", Capacity: 16},
		}},
		mtpncCli: &fakeMTPNCClient{m: map[string]*cns.NICResourceSliceInfo{
			"aa:aa:aa:aa:aa:02": {NetworkID: "net2", SubnetGUID: "guid2", SubnetName: "subnet2", Capacity: 1},
		}},
	}
	mw := &fakeSwiftV2NICMiddleware{macs: []string{
		"AA:AA:AA:AA:AA:01", // uppercase; matches canonical NICNC key → 16
		"aa:aa:aa:aa:aa:02", // dedicated NIC via MTPNC fallback → 1
		"aa:aa:aa:aa:aa:03", // no NICNetworkConfig/MTPNC → 0
	}}

	got, err := svc.podNICResources(context.Background(), zap.NewNop(), mw, nil)
	if err != nil {
		t.Fatalf("podNICResources: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d NICs, want 3: %+v", len(got), got)
	}

	gotCap := map[string]int{}
	gotNet := map[string]string{}
	gotSub := map[string]string{}
	for _, n := range got {
		gotCap[n.MacAddress] = n.Capacity
		gotNet[n.MacAddress] = n.NetworkID
		gotSub[n.MacAddress] = n.SubnetGUID
	}
	wantCap := map[string]int{
		"AA:AA:AA:AA:AA:01": 16, // NICNetworkConfig
		"aa:aa:aa:aa:aa:02": 1,  // MTPNC dedicated fallback
		"aa:aa:aa:aa:aa:03": 0,  // no NICNetworkConfig/MTPNC
	}
	for mac, want := range wantCap {
		if gotCap[mac] != want {
			t.Errorf("MAC %s capacity = %d, want %d", mac, gotCap[mac], want)
		}
	}
	if gotNet["AA:AA:AA:AA:AA:01"] != "net1" {
		t.Errorf("MAC 01 networkID = %q, want net1", gotNet["AA:AA:AA:AA:AA:01"])
	}
	if gotSub["AA:AA:AA:AA:AA:01"] != "guid1" {
		t.Errorf("MAC 01 subnetGUID = %q, want guid1", gotSub["AA:AA:AA:AA:AA:01"])
	}
	if gotNet["aa:aa:aa:aa:aa:02"] != "net2" {
		t.Errorf("MAC 02 (MTPNC fallback) networkID = %q, want net2", gotNet["aa:aa:aa:aa:aa:02"])
	}
}

// requestIPConfigsAndNICResources must reject non-POST verbs before doing any work.
func TestRequestIPConfigsAndNICResourcesMethodNotPost(t *testing.T) {
	svc := &HTTPRestService{Service: &cns.Service{Service: &common.Service{}}}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, cns.RequestIPConfigsAndNICResources, nil)
	svc.requestIPConfigsAndNICResources(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	var resp cns.IPConfigsAndNICResourcesResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Response.ReturnCode != types.UnsupportedVerb {
		t.Errorf("ReturnCode = %v, want %v", resp.Response.ReturnCode, types.UnsupportedVerb)
	}
}

// requestIPConfigsAndNICResources must return a clear error when the SWIFT v2 middleware
// is not configured, rather than panicking.
func TestRequestIPConfigsAndNICResourcesMiddlewareNotConfigured(t *testing.T) {
	svc := &HTTPRestService{Service: &cns.Service{Service: &common.Service{}}} // IPConfigsHandlerMiddleware nil

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, cns.RequestIPConfigsAndNICResources, strings.NewReader("{}"))
	svc.requestIPConfigsAndNICResources(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
	var resp cns.IPConfigsAndNICResourcesResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Response.ReturnCode != types.UnexpectedError {
		t.Errorf("ReturnCode = %v, want %v", resp.Response.ReturnCode, types.UnexpectedError)
	}
}
