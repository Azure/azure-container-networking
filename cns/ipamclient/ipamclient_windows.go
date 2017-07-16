// Copyright 2017 Microsoft. All rights reserved.
// MIT License

// +build windows

package ipamclient

import (
	"net/http"
)

const (
	defaultIpamPluginURL = "http://localhost:48080"
)

func getClient(url string) (http.Client, error) {
	httpc := http.Client{}
	return httpc, nil
}
