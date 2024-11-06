// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package cns

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/Azure/azure-container-networking/cns/common"
	"github.com/Azure/azure-container-networking/cns/logger"
	acn "github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/keyvault"
	localtls "github.com/Azure/azure-container-networking/server/tls"
	"github.com/Azure/azure-container-networking/store"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/pkg/errors"
)

const (
	defaultAPIServerPort = "10090"
	genericData          = "com.microsoft.azure.network.generic"
)

var errTLSConfig = errors.New("unsupported TLS version name from config")

// Service defines Container Networking Service.
type Service struct {
	*common.Service
	EndpointType string
	Listener     *acn.Listener
}

// NewService creates a new Service object.
func NewService(name, version, channelMode string, store store.KeyValueStore) (*Service, error) {
	service, err := common.NewService(name, version, channelMode, store)
	if err != nil {
		return nil, err
	}

	return &Service{
		Service: service,
	}, nil
}

func (service *Service) AddListener(config *common.ServiceConfig) error {
	var (
		err     error
		nodeURL *url.URL
	)

	// if cnsURL is empty the VM primary interface IP will be used
	// if customer specifies -c option, then use this URL with warning message and it will be deprecated soon
	cnsURL, ok := service.GetOption(acn.OptCnsURL).(string)
	if !ok {
		return errors.New("cnsURL type is wrong")
	}

	// if customer provides port number by -p option, then use VM IP with this port and localhost server also uses this port
	// otherwise it will use defaultAPIServerPort 10090
	cnsPort, ok := service.GetOption(acn.OptCnsPort).(string)
	if !ok {
		return errors.New("cnsPort type is wrong")
	}

	if cnsURL == "" {
		config.Server.EnableLocalServer = true
		// get VM primary interface's private IP
		// if customer does use -p option, then use port number customers provide
		if cnsPort == "" {
			nodeURL, err = url.Parse(fmt.Sprintf("tcp://%s:%s", config.Server.PrimaryInterfaceIP, defaultAPIServerPort))
		} else {
			config.Server.Port = cnsPort
			nodeURL, err = url.Parse(fmt.Sprintf("tcp://%s:%s", config.Server.PrimaryInterfaceIP, cnsPort))
		}

		if err != nil {
			return errors.Wrap(err, "Failed to parse URL for legacy server")
		}
	} else {
		// use the URL that customer provides by -c
		logger.Printf("user specifies -c option")

		// do not enable local server if customer uses -c option
		config.Server.EnableLocalServer = false
		nodeURL, err = url.Parse(cnsURL)
		if err != nil {
			return errors.Wrap(err, "Failed to parse URL that customer provides")
		}
	}

	logger.Debugf("CNS remote server url: %+v", nodeURL)

	nodeListener, err := acn.NewListener(nodeURL)
	if err != nil {
		return errors.Wrap(err, "Failed to construct url for node listener")
	}

	// only use TLS connection for DNC/CNS listener:
	if config.TLSSettings.TLSPort != "" {
		// listener.URL.Host will always be hostname:port, passed in to CNS via CNS command
		// else it will default to localhost
		// extract hostname and override tls port.
		hostParts := strings.Split(nodeListener.URL.Host, ":")
		tlsAddress := net.JoinHostPort(hostParts[0], config.TLSSettings.TLSPort)

		// Start the listener and HTTP and HTTPS server.
		tlsConfig, err := getTLSConfig(config.TLSSettings, config.ErrChan) //nolint
		if err != nil {
			logger.Printf("Failed to compose Tls Configuration with error: %+v", err)
			return errors.Wrap(err, "could not get tls config")
		}

		if err := nodeListener.StartTLS(config.ErrChan, tlsConfig, tlsAddress); err != nil {
			return errors.Wrap(err, "could not start tls")
		}
	}

	service.Listener = nodeListener
	logger.Debugf("[Azure CNS] Successfully initialized a service with config: %+v", config)

	return nil
}

// Initialize initializes the service and starts the listener.
func (service *Service) Initialize(config *common.ServiceConfig) error {
	logger.Debugf("[Azure CNS] Going to initialize a service with config: %+v", config)

	// Initialize the base service.
	if err := service.Service.Initialize(config); err != nil {
		return errors.Wrap(err, "failed to initialize")
	}

	if err := service.AddListener(config); err != nil {
		return errors.Wrap(err, "failed to initialize listener")
	}

	return nil
}

func getTLSConfig(tlsSettings localtls.TlsSettings, errChan chan<- error) (*tls.Config, error) {
	if tlsSettings.TLSCertificatePath != "" {
		return getTLSConfigFromFile(tlsSettings)
	}

	if tlsSettings.KeyVaultURL != "" {
		return getTLSConfigFromKeyVault(tlsSettings, errChan)
	}

	return nil, errors.Errorf("invalid tls settings: %+v", tlsSettings)
}

func getTLSConfigFromFile(tlsSettings localtls.TlsSettings) (*tls.Config, error) {
	tlsCertRetriever, err := localtls.GetTlsCertificateRetriever(tlsSettings)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get certificate retriever")
	}

	leafCertificate, err := tlsCertRetriever.GetCertificate()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get certificate")
	}

	if leafCertificate == nil {
		return nil, errors.New("certificate retrieval returned empty")
	}

	privateKey, err := tlsCertRetriever.GetPrivateKey()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get certificate private key")
	}

	tlsCert := tls.Certificate{
		Certificate: [][]byte{leafCertificate.Raw},
		PrivateKey:  privateKey,
		Leaf:        leafCertificate,
	}
	minTLSVersionNumber, err := parseTLSVersionName(tlsSettings.MinTLSVersion)
	if err != nil {
		return nil, errors.Wrap(err, "parsing MinTLSVersion from config")
	}

	tlsConfig := &tls.Config{
		MaxVersion: tls.VersionTLS13,
		MinVersion: minTLSVersionNumber,
		Certificates: []tls.Certificate{
			tlsCert,
		},
	}

	if tlsSettings.UseMTLS {
		rootCAs, err := mtlsRootCAsFromCertificate(&tlsCert)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get root CAs for configuring mTLS")
		}
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		tlsConfig.ClientCAs = rootCAs
		tlsConfig.RootCAs = rootCAs
	}

	logger.Debugf("TLS configured successfully from file: %+v", tlsSettings)

	return tlsConfig, nil
}

func getTLSConfigFromKeyVault(tlsSettings localtls.TlsSettings, errChan chan<- error) (*tls.Config, error) {
	credOpts := azidentity.ManagedIdentityCredentialOptions{ID: azidentity.ResourceID(tlsSettings.MSIResourceID)}
	cred, err := azidentity.NewManagedIdentityCredential(&credOpts)
	if err != nil {
		return nil, errors.Wrap(err, "could not create managed identity credential")
	}

	kvs, err := keyvault.NewShim(tlsSettings.KeyVaultURL, cred)
	if err != nil {
		return nil, errors.Wrap(err, "could not create new keyvault shim")
	}

	ctx := context.TODO()

	cr, err := keyvault.NewCertRefresher(ctx, kvs, logger.Log, tlsSettings.KeyVaultCertificateName)
	if err != nil {
		return nil, errors.Wrap(err, "could not create new cert refresher")
	}

	go func() {
		errChan <- cr.Refresh(ctx, tlsSettings.KeyVaultCertificateRefreshInterval)
	}()

	minTLSVersionNumber, err := parseTLSVersionName(tlsSettings.MinTLSVersion)
	if err != nil {
		return nil, errors.Wrap(err, "parsing MinTLSVersion from config")
	}

	tlsConfig := tls.Config{
		MinVersion: minTLSVersionNumber,
		MaxVersion: tls.VersionTLS13,
		GetCertificate: func(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
			return cr.GetCertificate(), nil
		},
	}

	if tlsSettings.UseMTLS {
		tlsCert := cr.GetCertificate()
		rootCAs, err := mtlsRootCAsFromCertificate(tlsCert)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get root CAs for configuring mTLS")
		}
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		tlsConfig.ClientCAs = rootCAs
		tlsConfig.RootCAs = rootCAs
	}

	logger.Debugf("TLS configured successfully from KV: %+v", tlsSettings)

	return &tlsConfig, nil
}

// Given a TLS cert, return the root CAs
func mtlsRootCAsFromCertificate(tlsCert *tls.Certificate) (*x509.CertPool, error) {
	switch {
	case tlsCert == nil || len(tlsCert.Certificate) == 0:
		return nil, errors.New("no certificate provided")
	case len(tlsCert.Certificate) == 1:
		certs := x509.NewCertPool()
		cert, err := x509.ParseCertificate(tlsCert.Certificate[0])
		if err != nil {
			return nil, errors.Wrap(err, "parsing self signed cert")
		}
		certs.AddCert(cert)

		return certs, nil
	default:
		certs := x509.NewCertPool()
		// given a fullchain cert, we skip leaf cert at index 0 because
		// we only want intermediate and root certs in the cert pool for mTLS
		for _, certBytes := range tlsCert.Certificate[1:] {
			cert, err := x509.ParseCertificate(certBytes)
			if err != nil {
				return nil, errors.Wrap(err, "parsing root certs")
			}
			certs.AddCert(cert)
		}
		return certs, nil
	}
}

func (service *Service) StartListener(config *common.ServiceConfig) error {
	logger.Debugf("[Azure CNS] Going to start listener: %+v", config)

	// Initialize the listener.
	if service.Listener != nil {
		logger.Debugf("[Azure CNS] Starting listener: %+v", config)
		// Start the listener.
		// continue to listen on the normal endpoint for http traffic, this will be supported
		// for sometime until partners migrate fully to https
		if err := service.Listener.Start(config.ErrChan); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("Failed to start a listener, it is not initialized, config %+v", config)
	}

	return nil
}

// Uninitialize cleans up the plugin.
func (service *Service) Uninitialize() {
	service.Listener.Stop()
	service.Service.Uninitialize()
}

// ParseOptions returns generic options from a libnetwork request.
func (service *Service) ParseOptions(options OptionMap) OptionMap {
	opt, _ := options[genericData].(OptionMap)
	return opt
}

// SendErrorResponse sends and logs an error response.
func (service *Service) SendErrorResponse(w http.ResponseWriter, errMsg error) {
	resp := errorResponse{errMsg.Error()}
	err := acn.Encode(w, &resp)
	logger.Errorf("[%s] %+v %s.", service.Name, &resp, err.Error())
}

// parseTLSVersionName returns the version number for the provided TLS version name
// (e.g. 0x0301)
func parseTLSVersionName(versionName string) (uint16, error) {
	switch versionName {
	case "TLS 1.2":
		return tls.VersionTLS12, nil
	case "TLS 1.3":
		return tls.VersionTLS13, nil
	default:
		return 0, errors.Wrapf(errTLSConfig, "version name %s", versionName)
	}
}
