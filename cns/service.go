// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package cns

import (
	"net/http"
	"net/url"

	"github.com/Azure/azure-container-networking/cns/common"
	acn "github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/store"
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
func NewService(name, version string, store store.KeyValueStore) (*Service, error) {
	service, err := common.NewService(name, version, store)

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
	service.Service.Initialize(config)

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

		// Start the listener.
		err = listener.Start(config.ErrChan)
		if err != nil {
			return err
		}

		config.Listener = listener
	}

	service.Listener = config.Listener

	log.Debugf("[Azure CNS] Successfully initialized a service with config: %+v", config)
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
