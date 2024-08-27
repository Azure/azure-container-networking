//go:build windows
// +build windows

package platform

import "golang.org/x/sys/windows/registry"

// Registry interface for interacting with the Windows registry
type Registry interface {
	OpenKey(k registry.Key, path string, access uint32) (RegistryKey, error)
}

// RegistryKey interface to represent an open registry key
type RegistryKey interface {
	GetStringValue(name string) (string, uint32, error)
	SetStringValue(name, value string) error
	Close() error
}

type WindowsRegistry struct{}

// WindowsRegistryKey implements the RegistryKey interface
type WindowsRegistryKey struct {
	key registry.Key
}

func (r *WindowsRegistry) OpenKey(k registry.Key, path string, access uint32) (RegistryKey, error) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, path, access)
	if err != nil {
		return nil, err
	}
	return &WindowsRegistryKey{key}, nil
}

func (k *WindowsRegistryKey) GetStringValue(name string) (val string, valtype uint32, err error) {
	value, valType, err := k.key.GetStringValue(name)
	return value, valType, err
}

func (k *WindowsRegistryKey) SetStringValue(name, value string) error {
	return k.key.SetStringValue(name, value)
}

func (k *WindowsRegistryKey) Close() error {
	return k.key.Close()
}
