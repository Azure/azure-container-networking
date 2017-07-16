// Copyright 2017 Microsoft. All rights reserved.
// MIT License

// +build linux

package ipamclient

import (
	"context"
	"log"
	"net"
	"net/http"
)

const (
	defaultIpamPluginURL = "http://unix"
	pluginSockPath       = "/run/docker/plugins/azure-vnet.sock"
)

//getClient - returns unix http client
func getClient(url string) (*http.Client, error) {
	var httpc *http.Client
	if url == defaultIpamPluginURL {
		dialContext, err := net.Dial("unix", pluginSockPath)
		if err != nil {
			log.Printf("[Azure CNS] Error.Dial context error %v", err.Error())
			return nil, err
		}

		httpc = &http.Client{
			Transport: &http.Transport{
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					return dialContext, nil
				},
			},
		}
	} else {
		httpc = &http.Client{}
	}

	return httpc, nil
}
