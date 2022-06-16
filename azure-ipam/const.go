package main

import "time"

const (
	pluginName    = "azure-ipam"
	cnsBaseURL    = "" // fallback to default http://localhost:10090
	csnReqTimeout = 15 * time.Second
)
