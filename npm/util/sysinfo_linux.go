package util

// this file has bits copied from github.com/zcalusic/sysinfo (v1.1.2)

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const kernelReleaseFilepath = "/proc/sys/kernel/osrelease"

var errNoKernelRelease = errors.New("error finding kernel release")

func KernelReleaseMajorVersion() (int, error) {
	rel, err := slurpFile(kernelReleaseFilepath)
	if err != nil {
		return 0, err
	}

	majorString := strings.Split(rel, ".")[0]
	if majorString == "" {
		return 0, errNoKernelRelease
	}

	majorInt, err := strconv.Atoi(majorString)
	if err != nil {
		return 0, fmt.Errorf("failed to convert kernel major version to int. version: %s. err: %w", majorString, err)
	}

	return majorInt, nil
}

// Read one-liner text files, strip newline.
func slurpFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file. file: %s. err: %w", path, err)
	}

	return strings.TrimSpace(string(data)), nil
}
