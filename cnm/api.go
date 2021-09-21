// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package cnm

const (
	// Libnetwork remote plugin paths
	activatePath = "/Plugin.Activate"

	// Libnetwork labels
	genericData = "com.docker.network.generic"
)

type OptionMap map[string]interface{}

//
// Libnetwork remote plugin API
//

// Error response sent by plugin when a request was decoded but failed.
type errorResponse struct {
	Err string
}

// Request sent by libnetwork for activation.
type activateRequest struct{}

// Response sent by plugin for activation.
type ActivateResponse struct {
	Err        string
	Implements []string
}
