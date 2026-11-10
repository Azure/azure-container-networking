package restserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/common"
	"github.com/Azure/azure-container-networking/cns/fakes"
	"github.com/Azure/azure-container-networking/cns/types"
	acn "github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/nmagent"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestGetRNCSupport(t *testing.T) {
	var apis []string
	var queryErr error
	apiCalls := 0
	monitor := NewNMAgentMonitor(&fakes.NMAgentClientFake{
		SupportedAPIsF: func(context.Context) ([]string, error) {
			apiCalls++
			return apis, queryErr
		},
		GetHomeAzF: func(context.Context) (nmagent.AzResponse, error) {
			return nmagent.AzResponse{}, errors.New("home az unavailable")
		},
	}, time.Minute)
	config := common.ServiceConfig{ChannelMode: cns.Direct}
	service, err := NewHTTPRestService(&config, &fakes.WireserverClientFake{},
		&fakes.WireserverProxyFake{}, &IPtablesProvider{}, &fakes.NMAgentClientFake{},
		nil, nil, monitor, fakes.NewMockIMDSClient())
	require.NoError(t, err)
	service.Options[acn.OptCnsURL] = "tcp://127.0.0.1:0"
	service.Options[acn.OptCnsPort] = "0"
	require.NoError(t, service.Init(&config))

	tests := []struct {
		name      string
		populate  bool
		apis      []string
		err       error
		code      types.ResponseCode
		supported bool
	}{
		{name: "empty cache", code: types.NotFound},
		{name: "supported", populate: true, apis: []string{RNCSupportAPIName}, supported: true},
		{name: "discovery failed after support", populate: true, err: errors.New("query failed"), code: types.NmAgentSupportedApisError},
		{name: "unsupported after recovery", populate: true},
		{name: "discovery failed after unsupported", populate: true, err: errors.New("query failed"), code: types.NmAgentSupportedApisError},
		{name: "RNC supported despite HomeAZ error", populate: true, apis: []string{RNCSupportAPIName, GetHomeAzAPIName}, supported: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.populate {
				apis, queryErr = test.apis, test.err
				monitor.Populate(t.Context())
			}
			callsBeforeRead := apiCalls
			for _, path := range []string{cns.GetRNCSupport, cns.V2Prefix + cns.GetRNCSupport} {
				recorder := httptest.NewRecorder()
				req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, http.NoBody)
				service.Listener.GetMux().ServeHTTP(recorder, req)
				require.Equal(t, http.StatusOK, recorder.Code)

				var got cns.GetRNCSupportResponse
				require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &got))
				require.Equal(t, test.code, got.Response.ReturnCode)
				require.Equal(t, test.supported, got.RNCSupportResponse.IsSupported)
				require.Equal(t, monitor.GetRNCSupport(t.Context()), got)
				if test.supported {
					require.JSONEq(t, `{
						"response": {"ReturnCode": 0, "Message": "RNC support query succeeded"},
						"rncSupportResponse": {"isSupported": true}
					}`, recorder.Body.String())
				}
			}
			require.Equal(t, callsBeforeRead, apiCalls, "HTTP reads must not query NMAgent")
		})
	}

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			for _, path := range []string{cns.GetRNCSupport, cns.V2Prefix + cns.GetRNCSupport} {
				recorder := httptest.NewRecorder()
				service.Listener.GetMux().ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), method, path, http.NoBody))
				require.Equal(t, http.StatusOK, recorder.Code)
				var got cns.GetRNCSupportResponse
				require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &got))
				require.Equal(t, types.UnsupportedVerb, got.Response.ReturnCode)
			}
		})
	}
}

func TestGetRNCSupportNotRegisteredOutsideDirectMode(t *testing.T) {
	config := common.ServiceConfig{ChannelMode: cns.CRD}
	service, err := NewHTTPRestService(&config, &fakes.WireserverClientFake{},
		&fakes.WireserverProxyFake{}, &IPtablesProvider{}, &fakes.NMAgentClientFake{},
		nil, nil, nil, fakes.NewMockIMDSClient())
	require.NoError(t, err)
	service.Options[acn.OptCnsURL] = "tcp://127.0.0.1:0"
	service.Options[acn.OptCnsPort] = "0"
	require.NoError(t, service.Init(&config))
	for _, path := range []string{cns.GetRNCSupport, cns.V2Prefix + cns.GetRNCSupport} {
		recorder := httptest.NewRecorder()
		service.Listener.GetMux().ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, http.NoBody))
		require.Equal(t, http.StatusNotFound, recorder.Code)
	}
}
