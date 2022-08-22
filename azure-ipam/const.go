package main

import (
	"time"
)

const (
	pluginName    = "azure-ipam"
	cnsBaseURL    = "" // fallback to default http://localhost:10090
	cnsReqTimeout = 15 * time.Second
)

// plugin specific error codes
// https://www.cni.dev/docs/spec/#error
const (
	ErrCreateIPConfigRequest uint = iota + 100
	ErrRequestIPConfigFromCNS
	ErrProcessIPConfigResponse
)
