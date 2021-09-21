//go:build !ignore_uncovered
// +build !ignore_uncovered

package installer

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	ccn "github.com/Azure/azure-container-networking/cni"
	c "github.com/Azure/azure-container-networking/tools/acncli/api"
)

type rawPlugin struct {
	Type string `json:"type"`
}

type rawConflist struct {
	Name       string        `json:"name"`
	CniVersion string        `json:"cniVersion"`
	Plugins    []interface{} `json:"plugins"`
}

func LoadConfList(conflistpath string) (rawConflist, error) {
	var conflist rawConflist

	jsonFile, err := os.Open(conflistpath)
	if err != nil {
		return conflist, err
	}
	defer jsonFile.Close()

	byteValue, err := ioutil.ReadAll(jsonFile)
	if err != nil {
		return conflist, err
	}

	err = json.Unmarshal(byteValue, &conflist)

	return conflist, nil
}

func LoadConf(conflistpath string) (ccn.NetworkConfig, rawConflist, int, error) {
	var (
		netconfig   ccn.NetworkConfig
		conflist    rawConflist
		pluginIndex int
		err         error
		plugin      rawPlugin
	)

	conflist, err = LoadConfList(conflistpath)
	if err != nil {
		return netconfig, conflist, pluginIndex, err
	}

	// find the conflist that matches the AzureCNIBin type
	for pluginIndex = range conflist.Plugins {
		mapbytes, err := json.Marshal(conflist.Plugins[pluginIndex])
		if err != nil {
			return netconfig, conflist, pluginIndex, err
		}
		err = json.Unmarshal(mapbytes, &plugin)

		if plugin.Type == c.AzureCNIBin {
			err = json.Unmarshal(mapbytes, &netconfig)
			return netconfig, conflist, pluginIndex, err
		}
	}

	return netconfig, conflist, pluginIndex, fmt.Errorf("Conf in conflist not found matching %s type", c.AzureCNIBin)
}

func ModifyConflists(conflistpath string, installerConf InstallerConfig, perm os.FileMode) error {
	netconfig, conflist, confindex, err := LoadConf(conflistpath)

	// change the netconfig from passed installerConf
	netconfig.Ipam.Type = installerConf.IPAMType
	netconfig.Mode = installerConf.CNIMode

	// no bridge in transparent mode
	if netconfig.Mode == c.Transparent {
		netconfig.Bridge = ""
	} else if netconfig.Mode == c.Bridge {
		netconfig.Bridge = c.Azure0
	}

	// set conf back in conflist
	conflist.Plugins[confindex] = netconfig

	// get target path
	dstFile := installerConf.DstConflistDir + filepath.Base(conflistpath)
	filebytes, err := json.MarshalIndent(conflist, "", "\t")
	if err != nil {
		return err
	}

	fmt.Printf("🚛 - Installing %v...\n", dstFile)
	return ioutil.WriteFile(dstFile, filebytes, perm)
}

func PrettyPrint(o interface{}) {
	pretty, _ := json.MarshalIndent(o, "", "  ")
	fmt.Println(string(pretty))
}
