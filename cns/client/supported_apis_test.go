package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSupportedAPIs(t *testing.T) {
	for _, test := range []struct {
		name    string
		status  int
		body    string
		want    []string
		wantErr bool
	}{
		{name: "capability names", status: http.StatusOK, body: `{"supportedApis":["exampleCapability"]}`, want: []string{"exampleCapability"}},
		{name: "no capabilities", status: http.StatusOK, body: `{"supportedApis":[]}`, want: []string{}},
		{name: "older CNS", status: http.StatusNotFound, body: "not found", wantErr: true},
		{name: "server error", status: http.StatusInternalServerError, body: "server error", wantErr: true},
		{name: "invalid JSON", status: http.StatusOK, body: "{", wantErr: true},
		{name: "missing API list", status: http.StatusOK, body: "{}", wantErr: true},
		{name: "null API list", status: http.StatusOK, body: `{"supportedApis":null}`, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, cns.SupportedAPIsPath, r.URL.Path)
				body, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				assert.Empty(t, body)
				w.WriteHeader(test.status)
				_, err = io.WriteString(w, test.body)
				assert.NoError(t, err)
			}))
			defer server.Close()
			client, err := New(server.URL, time.Second)
			require.NoError(t, err)
			got, err := client.SupportedAPIs(t.Context())
			if test.wantErr {
				require.Error(t, err)
				require.Nil(t, got)
				if test.status != http.StatusOK {
					var httpErr *FailedHTTPRequest
					require.ErrorAs(t, err, &httpErr)
					require.Equal(t, test.status, httpErr.Code)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
}

func TestSupportedAPIsCancellation(t *testing.T) {
	client, err := New(testBaseURL, time.Second)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	capabilities, err := client.SupportedAPIs(ctx)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, capabilities)
}

func TestSupportedAPIsIntegration(t *testing.T) {
	client, err := New(testBaseURL, time.Second)
	require.NoError(t, err)
	capabilities, err := client.SupportedAPIs(t.Context())
	require.NoError(t, err)
	require.Equal(t, []string{}, capabilities)
}
