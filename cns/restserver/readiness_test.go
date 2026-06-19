package restserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/types"
)

func TestNetworkReadinessResponse(t *testing.T) {
	tests := []struct {
		name       string
		service    *HTTPRestService
		wantState  cns.NetworkReadinessState
		wantReason cns.NetworkReadinessReason
	}{
		{
			name: "ready when no readiness gates are required",
			service: &HTTPRestService{
				state: &httpRestServiceState{},
				networkReadiness: networkReadiness{
					cniConflistWritten: true,
				},
			},
			wantState:  cns.NetworkReadinessStateReady,
			wantReason: cns.NetworkReadinessReasonReady,
		},
		{
			name: "not ready when nnc is required and no nc state exists",
			service: &HTTPRestService{
				state: &httpRestServiceState{},
				networkReadiness: networkReadiness{
					requiresNNC:        true,
					requiresIPAMReady:  true,
					cniConflistWritten: true,
				},
			},
			wantState:  cns.NetworkReadinessStateNotReady,
			wantReason: cns.NetworkReadinessReasonNNCNotReceived,
		},
		{
			name: "not ready when nc programming is required and no nc is programmed",
			service: &HTTPRestService{
				state: &httpRestServiceState{
					ContainerStatus: map[string]containerstatus{
						"nc-1": {
							ID:          "nc-1",
							HostVersion: "-1",
						},
					},
				},
				networkReadiness: networkReadiness{
					requiresNNC:          true,
					requiresNCProgrammed: true,
					requiresIPAMReady:    true,
					cniConflistWritten:   true,
				},
			},
			wantState:  cns.NetworkReadinessStateNotReady,
			wantReason: cns.NetworkReadinessReasonNCNotProgrammed,
		},
		{
			name: "not ready when cni conflist write is required and pending",
			service: &HTTPRestService{
				state: &httpRestServiceState{
					ContainerStatus: map[string]containerstatus{
						"nc-1": {
							ID:          "nc-1",
							HostVersion: "0",
						},
					},
				},
				networkReadiness: networkReadiness{
					requiresNNC:          true,
					requiresNCProgrammed: true,
					requiresIPAMReady:    true,
					requiresCNIConflist:  true,
					ipamReady:            true,
					ncProgrammed:         true,
				},
			},
			wantState:  cns.NetworkReadinessStateNotReady,
			wantReason: cns.NetworkReadinessReasonConflistNotWritten,
		},
		{
			name: "ready when required crd gates are satisfied",
			service: &HTTPRestService{
				state: &httpRestServiceState{
					ContainerStatus: map[string]containerstatus{
						"nc-1": {
							ID:          "nc-1",
							HostVersion: "0",
						},
					},
				},
				networkReadiness: networkReadiness{
					requiresNNC:          true,
					requiresNCProgrammed: true,
					requiresIPAMReady:    true,
					requiresCNIConflist:  true,
					ipamReady:            true,
					ncProgrammed:         true,
					cniConflistWritten:   true,
				},
			},
			wantState:  cns.NetworkReadinessStateReady,
			wantReason: cns.NetworkReadinessReasonReady,
		},
		{
			name: "ready for node subnet after ipam is ready without nc programming",
			service: &HTTPRestService{
				state: &httpRestServiceState{},
				networkReadiness: networkReadiness{
					requiresIPAMReady:   true,
					requiresCNIConflist: true,
					ipamReady:           true,
					cniConflistWritten:  true,
				},
			},
			wantState:  cns.NetworkReadinessStateReady,
			wantReason: cns.NetworkReadinessReasonReady,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := tt.service.networkReadinessResponse()
			if resp.State != tt.wantState {
				t.Fatalf("unexpected state: got %s, want %s", resp.State, tt.wantState)
			}
			if resp.Reason != tt.wantReason {
				t.Fatalf("unexpected reason: got %s, want %s", resp.Reason, tt.wantReason)
			}
		})
	}
}

func TestGetNetworkReadiness(t *testing.T) {
	tests := []struct {
		name       string
		service    *HTTPRestService
		wantStatus int
		wantCode   types.ResponseCode
	}{
		{
			name: "ready",
			service: &HTTPRestService{
				state: &httpRestServiceState{},
				networkReadiness: networkReadiness{
					cniConflistWritten: true,
				},
			},
			wantStatus: http.StatusOK,
			wantCode:   types.Success,
		},
		{
			name: "not ready",
			service: &HTTPRestService{
				state: &httpRestServiceState{},
				networkReadiness: networkReadiness{
					requiresNNC:        true,
					requiresIPAMReady:  true,
					cniConflistWritten: true,
				},
			},
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   types.NetworkNotReady,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, cns.GetNetworkReadinessPath, http.NoBody)
			rec := httptest.NewRecorder()

			tt.service.getNetworkReadiness(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("unexpected status: got %d, want %d", rec.Code, tt.wantStatus)
			}

			var resp cns.NetworkReadinessResponse
			if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
				t.Fatalf("decoding response: %v", err)
			}
			if resp.Response.ReturnCode != tt.wantCode {
				t.Fatalf("unexpected return code: got %d, want %d", resp.Response.ReturnCode, tt.wantCode)
			}
		})
	}
}
