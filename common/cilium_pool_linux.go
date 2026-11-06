package common

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

var errEmptyBootID = errors.New("empty kernel boot ID")

func validateCiliumPoolPaths(configPath, stateDir string) error {
	for _, path := range []string{configPath, stateDir} {
		for {
			info, err := os.Lstat(path)
			if err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("reading cilium pool path metadata: %w", err)
			}
			if err == nil {
				stat, ok := info.Sys().(*syscall.Stat_t)
				if !ok || stat.Uid != 0 || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o022 != 0 {
					return fmt.Errorf("%w: path must be root-owned, not symlinked or group/other writable: %s", errCiliumPoolConfig, path)
				}
			}
			parent := filepath.Dir(path)
			if parent == path {
				break
			}
			path = parent
		}
	}
	return nil
}

func CiliumPoolBootID() (string, error) {
	raw, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return "", fmt.Errorf("reading kernel boot ID: %w", err)
	}
	id := strings.TrimSpace(string(raw))
	if id == "" {
		return "", errEmptyBootID
	}
	return id, nil
}
