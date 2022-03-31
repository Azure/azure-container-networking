package transport

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"google.golang.org/grpc/credentials"
)

const (
	serverCertPEMFilename = "tls.crt"
	serverKeyPEMFilename  = "tls.key"
	caCertPEMFilename     = "ca.crt"
	path                  = "/usr/local/npm"
)

func serverTLSCreds() (credentials.TransportCredentials, error) {
	certFilepath := path + "/" + serverCertPEMFilename
	keyFilepath := path + "/" + serverKeyPEMFilename

	creds, err := credentials.NewServerTLSFromFile(certFilepath, keyFilepath)
	if err != nil {
		return nil, fmt.Errorf("failed to create creds from cert/key files : %w", err)
	}
	return creds, nil
}

func clientTLSConfig() (*tls.Config, error) {
	caCertFilepath := path + "/" + caCertPEMFilename
	// Load certificate of the CA who signed server's certificate
	pemServerCA, err := os.ReadFile(caCertFilepath)
	if err != nil {
		return nil, fmt.Errorf("Failed to read the CA cert : %w", err)
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(pemServerCA) {
		return nil, fmt.Errorf("failed to append ca cert to cert pool : %w", ErrTLSCerts)
	}

	// Create the credentials and return it
	return &tls.Config{ //nolint // setting tls min version to 3
		RootCAs:            certPool,
		InsecureSkipVerify: false,
	}, nil
}
