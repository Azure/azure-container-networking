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
		return "", errors.New("linux boot ID is empty")
	}

	return id, nil
}
