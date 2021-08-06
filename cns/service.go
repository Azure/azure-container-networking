// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package cns

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/Azure/azure-container-networking/cns/common"
	"github.com/Azure/azure-container-networking/cns/logger"
	acn "github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/store"
	"github.com/pkg/errors"
)

const (
	// Default CNS server URL.
	defaultAPIServerURL = "tcp://localhost:10090"
	genericData         = "com.microsoft.azure.network.generic"
)

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

// GetAPIServerURL returns the API server URL.
func (service *Service) getAPIServerURL() string {
	urls, _ := service.GetOption(acn.OptCnsURL).(string)
	if urls == "" {
		urls = defaultAPIServerURL
	}

	return urls
}

// Initialize initializes the service and starts the listener.
func (service *Service) Initialize(config *common.ServiceConfig) error {
	log.Debugf("[Azure CNS] Going to initialize a service with config: %+v", config)

	// Initialize the base service.
	if err := service.Service.Initialize(config); err != nil {
		return errors.Wrap(err, "failed to initialize")
	}

	// Initialize the listener.
	if config.Listener == nil {
		// Fetch and parse the API server URL.
		u, err := url.Parse(service.getAPIServerURL())
		if err != nil {
			return err
		}
		// Create the listener.
		listener, err := acn.NewListener(u)
		if err != nil {
			return err
		}
		if config.TlsSettings.TLSPort != "" {
			// listener.URL.Host will always be hostname:port, passed in to CNS via CNS command
			// else it will default to localhost
			// extract hostname and override tls port.
			hostParts := strings.Split(listener.URL.Host, ":")
			config.TlsSettings.TLSEndpoint = hostParts[0] + ":" + config.TlsSettings.TLSPort
			// Start the listener and HTTP and HTTPS server.
			if err = listener.StartTLS(config.ErrChan, config.TlsSettings); err != nil {
				return err
			}
		}

		logger.Printf("HTTP listener will be started later after CNS state has been reconciled")
		config.Listener = listener
	}

	service.Listener = config.Listener

	log.Debugf("[Azure CNS] Successfully initialized a service with config: %+v", config)
	return nil
}

func (service *Service) StartListener(config *common.ServiceConfig) error {
	log.Debugf("[Azure CNS] Going to start listener: %+v", config)

	// Initialize the listener.
	if service.Listener != nil {
		log.Debugf("[Azure CNS] Starting listener: %+v", config)
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
	err := service.Listener.Encode(w, &resp)
	log.Errorf("[%s] %+v %s.", service.Name, &resp, err.Error())
}
