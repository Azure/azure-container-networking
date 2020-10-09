package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	ccn "github.com/Azure/azure-container-networking/cni"
)

const (
	binPerm      = 755
	conflistPerm = 644

	linux   = "linux"
	windows = "windows"
	amd64   = "amd64"

	azureCNIBin             = "azure-vnet"
	azureTelemetryBin       = "azure-vnet-telemetry"
	azureCNSIPAM            = "azure-cns"
	auzureVNETIPAM          = "azure-vnet-ipam"
	conflistExtension       = ".conflist"
	cni                     = "cni"
	multitenancy            = "multitenancy"
	singletenancy           = "singletenancy"
	defaultSrcDirLinux      = "/output/"
	defaultBinDirLinux      = "/opt/cni/bin/"
	defaultConflistDirLinux = "/etc/cni/net.d/"
	defaultLogFile          = "/var/log/azure-vnet.log"
	transparent             = "transparent"
	bridge                  = "bridge"

	envCNIOS                     = "CNI_OS"
	envCNITYPE                   = "CNI_TYPE"
	envCNISourceDir              = "CNI_SRC_DIR"
	envCNIDestinationBinDir      = "CNI_DST_BIN_DIR"
	envCNIDestinationConflistDir = "CNI_DST_CONFLIST_DIR"
	envCNIIPAMType               = "CNI_IPAM_TYPE"
	envCNIMode                   = "CNI_MODE"
	envCNIExemptBins             = "CNI_EXCEMPT_BINS"
	envCNILogFile                = "CNI_LOG_FILE"
)

type installerConfig struct {
	srcDir         string
	dstBinDir      string
	dstConflistDir string
	ipamType       string
	exemptBins     map[string]bool
	logFile        string
	osType         string
	cniType        string
	cniMode        string
}

func (i *installerConfig) SetExempt(exempt []string) {
	// set exempt binaries to skip installing
	// convert to all lower case, strip whitespace, and split on comma
	for _, binName := range exempt {
		i.exemptBins[binName] = true
	}
}

func (i *installerConfig) SetOSType(osType string) error {
	if strings.EqualFold(osType, linux) || strings.EqualFold(osType, windows) {
		i.osType = fmt.Sprintf("%s_%s", osType, amd64)
	} else {
		return fmt.Errorf("No target OS type supplied, please set %q env and try again", envCNIOS)
	}
	return nil
}

func (i *installerConfig) SetCNIType(cniType string) error {
	// get paths for singletenancy and multitenancy
	switch {
	case strings.EqualFold(cniType, multitenancy):
		i.cniType = fmt.Sprintf("%s-%s", cni, multitenancy)
	case strings.EqualFold(cniType, singletenancy):
		i.cniType = cni
	default:
		return fmt.Errorf("No CNI type supplied, please set %q env to either %q or %q and try again", envCNITYPE, singletenancy, multitenancy)
	}
	return nil
}

func (i *installerConfig) SetCNIMode(cniMode string) error {
	// get paths for singletenancy and multitenancy
	if cniMode != "" {
		if strings.EqualFold(cniMode, transparent) || strings.EqualFold(cniMode, bridge) {
			i.cniMode = cniMode
			return nil
		}

		return fmt.Errorf("No CNI mode supplied, please set %q env to either %q or %q and try again", envCNIMode, transparent, bridge)
	}
	return nil
}

type rawConflist struct {
	Name       string        `json:"name"`
	CniVersion string        `json:"cniVersion"`
	Plugins    []interface{} `json:"plugins"`
}

var (
	version string
)

func install(envs installerConfig) error {
	if _, err := os.Stat(envs.dstBinDir); os.IsNotExist(err) {
		os.MkdirAll(envs.dstBinDir, binPerm)
	} else if err != nil {
		return fmt.Errorf("Failed to create destination bin %v directory: %v", envs.dstBinDir, err)
	}

	if _, err := os.Stat(envs.dstConflistDir); os.IsNotExist(err) {
		os.MkdirAll(envs.dstConflistDir, conflistPerm)
	} else if err != nil {
		return fmt.Errorf("Failed to create destination conflist %v directory: %v with err %v", envs.dstConflistDir, envs.dstBinDir, err)
	}

	binaries, conflists, err := getFiles(envs.srcDir)
	if err != nil {
		return fmt.Errorf("Failed to get CNI related file paths with err: %v", err)
	}

	err = copyBinaries(binaries, envs, binPerm)
	if err != nil {
		return fmt.Errorf("Failed to copy CNI binaries with err: %v", err)
	}

	for _, conf := range conflists {
		err = modifyConflists(conf, envs, conflistPerm)
		if err != nil {
			return err
		}
	}

	if version == "" {
		version = "[No version set]"
	}

	fmt.Printf("Successfully installed Azure CNI %s and binaries to %s and conflist to %s\n", version, envs.dstBinDir, envs.dstConflistDir)
	return nil
}

func modifyConflists(conflistpath string, envs installerConfig, perm os.FileMode) error {
	jsonFile, err := os.Open(conflistpath)
	if err != nil {
		return err
	}
	defer jsonFile.Close()

	var conflist rawConflist
	byteValue, err := ioutil.ReadAll(jsonFile)
	if err != nil {
		return err
	}

	err = json.Unmarshal(byteValue, &conflist)
	if err != nil {
		return err
	}

	// if we need to modify the conflist from env's do it here
	if envs.ipamType != "" || envs.cniMode != "" {
		confmap, err := modifyConf(conflist.Plugins[0], envs)
		if err != nil {
			return err
		}

		conflist.Plugins[0] = confmap

		pretty, _ := json.MarshalIndent(conflist, "", "  ")
		fmt.Printf("Modified conflist from envs:\n-------\n%+v\n-------\n", string(pretty))
	}

	// get target path
	dstFile := envs.dstConflistDir + filepath.Base(conflistpath)
	filebytes, err := json.MarshalIndent(conflist, "", "\t")
	if err != nil {
		return err
	}

	fmt.Printf("Installing %v...\n", dstFile)
	return ioutil.WriteFile(dstFile, filebytes, perm)
}

func modifyConf(conf interface{}, envs installerConfig) (interface{}, error) {
	mapbytes, err := json.Marshal(conf)
	if err != nil {
		return nil, err
	}

	netconfig := ccn.NetworkConfig{}
	if err := json.Unmarshal(mapbytes, &netconfig); err != nil {
		return nil, err
	}

	// change the netconfig from passed envs
	netconfig.Ipam.Type = envs.ipamType
	netconfig.Mode = envs.cniMode

	netconfigbytes, _ := json.Marshal(netconfig)
	var rawConfig interface{}
	if err := json.Unmarshal(netconfigbytes, &rawConfig); err != nil {
		return nil, err
	}

	return rawConfig, nil
}
