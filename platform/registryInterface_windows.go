package platform

import (
	"fmt"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

type RegistryClient interface {
	OpenKey(k registry.Key, path string, access uint32) (RegistryKey, error)
	restartService(serviceName string) error
}

type RegistryKey interface {
	GetStringValue(name string) (val string, valtype uint32, err error)
	SetStringValue(name, value string) error
	Close() error
}

type registryClient struct {
	Timeout time.Duration
	logger  *zap.Logger
}

type registryKey struct {
	key registry.Key
}

func NewRegistryClient(logger *zap.Logger) RegistryClient {
	return &registryClient{
		Timeout: defaultExecTimeout * time.Second,
		logger:  logger,
	}
}

func (r *registryClient) OpenKey(k registry.Key, path string, access uint32) (RegistryKey, error) {
	key, err := registry.OpenKey(k, path, access)
	if err != nil {
		return nil, err
	}
	return &registryKey{key}, nil
}

func (k *registryKey) GetStringValue(name string) (string, uint32, error) {
	return k.key.GetStringValue(name)
}

func (k *registryKey) SetStringValue(name string, value string) error {
	return k.key.SetStringValue(name, value)
}

func (k *registryKey) Close() error {
	return k.key.Close()
}

// straight out of chat gpt
func (r *registryClient) restartService(serviceName string) error {
	// Connect to the service manager
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("could not connect to service manager: %v", err)
	}
	defer m.Disconnect()

	// Open the service by name
	service, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("could not access service: %v", err)
	}
	defer service.Close()

	// Stop the service
	_, err = service.Control(svc.Stop)
	if err != nil {
		return fmt.Errorf("could not stop service: %v", err)
	}

	// Wait for the service to stop
	status, err := service.Query()
	if err != nil {
		return fmt.Errorf("could not query service status: %v", err)
	}
	for status.State != svc.Stopped {
		time.Sleep(500 * time.Millisecond)
		status, err = service.Query()
		if err != nil {
			return fmt.Errorf("could not query service status: %v", err)
		}
	}

	// Start the service again
	err = service.Start()
	if err != nil {
		return fmt.Errorf("could not start service: %v", err)
	}

	return nil
}
