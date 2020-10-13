package installer

import (
	"fmt"
	"os"
	"strings"

	c "github.com/Azure/azure-container-networking/acncli/api"
)

type InstallerConfig struct {
	SrcDir         string
	DstBinDir      string
	DstConflistDir string
	IPAMType       string
	ExemptBins     map[string]bool
	OSType         string
	CNITenancy     string
	CNIMode        string
}

func (i *InstallerConfig) SetExempt(exempt []string) {
	// set exempt binaries to skip installing
	// convert to all lower case, strip whitespace, and split on comma
	for _, binName := range exempt {
		i.ExemptBins[binName] = true
	}
}

func (i *InstallerConfig) SetOSType(osType string) error {
	if strings.EqualFold(osType, c.Linux) || strings.EqualFold(osType, c.Windows) {
		i.OSType = fmt.Sprintf("%s_%s", osType, c.Amd64)
	} else {
		return fmt.Errorf("Invalid target OS supplied: %s", osType)
	}
	return nil
}

func (i *InstallerConfig) SetCNIType(cniType string) error {
	// get paths for singletenancy and multitenancy
	switch {
	case strings.EqualFold(cniType, c.Multitenancy):
		i.CNITenancy = fmt.Sprintf("%s-%s", c.CNI, c.Multitenancy)
	case strings.EqualFold(cniType, c.Singletenancy):
		i.CNITenancy = c.CNI
	default:
		return fmt.Errorf("No CNI type supplied, please use %q or %q and try again", c.Transparent, c.Bridge)
	}
	return nil
}

func (i *InstallerConfig) SetCNIDatapathMode(cniMode string) error {
	// get paths for singletenancy and multitenancy
	if cniMode != "" {
		if strings.EqualFold(cniMode, c.Transparent) || strings.EqualFold(cniMode, c.Bridge) {
			i.CNIMode = cniMode
			return nil
		}

		return fmt.Errorf("No CNI mode supplied, please use %q or %q and try again", c.Transparent, c.Bridge)
	}
	return nil
}

func InstallLocal(installerConf InstallerConfig) error {
	if _, err := os.Stat(installerConf.DstBinDir); os.IsNotExist(err) {
		os.MkdirAll(installerConf.DstBinDir, c.BinPerm)
	} else if err != nil {
		return fmt.Errorf("Failed to create destination bin %v directory: %v", installerConf.DstBinDir, err)
	}

	if _, err := os.Stat(installerConf.DstConflistDir); os.IsNotExist(err) {
		os.MkdirAll(installerConf.DstConflistDir, c.ConflistPerm)
	} else if err != nil {
		return fmt.Errorf("Failed to create destination conflist %v directory: %v with err %v", installerConf.DstConflistDir, installerConf.DstBinDir, err)
	}

	binaries, conflists, err := getFiles(installerConf.SrcDir)
	if err != nil {
		return fmt.Errorf("Failed to get CNI related file paths with err: %v", err)
	}

	err = copyBinaries(binaries, installerConf, c.BinPerm)
	if err != nil {
		return fmt.Errorf("Failed to copy CNI binaries with err: %v", err)
	}

	for _, conf := range conflists {
		err = ModifyConflists(conf, installerConf, c.ConflistPerm)
		if err != nil {
			return err
		}
	}

	fmt.Printf("Successfully installed Azure CNI  and binaries to %s and conflist to %s\n", installerConf.DstBinDir, installerConf.DstConflistDir)
	return nil
}

func InstallCluster(installerConf InstallerConfig) error {
	return fmt.Errorf("Not implemented yet")
}
