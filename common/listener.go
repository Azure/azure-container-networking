// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package common

import (
	"crypto/tls"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"os"

	"github.com/Azure/azure-container-networking/log"
	"github.com/pkg/errors"
)

// Listener represents an HTTP listener.
type Listener struct {
	URL          *url.URL
	protocol     string
	localAddress string
	endpoints    []string
	active       bool
	listener     net.Listener
	tlsListener  net.Listener
	mux          *http.ServeMux
}

// NewListener creates a new Listener.
func NewListener(u *url.URL) (*Listener, error) {
	listener := Listener{
		URL:          u,
		protocol:     u.Scheme,
		localAddress: u.Host + u.Path,
	}

	listener.mux = http.NewServeMux()

	return &listener, nil
}

// StartTLS creates the listener socket and starts the HTTPS server.
func (l *Listener) StartTLS(errChan chan<- error, tlsConfig *tls.Config, address string) error {
	server := http.Server{
		TLSConfig: tlsConfig,
		Handler:   l.mux,
	}

	// listen on a separate endpoint for secure tls connections
	list, err := net.Listen(l.protocol, address)
	if err != nil {
		log.Printf("[Listener] Failed to listen on TlsEndpoint: %+v", err)
		return err
	}

	l.tlsListener = list
	log.Printf("[Listener] Started listening on tls endpoint %s.", address)

	// Launch goroutine for servicing https requests
	go func() {
		errChan <- server.ServeTLS(l.tlsListener, "", "")
	}()

	l.active = true
	return nil
}

// Start creates the listener socket and starts the HTTP server.
func (l *Listener) Start(errChan chan<- error) error {
	list, err := net.Listen(l.protocol, l.localAddress)
	if err != nil {
		log.Printf("[Listener] Failed to listen: %+v", err)
		return err
	}

	l.listener = list
	log.Printf("[Listener] Started listening on %s.", l.localAddress)

	// Launch goroutine for servicing requests.
	go func() {
		errChan <- http.Serve(l.listener, l.mux)
	}()

	l.active = true
	return nil
}

// Stop stops listening for requests.
func (l *Listener) Stop() {
	// Ignore if not active.
	if !l.active {
		return
	}
	l.active = false

	// Stop servicing requests.
	_ = l.listener.Close()

	if l.tlsListener != nil {
		// Stop servicing requests on secure listener
		_ = l.tlsListener.Close()
	}

	// Delete the unix socket.
	if l.protocol == "unix" {
		_ = os.Remove(l.localAddress)
	}

	log.Printf("[Listener] Stopped listening on %s", l.localAddress)
}

// GetMux returns the HTTP mux for the listener.
func (l *Listener) GetMux() *http.ServeMux {
	return l.mux
}

// GetEndpoints returns the list of registered protocol endpoints.
func (l *Listener) GetEndpoints() []string {
	return l.endpoints
}

// AddEndpoint registers a protocol endpoint.
func (l *Listener) AddEndpoint(endpoint string) {
	l.endpoints = append(l.endpoints, endpoint)
}

// AddHandler registers a protocol handler.
func (l *Listener) AddHandler(path string, handler http.HandlerFunc) {
	l.mux.HandleFunc(path, handler)
}

// todo: Decode and Encode below should not be methods, just functions. They make no use of Listener fields.

// Decode receives and decodes JSON payload to a request.
func (l *Listener) Decode(w http.ResponseWriter, r *http.Request, request interface{}) error {
	var err error
	if r.Body == nil {
		err = errors.New("request body is empty")
	} else {
		err = json.NewDecoder(r.Body).Decode(request)
	}

	if err != nil {
		http.Error(w, "Failed to decode request: "+err.Error(), http.StatusBadRequest)
		log.Printf("[Listener] Failed to decode request: %v\n", err.Error())
	}
	return err
}

// Encode encodes and sends a response as JSON payload.
func (l *Listener) Encode(w http.ResponseWriter, response interface{}) error {
	// Set the content type as application json
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	err := json.NewEncoder(w).Encode(response)
	if err != nil {
		http.Error(w, "Failed to encode response: "+err.Error(), http.StatusInternalServerError)
		log.Printf("[Listener] Failed to encode response: %v\n", err.Error())
	}
	return err
}
