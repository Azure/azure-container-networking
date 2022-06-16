package main

import "time"

const (
	PLUGIN_NAME     = "azure-ipam"
	CNS_BASE_URL    = "" // fallback to default http://localhost:10090
	CNS_REQ_TIMEOUT = 15 * time.Second
)
