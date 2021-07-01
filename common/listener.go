// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package common

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"

	"github.com/Azure/azure-container-networking/log"
	localtls "github.com/Azure/azure-container-networking/server/tls"
)

// Listener represents an HTTP listener.
type Listener struct {
	URL            *url.URL
	protocol       string
	localAddress   string
	endpoints      []string
	active         bool
	l              net.Listener
	securelistener net.Listener
	mux            *http.ServeMux
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

func GetTlsConfig(tlsSettings localtls.TlsSettings) (*tls.Config, error) {
	tlsCertRetriever, err := localtls.GetTlsCertificateRetriever(tlsSettings)
	if err != nil {
		return nil, fmt.Errorf("Failed to get certificate retriever %+v", err)
	}
	leafCertificate, err := tlsCertRetriever.GetCertificate()
	if err != nil {
		return nil, fmt.Errorf("Failed to get certificate %+v", err)
	}
	if leafCertificate == nil {
		return nil, fmt.Errorf("Certificate retrival returned empty %+v", err)
	}
	privateKey, err := tlsCertRetriever.GetPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("Failed to get certificate private key %+v", err)
	}
	tlsCert := tls.Certificate{
		Certificate: [][]byte{leafCertificate.Raw},
		PrivateKey:  privateKey,
		Leaf:        leafCertificate,
	}
	tlsConfig := &tls.Config{
		MaxVersion: tls.VersionTLS12,
		MinVersion: tls.VersionTLS12,
		Certificates: []tls.Certificate{
			tlsCert,
		},
	}
	return tlsConfig, nil
}

// Start creates the listener socket and starts the HTTPS server.
func (listener *Listener) StartTLS(errChan chan<- error, tlsSettings localtls.TlsSettings) error {
	tlsConfig, err := GetTlsConfig(tlsSettings)
	if err != nil {
		log.Printf("[Listener] Failed to compose Tls Configuration with errror: %+v", err)
		return err
	}
	server := http.Server{
		TLSConfig: tlsConfig,
		Handler:   listener.mux,
	}

	// listen on a seperate endpoint for secure tls connections
	listener.securelistener, err = net.Listen(listener.protocol, tlsSettings.TLSEndpoint)
	if err != nil {
		log.Printf("[Listener] Failed to listen on TlsEndpoint: %+v", err)
		return err
	}
	log.Printf("[Listener] Started listening on tls endpoint %s.", tlsSettings.TLSEndpoint)

	// Launch goroutine for servicing https requests
	go func() {
		errChan <- server.ServeTLS(listener.securelistener, "", "")
	}()

	listener.active = true
	return nil
}

// Start creates the listener socket and starts the HTTP server.
func (listener *Listener) Start(errChan chan<- error) error {
	var err error

	// Succeed early if no socket was requested.
	if listener.localAddress == "null" {
		return nil
	}

	listener.l, err = net.Listen(listener.protocol, listener.localAddress)
	if err != nil {
		log.Printf("[Listener] Failed to listen: %+v", err)
		return err
	}

	log.Printf("[Listener] Started listening on %s.", listener.localAddress)

	// Launch goroutine for servicing requests.
	go func() {
		errChan <- http.Serve(listener.l, listener.mux)
	}()

	listener.active = true
	return nil
}

// Stop stops listening for requests.
func (listener *Listener) Stop() {
	// Ignore if not active.
	if !listener.active {
		return
	}
	listener.active = false

	// Stop servicing requests.
	listener.l.Close()

	if listener.securelistener != nil {
		// Stop servicing requests on secure listener
		listener.securelistener.Close()
	}

	// Delete the unix socket.
	if listener.protocol == "unix" {
		os.Remove(listener.localAddress)
	}

	log.Printf("[Listener] Stopped listening on %s", listener.localAddress)
}

// GetMux returns the HTTP mux for the listener.
func (listener *Listener) GetMux() *http.ServeMux {
	return listener.mux
}

// GetEndpoints returns the list of registered protocol endpoints.
func (listener *Listener) GetEndpoints() []string {
	return listener.endpoints
}

// AddEndpoint registers a protocol endpoint.
func (listener *Listener) AddEndpoint(endpoint string) {
	listener.endpoints = append(listener.endpoints, endpoint)
}

// AddHandler registers a protocol handler.
func (listener *Listener) AddHandler(path string, handler func(http.ResponseWriter, *http.Request)) {
	listener.mux.HandleFunc(path, handler)
}

// Decode receives and decodes JSON payload to a request.
func (listener *Listener) Decode(w http.ResponseWriter, r *http.Request, request interface{}) error {
	var err error

	if r.Body == nil {
		err = fmt.Errorf("Request body is empty")
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
func (listener *Listener) Encode(w http.ResponseWriter, response interface{}) error {
	// Set the content type as application json
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	err := json.NewEncoder(w).Encode(response)
	if err != nil {
		http.Error(w, "Failed to encode response: "+err.Error(), http.StatusInternalServerError)
		log.Printf("[Listener] Failed to encode response: %v\n", err.Error())
	}
	return err
}
