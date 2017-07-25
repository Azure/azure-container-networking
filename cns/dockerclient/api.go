// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package dockerclient

const (
	createNetworkPath  = "/networks/create"
	inspectNetworkPath = "/networks/"

	OptDisableSnat = "DisableSNAT"
)

// Config describes subnet/gateway for ipam.
type Config struct {
	Subnet string
}

// IPAM describes ipam details
type IPAM struct {
	Driver string
	Config []Config
}

// NetworkConfiguration describes configuration for docker network create.
type NetworkConfiguration struct {
	Name     string
	Driver   string
	IPAM     IPAM
	Internal bool
	Options  map[string]interface{}
}

// DockerErrorResponse defines the error response retunred by docker.
type DockerErrorResponse struct {
	message string
}
