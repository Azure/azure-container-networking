// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package cnm

import (
	"io/ioutil"
	"net/http"
	"net/url"
	"os"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
)

// Plugin is the parent class for CNM plugins.
type Plugin struct {
	*common.Plugin
	EndpointType string
	Listener     *common.Listener
}

// NewPlugin creates a new Plugin object.
func NewPlugin(name, version, endpointType string) (*Plugin, error) {
	// Setup base plugin.
	plugin, err := common.NewPlugin(name, version)
	if err != nil {
		return nil, err
	}

	return &Plugin{
		Plugin:       plugin,
		EndpointType: endpointType,
	}, nil
}

// Initialize initializes the plugin and starts the listener.
func (plugin *Plugin) Initialize(config *common.PluginConfig) error {
	// Initialize the base plugin.
	plugin.Plugin.Initialize(config)

	// Initialize the shared listener.
	if config.Listener == nil {
		// Fetch and parse the API server URL.
		u, err := url.Parse(plugin.getAPIServerURL())
		if err != nil {
			return err
		}

		// Create the listener.
		listener, err := common.NewListener(u)
		if err != nil {
			return err
		}

		// Add generic protocol handlers.
		listener.AddHandler(activatePath, plugin.activate)

		// Start the listener.
		err = listener.Start(config.ErrChan)
		if err != nil {
			return err
		}

		config.Listener = listener
	}

	plugin.Listener = config.Listener

	return nil
}

// Uninitialize cleans up the plugin.
func (plugin *Plugin) Uninitialize() {
	plugin.Listener.Stop()
	plugin.Plugin.Uninitialize()
}

// EnableDiscovery enables Docker to discover the plugin by creating the plugin spec file.
func (plugin *Plugin) EnableDiscovery() error {
	// Plugins using unix domain sockets do not need a spec file.
	if plugin.Listener.URL.Scheme == "unix" {
		return nil
	}

	// Create the spec directory.
	path := plugin.getSpecPath()
	os.MkdirAll(path, 0755)

	// Write the listener URL to the spec file.
	fileName := path + plugin.Name + ".spec"
	url := plugin.Listener.URL.String()
	err := ioutil.WriteFile(fileName, []byte(url), 0644)
	return err
}

// DisableDiscovery disables discovery by deleting the plugin spec file.
func (plugin *Plugin) DisableDiscovery() {
	// Plugins using unix domain sockets do not need a spec file.
	if plugin.Listener.URL.Scheme == "unix" {
		return
	}

	fileName := plugin.getSpecPath() + plugin.Name + ".spec"
	os.Remove(fileName)
}

// ParseOptions returns generic options from a libnetwork request.
func (plugin *Plugin) ParseOptions(options OptionMap) OptionMap {
	opt, _ := options[genericData].(map[string]interface{})
	return opt
}

//
// Libnetwork remote plugin API
//

// Activate handles Activate requests.
func (plugin *Plugin) activate(w http.ResponseWriter, r *http.Request) {
	var req activateRequest

	log.Request(plugin.Name, &req, nil)

	resp := ActivateResponse{Implements: plugin.Listener.GetEndpoints()}
	err := plugin.Listener.Encode(w, &resp)

	log.Response(plugin.Name, &resp, 0, "Success", err)
}

// SendErrorResponse sends and logs an error response.
func (plugin *Plugin) SendErrorResponse(w http.ResponseWriter, errMsg error) {
	resp := errorResponse{errMsg.Error()}
	err := plugin.Listener.Encode(w, &resp)

	log.Response(plugin.Name, &resp, 0, "Success", err)
}
