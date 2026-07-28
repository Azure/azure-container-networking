package restserver

import (
	"context"
	"encoding/json"
	"net"
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

type fakeSwiftV2NICMiddleware struct{ macs []string }

func (f *fakeSwiftV2NICMiddleware) GetPodInfoByClaimUID(context.Context, k8stypes.UID) (cns.PodInfo, types.ResponseCode, string) {
	return nil, types.Success, ""
}

func (f *fakeSwiftV2NICMiddleware) GetSwiftV2IPConfigs(context.Context, cns.PodInfo) ([]cns.PodIpInfo, error) {
	return nil, nil
}

func (f *fakeSwiftV2NICMiddleware) GetPodNICMACs(context.Context, cns.PodInfo) ([]string, error) {
	return f.macs, nil
}

// podNICResources enriches each of the pod's NICs from NICNetworkConfig, falling back to
// MTPNC for dedicated NICs; the pod's NIC MACs come from its MTPNC via the middleware.
func TestPodNICResources(t *testing.T) {
	tests := []struct {
		name     string
		mac      string // MAC the middleware returns for the pod (from its MTPNC)
		nicnc    *cns.NICResourceInfo
		mtpnc    *cns.NICResourceInfo
		wantCap  string
		wantNet  string
		wantGUID string
	}{
		{
			name:     "NICNetworkConfig NIC, uppercase MAC matches canonical key",
			mac:      "AA:BB:CC:DD:EE:01",
			nicnc:    &cns.NICResourceInfo{NetworkID: "nicnc-net", SubnetGUID: "nicnc-guid", Capacity: 16},
			wantCap:  "16",
			wantNet:  "nicnc-net",
			wantGUID: "nicnc-guid",
		},
		{
			name:     "MTPNC dedicated NIC fallback",
			mac:      "aa:bb:cc:dd:ee:02",
			mtpnc:    &cns.NICResourceInfo{NetworkID: "mtpnc-net", SubnetGUID: "mtpnc-guid", Capacity: 1},
			wantCap:  "1",
			wantNet:  "mtpnc-net",
			wantGUID: "mtpnc-guid",
		},
		{
			name:    "free NIC in neither advertises placeholder capacity",
			mac:     "aa:bb:cc:dd:ee:03",
			wantCap: "1",
		},
	}

	nicNC := map[string]*cns.NICResourceInfo{}
	mtpnc := map[string]*cns.NICResourceInfo{}
	macs := make([]string, 0, len(tests))
	for _, tc := range tests {
		macs = append(macs, tc.mac)
		hw, err := net.ParseMAC(tc.mac)
		if err != nil {
			t.Fatalf("invalid test MAC %q: %v", tc.mac, err)
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
		Service:     &cns.Service{Service: &common.Service{}},
		nicncClient: &fakeNICNCClient{m: nicNC},
		mtpncClient: &fakeMTPNCClient{m: mtpnc},
	}
	mw := &fakeSwiftV2NICMiddleware{macs: macs}

	gotNICs, err := svc.podNICResources(context.Background(), zap.NewNop(), mw, nil)
	if err != nil {
		t.Fatalf("podNICResources: %v", err)
	}
	if len(gotNICs) != len(tests) {
		t.Fatalf("got %d NICs, want %d: %+v", len(gotNICs), len(tests), gotNICs)
	}
	got := make(map[string]cns.NICResource, len(gotNICs))
	for _, n := range gotNICs {
		got[n.MacAddress] = n
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			n, ok := got[tc.mac]
			if !ok {
				t.Fatalf("MAC %s missing", tc.mac)
			}
			if n.Capacity != tc.wantCap {
				t.Errorf("capacity = %s, want %s", n.Capacity, tc.wantCap)
			}
			if n.NetworkID != tc.wantNet {
				t.Errorf("networkID = %q, want %q", n.NetworkID, tc.wantNet)
			}
			if n.SubnetGUID != tc.wantGUID {
				t.Errorf("subnetGUID = %q, want %q", n.SubnetGUID, tc.wantGUID)
			}
		})
	}
}

// requestClaimResourceInfo must reject non-POST verbs before doing any work.
func TestRequestClaimResourceInfoMethodNotPost(t *testing.T) {
	svc := &HTTPRestService{
		Service:        &cns.Service{Service: &common.Service{}},
		nicncClient:    &fakeNICNCClient{},
		mtpncClient:    &fakeMTPNCClient{},
		nodeinfoClient: &fakeNodeInfoClient{},
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, cns.RequestClaimResourceInfo, http.NoBody)
	svc.requestClaimResourceInfo(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	var resp cns.ClaimResourceInfoResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil { //nolint:musttag // response embeds pre-existing PodIpInfo type
		t.Fatalf("decode: %v", err)
	}
	if resp.Response.ReturnCode != types.UnsupportedVerb {
		t.Errorf("ReturnCode = %v, want %v", resp.Response.ReturnCode, types.UnsupportedVerb)
	}
}

// requestClaimResourceInfo must return a clear error when the SWIFT v2 middleware
// is not configured, rather than panicking.
func TestRequestClaimResourceInfoMiddlewareNotConfigured(t *testing.T) {
	svc := &HTTPRestService{
		Service:        &cns.Service{Service: &common.Service{}},
		nicncClient:    &fakeNICNCClient{},
		mtpncClient:    &fakeMTPNCClient{},
		nodeinfoClient: &fakeNodeInfoClient{},
	} // IPConfigsHandlerMiddleware nil

	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(context.Background(), http.MethodPost, cns.RequestClaimResourceInfo, strings.NewReader("{}"))
	svc.requestClaimResourceInfo(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
	var resp cns.ClaimResourceInfoResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil { //nolint:musttag // response embeds pre-existing PodIpInfo type
		t.Fatalf("decode: %v", err)
	}
	if resp.Response.ReturnCode != types.UnexpectedError {
		t.Errorf("ReturnCode = %v, want %v", resp.Response.ReturnCode, types.UnexpectedError)
	}
}

// requestClaimResourceInfo must return 404 UnsupportedAPI when SwiftV2 prefix
// allocation is disabled (its clients are not wired), before any other processing.
func TestRequestClaimResourceInfoFeatureDisabled(t *testing.T) {
	svc := &HTTPRestService{Service: &cns.Service{Service: &common.Service{}}} // no NIC-resources clients wired

	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(context.Background(), http.MethodPost, cns.RequestClaimResourceInfo, strings.NewReader("{}"))
	svc.requestClaimResourceInfo(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
	var resp cns.ClaimResourceInfoResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil { //nolint:musttag // response embeds pre-existing PodIpInfo type
		t.Fatalf("decode: %v", err)
	}
	if resp.Response.ReturnCode != types.UnsupportedAPI {
		t.Errorf("ReturnCode = %v, want %v", resp.Response.ReturnCode, types.UnsupportedAPI)
	}
}
