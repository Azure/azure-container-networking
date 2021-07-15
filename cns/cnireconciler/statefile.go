package cnireconciler

import (
	"encoding/json"
	"errors"
	"os"

	"github.com/Azure/azure-container-networking/cns/logger"
)

// WriteObjectToCNIStatefile checks for a file at the CNI statefile path,
// and checks if it is empty. If it is empty, writes an empty JSON object to
// it so older CNI can execute. Does nothing and returns no error if the
// file does not exist.
//
// This is a hack to get older CNI to run when CNS has mounted the statefile
// path, but the statefile wasn't written by CNI yet. Kubelet will stub an
// empty file on the host filesystem, crashing older CNI because it doesn't know
// how to handle empty statefiles.
func WriteObjectToCNIStatefile() error {
	filename := "/var/run/azure-vnet.json"
	return writeObjectToFile(filename)
}

func writeObjectToFile(filename string) error {
	fi, err := os.Stat(filename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			logger.Printf("CNI statefile %s does not exist", filename)
			return nil
		}
		return err
	}

	if fi.Size() != 0 {
		return nil
	}

	logger.Printf("Writing {} to CNI statefile %s", filename)
	b, _ := json.Marshal(map[string]string{})
	return os.WriteFile(filename, b, os.FileMode(0666))
}
