// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package cni

import (
	cniSkel "github.com/containernetworking/cni/pkg/skel"
)

const (
	// CNI commands.
	Cmd       = "CNI_COMMAND"
	CmdAdd    = "ADD"
	CmdGet    = "GET"
	CmdDel    = "DEL"
	CmdUpdate = "UPDATE"

	// nonstandard CNI spec command, used to dump CNI state to stdout
	CmdGetEndpointsState = "GET_ENDPOINT_STATE"

	// CNI errors.
	ErrRuntime = 100

	// DefaultVersion is the CNI version used when no version is specified in a network config file.
	defaultVersion = "0.2.0"
)

// Supported CNI versions.
var supportedVersions = []string{"0.1.0", "0.2.0", "0.3.0", "0.3.1", "0.4.0"}

// CNI contract.
type PluginApi interface {
	Add(args *cniSkel.CmdArgs) error
	Get(args *cniSkel.CmdArgs) error
	Delete(args *cniSkel.CmdArgs) error
	Update(args *cniSkel.CmdArgs) error
}
