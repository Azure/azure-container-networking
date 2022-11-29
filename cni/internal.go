// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package cni

import (
	"encoding/json"

	cniSkel "github.com/containernetworking/cni/pkg/skel"
	cniTypes "github.com/containernetworking/cni/pkg/types"
)

const (
	Internal = "internal"
)

// CallPlugin calls the given CNI plugin through the internal interface.
func CallPlugin(plugin PluginApi, cmd string, args *cniSkel.CmdArgs, nwCfg *NetworkConfig) (*cniTypes.Result, error) {
	var err error

	savedType := nwCfg.IPAM.Type
	nwCfg.IPAM.Type = Internal
	args.StdinData = nwCfg.Serialize()

	// Call the plugin's internal interface.
	if cmd == CmdAdd {
		err = plugin.Add(args)
	} else {
		err = plugin.Delete(args)
	}

	nwCfg.IPAM.Type = savedType

	if err != nil {
		return nil, err
	}

	// Read back the result.
	var result cniTypes.Result
	err = json.Unmarshal(args.StdinData, &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}
