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

func (f *fakeSwiftV2NICMiddleware) GetAllIPConfigs(context.Context, cns.PodInfo) ([]cns.PodIpInfo, error) {
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
		nicnc    *cns.NICResourceNetworkInfo
		mtpnc    *cns.NICResourceNetworkInfo
		wantCap  string
		wantNet  string
		wantGUID string
	}{
		{
			name:     "NICNetworkConfig NIC, uppercase MAC matches canonical key",
			mac:      "AA:BB:CC:DD:EE:01",
			nicnc:    &cns.NICResourceNetworkInfo{NetworkID: "nicnc-net", SubnetGUID: "nicnc-guid", Capacity: 16},
			wantCap:  "16",
			wantNet:  "nicnc-net",
			wantGUID: "nicnc-guid",
		},
		{
			name:     "MTPNC dedicated NIC fallback",
			mac:      "aa:bb:cc:dd:ee:02",
			mtpnc:    &cns.NICResourceNetworkInfo{NetworkID: "mtpnc-net", SubnetGUID: "mtpnc-guid", Capacity: 1},
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

	nicNC := map[string]*cns.NICResourceNetworkInfo{}
	mtpnc := map[string]*cns.NICResourceNetworkInfo{}
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
