package platform

import (
	"errors"

	"golang.org/x/sys/windows/registry"
)

type MockRegistryClient struct {
	returnError       bool
	keys              map[string]*mockRegistryKey
	setOpenKey        setOpenKey
	setRestartService setRestartService
}

type (
	setOpenKey        func(k registry.Key, path string, access uint32) (RegistryKey, error)
	setRestartService func(serviceName string) error
)

type mockRegistryKey struct {
	Values map[string]string
}

func NewMockRegistryClient(returnErr bool) *MockRegistryClient {
	return &MockRegistryClient{
		returnError: returnErr,
		keys:        make(map[string]*mockRegistryKey),
	}
}

func (r *MockRegistryClient) SetOpenKey(fn setOpenKey) {
	r.setOpenKey = fn
}

func (r *MockRegistryClient) OpenKey(k registry.Key, path string, access uint32) (RegistryKey, error) {
	return r.setOpenKey(k, path, access)

}

func (k *mockRegistryKey) GetStringValue(name string) (string, uint32, error) {
	if value, exists := k.Values[name]; exists {
		return value, registry.SZ, nil
	}
	return "", 0, errors.New("value does not exist")
}

func (k *mockRegistryKey) SetStringValue(name string, value string) error {
	k.Values[name] = name
	return nil
}

func (k *mockRegistryKey) Close() error {
	return nil
}

func (r *MockRegistryClient) SetRestartService(fn setRestartService) {
	r.setRestartService = fn
}

func (r *MockRegistryClient) restartService(serviceName string) error {
	return r.setRestartService(serviceName)

}
