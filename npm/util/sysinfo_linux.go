package util

// this file has bits copied from github.com/zcalusic/sysinfo (v1.1.2)

import (
	"os"
	"strings"
)

func KernelRelease() string {
	return slurpFile("/proc/sys/kernel/osrelease")
}

// Read one-liner text files, strip newline.
func slurpFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(data))
}
