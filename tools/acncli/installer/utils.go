//go:build !ignore_uncovered
// +build !ignore_uncovered

package installer

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	c "github.com/Azure/azure-container-networking/tools/acncli/api"
)

func SetOrUseDefault(setValue, defaultValue string) string {
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
				if ext == c.ConflistExtension {
					conflists = append(conflists, path)
				} else {
					binaries = append(binaries, path)
				}

			}

			return nil
		})

	return
}

func copyBinaries(filePaths []string, installerConf InstallerConfig, perm os.FileMode) error {
	for _, path := range filePaths {
		fileName := filepath.Base(path)

		if exempt, ok := installerConf.ExemptBins[fileName]; ok && exempt {
			fmt.Printf("Skipping %s, marked as exempt\n", fileName)
		} else {
			fmt.Printf("🚚 - Installing %v...\n", installerConf.DstBinDir+fileName)
			err := copyFile(path, installerConf.DstBinDir+fileName, perm)
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
