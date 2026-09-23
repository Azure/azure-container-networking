package restserver

import (
	stderrors "errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/Azure/azure-container-networking/cns"
)

// IPAMUnixSocketPath is the path of the CNS IPAM Unix socket.
const IPAMUnixSocketPath = "/var/run/azure-cns/cns.sock"

const ipamReadHeaderTimeout = 5 * time.Second

type ipamServer struct {
	server   *http.Server
	listener net.Listener
}

func startIPAMServer(socketPath string, service *HTTPRestService, errChan chan<- error) (*ipamServer, error) {
	listener, err := listenIPAMSocket(socketPath)
	if err != nil {
		return nil, fmt.Errorf("starting CNS IPAM Unix listener: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc(cns.RequestIPConfig, service.RequestIPConfigHandler)
	mux.HandleFunc(cns.RequestIPConfigs, service.RequestIPConfigsHandler)
	mux.HandleFunc(cns.ReleaseIPConfig, service.ReleaseIPConfigHandler)
	mux.HandleFunc(cns.ReleaseIPConfigs, service.ReleaseIPConfigsHandler)
	server := &http.Server{Handler: mux, ReadHeaderTimeout: ipamReadHeaderTimeout}
	ipam := &ipamServer{server: server, listener: listener}

	go func() {
		if err := server.Serve(listener); err != nil && !stderrors.Is(err, http.ErrServerClosed) && !stderrors.Is(err, net.ErrClosed) {
			errChan <- fmt.Errorf("serving CNS IPAM Unix socket: %w", err)
		}
	}()

	return ipam, nil
}

func (s *ipamServer) stop() {
	_ = s.server.Close()
	// Close the listener directly because server.Close does not close it if Serve has not started yet.
	_ = s.listener.Close()
}
