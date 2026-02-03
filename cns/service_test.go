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
			MinTLSVersion:      "TLS 1.2",
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

		minTLSVersionNumber, err := parseTLSVersionName(config.TLSSettings.MinTLSVersion)
		require.NoError(t, err)

		tlsClient := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					MinVersion: minTLSVersionNumber,
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

		cases := []struct {
			name                     string
			tlsSettings              serverTLS.TlsSettings
			handshakeFailureExpected bool
		}{
			{
				name: "matching client SANs",
				tlsSettings: serverTLS.TlsSettings{
					TLSPort:                   "10091",
					TLSSubjectName:            "localhost",
					TLSCertificatePath:        testCertFilePath,
					UseMTLS:                   true,
					MinTLSVersion:             "TLS 1.2",
					MtlsClientCertSubjectName: "example.com",
				},
				handshakeFailureExpected: false,
			},
			{
				name: "matching client cert CN",
				tlsSettings: serverTLS.TlsSettings{
					TLSPort:                   "10093",
					TLSSubjectName:            "localhost",
					TLSCertificatePath:        testCertFilePath,
					UseMTLS:                   true,
					MinTLSVersion:             "TLS 1.2",
					MtlsClientCertSubjectName: "foo.com", // Common Name from test certificate
				},
				handshakeFailureExpected: false,
			},
			{
				name: "failing to match client SANs and CN",
				tlsSettings: serverTLS.TlsSettings{
					TLSPort:                   "10092",
					TLSSubjectName:            "localhost",
					TLSCertificatePath:        testCertFilePath,
					UseMTLS:                   true,
					MinTLSVersion:             "TLS 1.2",
					MtlsClientCertSubjectName: "random.com",
				},
				handshakeFailureExpected: true,
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				config.TLSSettings = tc.tlsSettings

				svc, err := NewService(config.Name, config.Version, config.ChannelMode, config.Store)
				require.NoError(t, err)
				require.IsType(t, &Service{}, svc)

				svc.SetOption(acn.OptCnsURL, "")
				svc.SetOption(acn.OptCnsPort, "")

				err = svc.Initialize(config)
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

				tlsURL := "https://localhost:" + tc.tlsSettings.TLSPort
				// TLS listener
				req, err := http.NewRequestWithContext(context.TODO(), http.MethodGet, tlsURL, http.NoBody)
				require.NoError(t, err)
				resp, err := client.Do(req)
				if tc.handshakeFailureExpected {
					require.Error(t, err)
					require.ErrorContains(t, err, "Failed to verify client certificate subject name during mTLS")
				} else {
					require.NoError(t, err)
					t.Cleanup(func() {
						if resp != nil && resp.Body != nil {
							resp.Body.Close()
						}
					})
				}

				// HTTP listener
				httpClient := &http.Client{}
				req, err = http.NewRequestWithContext(context.TODO(), http.MethodGet, "http://localhost:10090", http.NoBody)
				require.NoError(t, err)
				resp, err = httpClient.Do(req)
				require.NoError(t, err)
				t.Cleanup(func() {
					if resp != nil && resp.Body != nil {
						resp.Body.Close()
					}
				})

				// Cleanup
				svc.Uninitialize()
			})
		}
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

	tests := []struct {
		name       string
		cert       *tls.Certificate
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "returns root CA pool when provided a single self-signed CA cert",
			cert: &tls.Certificate{
				Certificate: [][]byte{cert.Raw},
				PrivateKey:  key,
				Leaf:        cert,
			},
			wantErr:    false,
			wantErrMsg: "",
		},
		{
			name: "returns root CA pool when provided with a full cert chain",
			cert: &tls.Certificate{
				Certificate: [][]byte{cert.Raw, cert.Raw},
				PrivateKey:  key,
				Leaf:        cert,
			},
			wantErr:    false,
			wantErrMsg: "",
		},
		{
			name:       "does not return root CA pool when provided with nil",
			cert:       nil,
			wantErr:    true,
			wantErrMsg: "no certificate provided",
		},
		{
			name:       "does not return root CA pool when provided with empty cert",
			cert:       &tls.Certificate{},
			wantErr:    true,
			wantErrMsg: "no certificate provided",
		},
		{
			name: "does not return root CA pool when provided with single invalid cert",
			cert: &tls.Certificate{
				Certificate: [][]byte{[]byte("invalid leaf cert")},
			},
			wantErr:    true,
			wantErrMsg: "parsing self signed cert",
		},
		{
			name: "does not return root CA pool when provided with invalid full chain cert",
			cert: &tls.Certificate{
				Certificate: [][]byte{[]byte("invalid leaf cert"), []byte("invalid root CA cert")},
			},
			wantErr:    true,
			wantErrMsg: "parsing root certs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := mtlsRootCAsFromCertificate(tt.cert)
			if tt.wantErr {
				require.Error(t, err)
				require.ErrorContains(t, err, tt.wantErrMsg)
				assert.Nil(t, r)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, r)
			}
		})
	}
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

	fileContent, err := postProcessPEMCert(pemCert)
	require.NoError(t, err)

	// Write PEM cert and key to a file in a temp dir
	testCertFilePath := filepath.Join(t.TempDir(), "dummy.pem")
	err = os.WriteFile(testCertFilePath, fileContent, 0o600)
	require.NoError(t, err)

	t.Log("Created test certificate file at: ", testCertFilePath)

	return testCertFilePath
}

func TestTLSVersionNumber(t *testing.T) {
	t.Run("unsupported ServerSettings.MinTLSVersion TLS 1.0", func(t *testing.T) {
		versionNumber, err := parseTLSVersionName("TLS 1.0")
		require.Equal(t, uint16(0), versionNumber)
		require.Error(t, err)
		require.ErrorContains(t, err, "unsupported TLS version name")
	})

	t.Run("unsupported ServerSettings.MinTLSVersion TLS 1.1", func(t *testing.T) {
		versionNumber, err := parseTLSVersionName("TLS 1.1")
		require.Equal(t, uint16(0), versionNumber)
		require.Error(t, err)
		require.ErrorContains(t, err, "unsupported TLS version name")
	})
	t.Run("unsupported ServerSettings.MinTLSVersion TLS 1.4", func(t *testing.T) {
		versionNumber, err := parseTLSVersionName("TLS 1.4")
		require.Equal(t, uint16(0), versionNumber)
		require.Error(t, err)
		require.ErrorContains(t, err, "unsupported TLS version name")
	})

	t.Run("valid ServerSettings.MinTLSVersion", func(t *testing.T) {
		versionNumber, err := parseTLSVersionName("TLS 1.2")
		require.Equal(t, uint16(tls.VersionTLS12), versionNumber)
		require.NoError(t, err)
	})
}

func TestMaskHalf(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"one char string", "e", "*"},
		{"two chars string", "ex", "e*"},
		{"three chars string", "exa", "e**"},
		{"four chars string", "exam", "ex**"},
		{"five chars string", "examp", "ex***"},
		{"long string", "example.com", "examp******"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := maskHalf(tc.in)
			if got != tc.want {
				t.Fatalf("maskHalf(%s) = %s, want %s", tc.in, got, tc.want)
			}
		})
	}
}
