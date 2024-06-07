// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package cns

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns/common"
	"github.com/Azure/azure-container-networking/cns/logger"
	acn "github.com/Azure/azure-container-networking/common"
	serverTLS "github.com/Azure/azure-container-networking/server/tls"
	"github.com/Azure/azure-container-networking/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewService(t *testing.T) {
	logger.InitLogger("azure-cns.log", 0, 0, "/")
	mockStore := store.NewMockStore("test")

	config := &common.ServiceConfig{
		Name:        "test",
		Version:     "1.0",
		ChannelMode: "Direct",
		Store:       mockStore,
	}

	t.Run("NewService", func(t *testing.T) {
		svc, err := NewService(config.Name, config.Version, config.ChannelMode, config.Store)
		require.NoError(t, err)
		require.IsType(t, &Service{}, svc)

		svc.SetOption(acn.OptCnsURL, "")
		svc.SetOption(acn.OptCnsPort, "")

		require.Empty(t, config.TLSSettings)

		err = svc.Initialize(config)
		t.Cleanup(func() {
			svc.Uninitialize()
		})
		require.NoError(t, err)

		err = svc.StartListener(config)
		require.NoError(t, err)

		client := &http.Client{}

		req, err := http.NewRequestWithContext(context.TODO(), http.MethodGet, "http://localhost:10090", http.NoBody)
		require.NoError(t, err)
		resp, err := client.Do(req)
		t.Cleanup(func() {
			resp.Body.Close()
		})
		require.NoError(t, err)
	})

	t.Run("NewServiceWithTLS", func(t *testing.T) {
		testCertFilePath := createTestCertificate(t)

		config.TLSSettings = serverTLS.TlsSettings{
			TLSPort:            "10091",
			TLSSubjectName:     "localhost",
			TLSCertificatePath: testCertFilePath,
		}

		svc, err := NewService(config.Name, config.Version, config.ChannelMode, config.Store)
		require.NoError(t, err)
		require.IsType(t, &Service{}, svc)

		svc.SetOption(acn.OptCnsURL, "")
		svc.SetOption(acn.OptCnsPort, "")

		err = svc.Initialize(config)
		t.Cleanup(func() {
			svc.Uninitialize()
		})
		require.NoError(t, err)

		err = svc.StartListener(config)
		require.NoError(t, err)

		tlsClient := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					MinVersion: tls.VersionTLS12,
					MaxVersion: tls.VersionTLS13,
					ServerName: config.TLSSettings.TLSSubjectName,
					// #nosec G402 for test purposes only
					InsecureSkipVerify: true,
				},
			},
		}

		// TLS listener
		req, err := http.NewRequestWithContext(context.TODO(), http.MethodGet, "https://localhost:10091", http.NoBody)
		require.NoError(t, err)
		resp, err := tlsClient.Do(req)
		t.Cleanup(func() {
			resp.Body.Close()
		})
		require.NoError(t, err)

		// HTTP listener
		httpClient := &http.Client{}
		req, err = http.NewRequestWithContext(context.TODO(), http.MethodGet, "http://localhost:10090", http.NoBody)
		require.NoError(t, err)
		resp, err = httpClient.Do(req)
		t.Cleanup(func() {
			resp.Body.Close()
		})
		require.NoError(t, err)
	})

	t.Run("NewServiceWithMutualTLS", func(t *testing.T) {
		testCertFilePath := createTestCertificate(t)

		config.TLSSettings = serverTLS.TlsSettings{
			TLSPort:            "10091",
			TLSSubjectName:     "localhost",
			TLSCertificatePath: testCertFilePath,
			UseMTLS:            true,
		}

		svc, err := NewService(config.Name, config.Version, config.ChannelMode, config.Store)
		require.NoError(t, err)
		require.IsType(t, &Service{}, svc)

		svc.SetOption(acn.OptCnsURL, "")
		svc.SetOption(acn.OptCnsPort, "")

		err = svc.Initialize(config)
		t.Cleanup(func() {
			svc.Uninitialize()
		})
		require.NoError(t, err)

		err = svc.StartListener(config)
		require.NoError(t, err)

		mTLSConfig, err := getTLSConfigFromFile(config.TLSSettings)
		require.NoError(t, err)

		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: mTLSConfig,
			},
		}

		// TLS listener
		req, err := http.NewRequestWithContext(context.TODO(), http.MethodGet, "https://localhost:10091", http.NoBody)
		require.NoError(t, err)
		resp, err := client.Do(req)
		t.Cleanup(func() {
			resp.Body.Close()
		})
		require.NoError(t, err)

		// HTTP listener
		httpClient := &http.Client{}
		req, err = http.NewRequestWithContext(context.TODO(), http.MethodGet, "http://localhost:10090", http.NoBody)
		require.NoError(t, err)
		resp, err = httpClient.Do(req)
		t.Cleanup(func() {
			resp.Body.Close()
		})
		require.NoError(t, err)
	})
}

func TestMtlsRootCAsFromCertificate(t *testing.T) {
	testCertFilePath := createTestCertificate(t)

	tlsSettings := serverTLS.TlsSettings{
		TLSCertificatePath: testCertFilePath,
	}
	tlsCertRetriever, err := serverTLS.GetTlsCertificateRetriever(tlsSettings)
	require.NoError(t, err)

	cert, err := tlsCertRetriever.GetCertificate()
	require.NoError(t, err)

	key, err := tlsCertRetriever.GetPrivateKey()
	require.NoError(t, err)

	t.Run("returns root CA pool when provided a single self-signed CA cert", func(t *testing.T) {
		// one root CA
		tlsCert := tls.Certificate{
			Certificate: [][]byte{cert.Raw},
			PrivateKey:  key,
			Leaf:        cert,
		}

		var r *x509.CertPool
		r, err = mtlsRootCAsFromCertificate(&tlsCert)
		require.NoError(t, err)
		assert.NotNil(t, r)
	})
	t.Run("returns root CA pool when provided with a full cert chain", func(t *testing.T) {
		// simulate a full cert chain (leaf cert + root CA cert)
		tlsCert := tls.Certificate{
			Certificate: [][]byte{cert.Raw, cert.Raw},
			PrivateKey:  key,
			Leaf:        cert,
		}
		require.NoError(t, err)
		r, err := mtlsRootCAsFromCertificate(&tlsCert)
		require.NoError(t, err)
		assert.NotNil(t, r)
	})
	t.Run("does not return root CA pool when provided with no cert", func(t *testing.T) {
		r, err := mtlsRootCAsFromCertificate(nil)
		require.Error(t, err)
		assert.Nil(t, r)

		r, err = mtlsRootCAsFromCertificate(&tls.Certificate{})
		require.Error(t, err)
		assert.Nil(t, r)
	})
	t.Run("does not return root CA pool when provided with invalid certs", func(t *testing.T) {
		tt := []struct {
			invalidCert [][]byte
		}{
			{[][]byte{[]byte("invalid leaf cert")}},
			{[][]byte{[]byte("invalid leaf cert"), []byte("invalid root CA cert")}},
		}

		for _, tc := range tt {
			cert := tls.Certificate{
				Certificate: tc.invalidCert,
			}
			r, err := mtlsRootCAsFromCertificate(&cert)
			require.Error(t, err)
			assert.Nil(t, r)
		}
	})
}

// createTestCertificate is a test helper that creates a test certificate
// and writes it to a temporary file that is cleaned up after the test.
// Returns the path to the test certificate file
func createTestCertificate(t *testing.T) string {
	t.Helper()

	t.Log("Creating test certificate...")

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: "foo.com",
		},
		DNSNames:  []string{"localhost", "127.0.0.1", "example.com"},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(3 * time.Hour),

		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	// Create certificate with the template and keys
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	require.NoError(t, err)

	// Cert PEM
	pemCert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	require.NotNil(t, pemCert)

	// Private Key PEM
	privBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)
	pemKey := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})
	require.NotNil(t, pemKey)

	pemCert = append(pemCert, pemKey...)

	// Write PEM cert and key to a file in a temp dir
	testCertFilePath := filepath.Join(t.TempDir(), "dummy.pem")
	err = os.WriteFile(testCertFilePath, pemCert, 0o600)
	require.NoError(t, err)

	t.Log("Created test certificate file at: ", testCertFilePath)

	return testCertFilePath
}
