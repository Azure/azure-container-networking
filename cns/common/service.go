// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package common

import (
	"errors"

	acn "github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	bolt "go.etcd.io/bbolt"
)

// Service implements behavior common to all services.
type Service struct {
	Name     string
	Version  string
	Options  map[string]interface{}
	ErrChan  chan error
	Database *bolt.DB
}

// ServiceAPI defines base interface.
type ServiceAPI interface {
	Start(*ServiceConfig) error
	Stop()
	GetOption(string) interface{}
	SetOption(string, interface{})
}

// ServiceConfig specifies common configuration.
type ServiceConfig struct {
	Name     string
	Version  string
	Listener *acn.Listener
	ErrChan  chan error
	Database *bolt.DB
}

// NewService creates a new Service object.
func NewService(name, version string, database *bolt.DB) (*Service, error) {
	log.Debugf("[Azure CNS] Going to create a service object with name: %v. version: %v.", name, version)

	svc := &Service{
		Name:     name,
		Version:  version,
		Options:  make(map[string]interface{}),
		Database: database,
	}

	log.Debugf("[Azure CNS] Finished creating service object with name: %v. version: %v.", name, version)
	return svc, nil
}

// Initialize initializes the service.
func (service *Service) Initialize(config *ServiceConfig) error {
	if config == nil {
		err := "[Azure CNS Errror] Initialize called with nil ServiceConfig."
		log.Printf(err)
		return errors.New(err)
	}

	log.Debugf("[Azure CNS] Going to initialize the service: %+v with config: %+v.", service, config)

	service.ErrChan = config.ErrChan
	service.Database = config.Database
	service.Version = config.Version

	log.Debugf("[Azure CNS] nitialized service: %+v with config: %+v.", service, config)

	return nil
}

// Uninitialize cleans up the service.
func (service *Service) Uninitialize() {
}

// GetOption gets the option value for the given key.
func (service *Service) GetOption(key string) interface{} {
	return service.Options[key]
}

// SetOption sets the option value for the given key.
func (service *Service) SetOption(key string, value interface{}) {
	service.Options[key] = value
}
