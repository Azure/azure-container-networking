package restserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/common"
	"github.com/Azure/azure-container-networking/cns/fakes"
	acn "github.com/Azure/azure-container-networking/common"
	"github.com/stretchr/testify/require"
)

func TestSupportedAPIs(t *testing.T) {
	const listenerURL = "tcp://127.0.0.1:0"

	for _, mode := range []string{cns.Direct, cns.Managed, cns.CRD, cns.MultiTenantCRD, cns.AzureHost} {
		for _, ipamSocket := range []bool{false, true} {
			t.Run(mode+"/ipamSocket="+strconv.FormatBool(ipamSocket), func(t *testing.T) {
				config := common.ServiceConfig{ChannelMode: mode, IPAMSocket: ipamSocket}
				service, err := NewHTTPRestService(&config, &fakes.WireserverClientFake{},
					&fakes.WireserverProxyFake{}, &IPtablesProvider{}, &fakes.NMAgentClientFake{},
					nil, nil, nil, fakes.NewMockIMDSClient())
				require.NoError(t, err)
				service.Options[acn.OptCnsURL] = listenerURL
				service.Options[acn.OptCnsPort] = "0"
				require.NoError(t, service.Init(&config))
				service.RegisterPProfEndpoints()
				service.nma = nil

				for _, path := range []string{cns.SupportedAPIsPath, cns.V2Prefix + cns.SupportedAPIsPath} {
					recorder := httptest.NewRecorder()
					service.Listener.GetMux().ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, http.NoBody))
					require.Equal(t, http.StatusOK, recorder.Code)
					require.Equal(t, "application/json; charset=UTF-8", recorder.Header().Get("Content-Type"))
					require.JSONEq(t, `{"supportedApis":[]}`, recorder.Body.String())
					var response cns.SupportedAPIsResponse
					require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
					require.Equal(t, []string{}, response.SupportedAPIs)

					for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions} {
						recorder := httptest.NewRecorder()
						service.Listener.GetMux().ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), method, path, http.NoBody))
						require.Equal(t, http.StatusMethodNotAllowed, recorder.Code)
						require.Equal(t, http.MethodGet, recorder.Header().Get("Allow"))
					}
				}
			})
		}
	}
}
