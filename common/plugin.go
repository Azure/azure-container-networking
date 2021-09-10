// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package common

import (
	"github.com/Azure/azure-container-networking/store"
)

// Plugin is the parent class that implements behavior common to all plugins.
type Plugin struct {
	Name    string
	Version string
	Options map[string]interface{}
	ErrChan chan error
	Store   store.KeyValueStore
}

// Plugin base interface.
type PluginApi interface {
	Start(*PluginConfig) error
	Stop()
	GetOption(string) interface{}
	SetOption(string, interface{})
}

// Network internal interface.
type NetApi interface {
	AddExternalInterface(ifName string, subnet string) error
}

// IPAM internal interface.
type IpamApi interface{}

// Plugin common configuration.
type PluginConfig struct {
	Version  string
	NetApi   NetApi
	IpamApi  IpamApi
	Listener *Listener
	ErrChan  chan error
	Store    store.KeyValueStore
}

// NewPlugin creates a new Plugin object.
func NewPlugin(name, version string) (*Plugin, error) {
	return &Plugin{
		Name:    name,
		Version: version,
		Options: make(map[string]interface{}),
	}, nil
}

// Initialize initializes the plugin.
func (plugin *Plugin) Initialize(config *PluginConfig) error {
	plugin.ErrChan = config.ErrChan
	plugin.Store = config.Store

	return nil
}

// Uninitialize cleans up the plugin.
func (plugin *Plugin) Uninitialize() {
}

// GetOption gets the option value for the given key.
func (plugin *Plugin) GetOption(key string) interface{} {
	return plugin.Options[key]
}

// SetOption sets the option value for the given key.
func (plugin *Plugin) SetOption(key string, value interface{}) {
	plugin.Options[key] = value
}
