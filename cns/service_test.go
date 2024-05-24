// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package cns

import (
	"context"
	"crypto/tls"
	"net/http"
	"testing"

	"github.com/Azure/azure-container-networking/cns/common"
	"github.com/Azure/azure-container-networking/cns/logger"
	acn "github.com/Azure/azure-container-networking/common"
	serverTLS "github.com/Azure/azure-container-networking/server/tls"
	"github.com/Azure/azure-container-networking/store"
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
