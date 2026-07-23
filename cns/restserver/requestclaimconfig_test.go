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
	k8stypes "k8s.io/apimachinery/pkg/types"
)

const testMAC01Upper = "AA:AA:AA:AA:AA:01"

type fakeSwiftV2NICMiddleware struct{ macs []string }

func (f *fakeSwiftV2NICMiddleware) GetPodInfoByClaimUID(context.Context, k8stypes.UID) (cns.PodInfo, types.ResponseCode, string) {
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
		nicncClient: &fakeNICNCClient{m: map[string]*cns.NICResourceSliceInfo{
			testMAC01: {NetworkID: testNet1, SubnetGUID: testGUID1, SubnetName: testSubnet1, Capacity: 16},
		}},
		mtpncClient: &fakeMTPNCClient{m: map[string]*cns.NICResourceSliceInfo{
			testMAC02: {NetworkID: testNet2, SubnetGUID: "guid2", SubnetName: "subnet2", Capacity: 1},
		}},
	}
	mw := &fakeSwiftV2NICMiddleware{macs: []string{
		testMAC01Upper, // uppercase; matches canonical NICNC key → 16
		testMAC02,      // dedicated NIC via MTPNC fallback → 1
		testMAC03,      // no NICNetworkConfig/MTPNC → 0
	}}

	got, err := svc.podNICResources(context.Background(), zap.NewNop(), mw, nil)
	if err != nil {
		t.Fatalf("podNICResources: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d NICs, want 3: %+v", len(got), got)
	}

	gotCap := map[string]string{}
	gotNet := map[string]string{}
	gotSub := map[string]string{}
	for _, n := range got {
		gotCap[n.MacAddress] = n.Capacity
		gotNet[n.MacAddress] = n.NetworkID
		gotSub[n.MacAddress] = n.SubnetGUID
	}
	wantCap := map[string]string{
		testMAC01Upper: "16", // NICNetworkConfig
		testMAC02:      "1",  // MTPNC dedicated fallback
		testMAC03:      "0",  // no NICNetworkConfig/MTPNC
	}
	for mac, want := range wantCap {
		if gotCap[mac] != want {
			t.Errorf("MAC %s capacity = %s, want %s", mac, gotCap[mac], want)
		}
	}
	if gotNet[testMAC01Upper] != testNet1 {
		t.Errorf("MAC 01 networkID = %q, want %s", gotNet[testMAC01Upper], testNet1)
	}
	if gotSub[testMAC01Upper] != testGUID1 {
		t.Errorf("MAC 01 subnetGUID = %q, want %s", gotSub[testMAC01Upper], testGUID1)
	}
	if gotNet[testMAC02] != testNet2 {
		t.Errorf("MAC 02 (MTPNC fallback) networkID = %q, want %s", gotNet[testMAC02], testNet2)
	}
}

// requestClaimConfig must reject non-POST verbs before doing any work.
func TestRequestClaimConfigMethodNotPost(t *testing.T) {
	svc := &HTTPRestService{Service: &cns.Service{Service: &common.Service{}}}

	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, cns.RequestClaimConfig, http.NoBody)
	svc.requestClaimConfig(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	var resp cns.ClaimConfigResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil { //nolint:musttag // response embeds pre-existing PodIpInfo type
		t.Fatalf("decode: %v", err)
	}
	if resp.Response.ReturnCode != types.UnsupportedVerb {
		t.Errorf("ReturnCode = %v, want %v", resp.Response.ReturnCode, types.UnsupportedVerb)
	}
}

// requestClaimConfig must return a clear error when the SWIFT v2 middleware
// is not configured, rather than panicking.
func TestRequestClaimConfigMiddlewareNotConfigured(t *testing.T) {
	svc := &HTTPRestService{Service: &cns.Service{Service: &common.Service{}}} // IPConfigsHandlerMiddleware nil

	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(context.Background(), http.MethodPost, cns.RequestClaimConfig, strings.NewReader("{}"))
	svc.requestClaimConfig(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
	var resp cns.ClaimConfigResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil { //nolint:musttag // response embeds pre-existing PodIpInfo type
		t.Fatalf("decode: %v", err)
	}
	if resp.Response.ReturnCode != types.UnexpectedError {
		t.Errorf("ReturnCode = %v, want %v", resp.Response.ReturnCode, types.UnexpectedError)
	}
}
