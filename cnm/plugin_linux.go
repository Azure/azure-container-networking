// Copyright 2017 Microsoft. All rights reserved.
// MIT License

// +build linux

package cnm

import (
	"os"

	"github.com/Azure/azure-container-networking/common"
)

const (
	// Default API server URL.
	defaultAPIServerURL = "unix:///run/docker/plugins/"

	// Docker plugin paths.
	pluginSpecPath   = "/etc/docker/plugins/"
	pluginSocketPath = "/run/docker/plugins/"
)

// GetAPIServerURL returns the API server URL.
func (plugin *Plugin) getAPIServerURL() string {
	urls, _ := plugin.GetOption(common.OptAPIServerURL).(string)
	if urls == "" {
		urls = defaultAPIServerURL + plugin.Name + ".sock"
	}

	os.MkdirAll(pluginSocketPath, 0o755)

	return urls
}

// GetSpecPath returns the Docker plugin spec path.
func (plugin *Plugin) getSpecPath() string {
	return pluginSpecPath
}
