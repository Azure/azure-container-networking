//go:build linux

package main

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	cniTypes "github.com/containernetworking/cni/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

const testCNIVersion = "1.0.0"

func TestPluginUnixSocketAddAndDel(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "cns.sock")
	var requestCount, releaseCount atomic.Int32

	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(t.Context(), "unix", socketPath)
	require.NoError(t, err)

	server := &http.Server{
		ReadHeaderTimeout: time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch r.URL.Path {
			case cns.RequestIPConfigs:
				requestCount.Add(1)
				_, _ = io.WriteString(w, `{"PodIPInfo":[{"PodIPConfig":{"IPAddress":"10.0.2.10","PrefixLength":24},`+
					`"NetworkContainerPrimaryIPConfig":{"IPSubnet":{"IPAddress":"10.0.2.0","PrefixLength":24},`+
					`"GatewayIPAddress":"10.0.2.1"},"MacAddress":"00:11:22:33:44:66"}],"Response":{"ReturnCode":0}}`)
			case cns.ReleaseIPConfigs:
				releaseCount.Add(1)
				_, _ = io.WriteString(w, `{"Response":{"ReturnCode":0}}`)
			default:
				http.NotFound(w, r)
			}
		}),
	}
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(func() {
		_ = server.Close()
	})

	stdin, err := json.Marshal(netConf{
		NetConf: cniTypes.NetConf{
			CNIVersion: testCNIVersion,
			Name:       "cilium",
		},
		CNSSocketPath: socketPath,
	})
	require.NoError(t, err)
	args := buildArgs("unixContainer", happyPodArgs, stdin)

	writer := &cniResultsWriter{}
	plugin, err := NewPlugin(zap.NewNop(), nil, writer)
	require.NoError(t, err)

	require.NoError(t, plugin.CmdAdd(args))
	require.EqualValues(t, 1, requestCount.Load())
	require.NotNil(t, writer.result)
	require.Len(t, writer.result.IPs, 1)

	require.NoError(t, plugin.CmdDel(args))
	require.EqualValues(t, 1, releaseCount.Load())
}
