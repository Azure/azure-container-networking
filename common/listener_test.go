package common

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestListenerTLSOnly(t *testing.T) {
	certificateServer := httptest.NewTLSServer(http.NotFoundHandler())
	tlsConfig := certificateServer.TLS.Clone()
	client := certificateServer.Client()
	client.Timeout = 5 * time.Second
	t.Cleanup(client.CloseIdleConnections)
	certificateServer.Close()

	address, err := url.Parse("tcp://127.0.0.1:0")
	require.NoError(t, err)
	listener, err := NewListener(address)
	require.NoError(t, err)
	listener.AddHandler("/test", func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusNoContent)
	})
	errChan := make(chan error, 1)
	require.NoError(t, listener.StartTLS(errChan, tlsConfig, "127.0.0.1:0"))
	t.Cleanup(listener.Stop)
	require.Nil(t, listener.listener)
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://"+listener.tlsListener.Addr().String()+"/test", http.NoBody)
	require.NoError(t, err)
	response, err := client.Do(request)
	require.NoError(t, err)
	_, err = io.Copy(io.Discard, response.Body)
	require.NoError(t, response.Body.Close())
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, response.StatusCode)
	require.NotPanics(t, listener.Stop)
	select {
	case err := <-errChan:
		require.ErrorIs(t, err, net.ErrClosed)
	case <-time.After(5 * time.Second):
		t.Fatal("TLS server did not stop")
	}
	require.NotPanics(t, listener.Stop)
}
