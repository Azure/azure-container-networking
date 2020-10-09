package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

func getInstallerConfigFromEnv() (installerConfig, error) {
	osType := os.Getenv(envCNIOS)
	cniType := os.Getenv(envCNITYPE)
	srcDirectory := os.Getenv(envCNISourceDir)
	dstBinDirectory := os.Getenv(envCNIDestinationBinDir)
	dstConflistDirectory := os.Getenv(envCNIDestinationConflistDir)
	ipamType := os.Getenv(envCNIIPAMType)
	envCNIExemptBins := os.Getenv(envCNIExemptBins)
	cniLogFile := os.Getenv(envCNILogFile)

	envs := installerConfig{
		exemptBins: make(map[string]bool),
	}

	// only allow windows and linux binaries
	if err := envs.SetOSType(osType); err != nil {
		return envs, err
	}

	// only allow windows and linux binaries
	if err := envs.SetCNIType(cniType); err != nil {
		return envs, err
	}

	envs.SetExempt(strings.Split(strings.Replace(strings.ToLower(envCNIExemptBins), " ", "", -1), ","))

	envs.srcDir = fmt.Sprintf("%s%s/%s/", setOrUseDefault(srcDirectory, defaultSrcDirLinux), envs.osType, envs.cniType)
	envs.dstBinDir = setOrUseDefault(dstBinDirectory, defaultBinDirLinux)
	envs.dstConflistDir = setOrUseDefault(dstConflistDirectory, defaultConflistDirLinux)
	envs.logFile = setOrUseDefault(cniLogFile, defaultLogFile)
	envs.ipamType = ipamType

	return envs, nil
}

func setOrUseDefault(setValue, defaultValue string) string {
	if setValue == "" {
		setValue = defaultValue
	}
	return setValue
}

func getFiles(path string) (binaries []string, conflists []string, err error) {
	err = filepath.Walk(path,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return fmt.Errorf("Failed to traverse path %s with err %s", path, err)
			}

			if !info.IsDir() {
				ext := filepath.Ext(path)
				if ext == conflistExtension {
					conflists = append(conflists, path)
				} else {
					binaries = append(binaries, path)
				}

			}

			return nil
		})

	return
}

func copyBinaries(filePaths []string, envs installerConfig, perm os.FileMode) error {
	for _, path := range filePaths {
		fileName := filepath.Base(path)

		if exempt, ok := envs.exemptBins[fileName]; ok && exempt {
			fmt.Printf("Skipping %s, marked as exempt\n", fileName)
		} else {
			err := copyFile(path, envs.dstBinDir+fileName, perm)
			fmt.Printf("Installing %v...\n", envs.dstBinDir+fileName)
			if err != nil {
				return err
			}
		}

	}

	return nil
}

func copyFile(src string, dst string, perm os.FileMode) error {
	data, err := ioutil.ReadFile(src)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(dst, data, perm)
	if err != nil {
		return err
	}

	return nil
}
