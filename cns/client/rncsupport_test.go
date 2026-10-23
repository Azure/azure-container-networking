package client

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/stretchr/testify/require"
)

func TestGetRNCSupport(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		body      string
		wantCode  types.ResponseCode
		wantError bool
		supported bool
	}{
		{
			name:      "supported",
			status:    http.StatusOK,
			body:      `{"response":{"ReturnCode":0,"Message":"success"},"rncSupportResponse":{"isSupported":true}}`,
			supported: true,
		},
		{
			name:   "unsupported NMAgent",
			status: http.StatusOK,
			body:   `{"response":{"ReturnCode":0,"Message":"success"},"rncSupportResponse":{"isSupported":false}}`,
		},
		{
			name:      "older CNS without endpoint",
			status:    http.StatusNotFound,
			body:      "404 page not found",
			wantCode:  types.UnsupportedAPI,
			wantError: true,
		},
		{
			name:      "HTTP failure is not unsupported",
			status:    http.StatusInternalServerError,
			body:      "internal server error",
			wantCode:  types.UnexpectedError,
			wantError: true,
		},
		{
			name:      "empty cache is not unsupported",
			status:    http.StatusOK,
			body:      rncSupportErrorJSON(t, types.NotFound),
			wantCode:  types.NotFound,
			wantError: true,
		},
		{
			name:      "discovery failure is not unsupported",
			status:    http.StatusOK,
			body:      rncSupportErrorJSON(t, types.NmAgentSupportedApisError),
			wantCode:  types.NmAgentSupportedApisError,
			wantError: true,
		},
		{
			name:      "invalid JSON",
			status:    http.StatusOK,
			body:      "{",
			wantError: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != cns.GetRNCSupport || r.ContentLength != 0 {
					t.Errorf("unexpected request: %s %s, content length %d", r.Method, r.URL.Path, r.ContentLength)
				}
				w.WriteHeader(test.status)
				_, err := w.Write([]byte(test.body))
				if err != nil {
					t.Errorf("writing response: %v", err)
				}
			}))
			t.Cleanup(server.Close)
			client, err := New(server.URL, time.Second)
			require.NoError(t, err)
			got, err := client.GetRNCSupport(t.Context())
			if test.wantError {
				require.Error(t, err)
				require.Nil(t, got)
				if test.wantCode != types.Success {
					var cnsErr *CNSClientError
					require.ErrorAs(t, err, &cnsErr)
					require.Equal(t, test.wantCode, cnsErr.Code)
					require.Equal(t, test.wantCode == types.UnsupportedAPI, IsUnsupportedAPI(err))
				}
			} else {
				require.NoError(t, err)
				require.Equal(t, test.supported, got.RNCSupportResponse.IsSupported)
				require.Equal(t, types.Success, got.Response.ReturnCode)
			}
		})
	}
}

func rncSupportErrorJSON(t *testing.T, code types.ResponseCode) string {
	t.Helper()
	payload, err := json.Marshal(cns.GetRNCSupportResponse{
		Response: cns.Response{ReturnCode: code, Message: "discovery unavailable"},
	})
	require.NoError(t, err)
	return string(payload)
}

type rncSupportDoFunc func(*http.Request) (*http.Response, error)

func (f rncSupportDoFunc) Do(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestGetRNCSupportRequestErrors(t *testing.T) {
	t.Run("invalid request URL", func(t *testing.T) {
		client := &Client{
			routes: map[string]url.URL{cns.GetRNCSupport: {Scheme: "http", Host: "bad\nhost"}},
		}
		got, err := client.GetRNCSupport(t.Context())
		require.ErrorContains(t, err, "building http request")
		require.Nil(t, got)
	})

	t.Run("transport failure", func(t *testing.T) {
		cause := errors.New("transport unavailable")
		routes, err := buildRoutes(defaultBaseURL, clientPaths)
		require.NoError(t, err)
		client := &Client{
			routes: routes,
			client: rncSupportDoFunc(func(r *http.Request) (*http.Response, error) {
				require.Equal(t, t.Context(), r.Context())
				return nil, cause
			}),
		}
		got, err := client.GetRNCSupport(t.Context())
		require.ErrorIs(t, err, cause)
		require.Nil(t, got)
	})
}
