package utils

import (
	"io/ioutil"
	"os"
)

// WriteFile provide one way to write the file in one transaction.
// or when the process crash or the system reboot, we will have one incomplete file.
func WriteFile(dstFile string, b []byte, perm os.FileMode) error {
	tempFilePath := fmt.Sprintf("%s.tmp", dstFile)
	err := ioutil.WriteFile(tempFilePath, b, perm)
	if err != nil {
		return err
	}

	return os.Rename(tempFilePath, dstFile)
}
