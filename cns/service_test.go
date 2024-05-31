// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package cns

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"testing"

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
		config.TLSSettings = serverTLS.TlsSettings{
			TLSPort:            "10091",
			TLSSubjectName:     "localhost",
			TLSCertificatePath: "testdata/dummy.pem",
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

		client := &http.Client{
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

		req, err := http.NewRequestWithContext(context.TODO(), http.MethodGet, "https://localhost:10091", http.NoBody)
		require.NoError(t, err)
		resp, err := client.Do(req)
		t.Cleanup(func() {
			resp.Body.Close()
		})
		require.NoError(t, err)
	})

	t.Run("NewServiceWithMutualTLS", func(t *testing.T) {
		config.TLSSettings = serverTLS.TlsSettings{
			TLSPort:            "10091",
			TLSSubjectName:     "localhost",
			TLSCertificatePath: "testdata/dummy.pem",
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

		tlsConfig, err := getTLSConfigFromFile(config.TLSSettings)
		require.NoError(t, err)

		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsConfig,
			},
		}

		req, err := http.NewRequestWithContext(context.TODO(), http.MethodGet, "https://localhost:10091", http.NoBody)
		require.NoError(t, err)
		resp, err := client.Do(req)
		t.Cleanup(func() {
			resp.Body.Close()
		})
		require.NoError(t, err)
	})
}

func TestMtlsRootCAsFromCertificate(t *testing.T) {
	tlsSettings := serverTLS.TlsSettings{
		TLSCertificatePath: "testdata/dummy.pem",
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
