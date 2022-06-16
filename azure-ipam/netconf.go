package main

import (
	"encoding/json"

	cniTypes "github.com/containernetworking/cni/pkg/types"
)

// Parse network config from given byte array
func ParseNetConf(b []byte) (*cniTypes.NetConf, error) {
	netConf := &cniTypes.NetConf{}
	err := json.Unmarshal(b, netConf)
	if err != nil {
		return nil, err
	}

	if netConf.CNIVersion == "" {
		netConf.CNIVersion = "0.2.0"
	}

	return netConf, nil
}
