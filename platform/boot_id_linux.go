// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package platform

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

const linuxBootIDPath = "/proc/sys/kernel/random/boot_id"

var errLinuxBootIDEmpty = errors.New("platform: linux boot ID is empty")

type bootIDReader func(string) ([]byte, error)

// BootID returns the identity of the current Linux boot.
func BootID() (string, error) {
	return bootID(os.ReadFile)
}

func bootID(read bootIDReader) (string, error) {
	data, err := read(linuxBootIDPath)
	if err != nil {
		return "", fmt.Errorf("read linux boot ID: %w", err)
	}

	id := strings.TrimSpace(string(data))
	if id == "" {
		return "", errLinuxBootIDEmpty
	}

	return id, nil
}
