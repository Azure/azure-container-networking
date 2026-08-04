// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package platform

import (
	"errors"
	"fmt"
	"strconv"

	"golang.org/x/sys/windows/registry"
)

const (
	windowsBootIDRegistryPath = `SYSTEM\CurrentControlSet\Control\Session Manager\Memory Management\PrefetchParameters`
	windowsBootIDValueName    = "BootId"
)

var errWindowsBootIDNotDWORD = errors.New("platform: boot ID registry value is not a DWORD")

type bootIDQuery func() (uint64, uint32, error)

// BootID returns the identity of the current Windows boot.
func BootID() (string, error) {
	return bootID(queryBootIDRegistry)
}

func queryBootIDRegistry() (id uint64, valueType uint32, err error) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, windowsBootIDRegistryPath, registry.QUERY_VALUE)
	if err != nil {
		return 0, 0, fmt.Errorf("open boot ID registry key: %w", err)
	}
	defer key.Close()

	id, valueType, err = key.GetIntegerValue(windowsBootIDValueName)
	if err != nil {
		return 0, 0, fmt.Errorf("get boot ID registry value: %w", err)
	}

	return id, valueType, nil
}

func bootID(query bootIDQuery) (string, error) {
	id, valueType, err := query()
	if err != nil {
		return "", fmt.Errorf("query windows boot ID: %w", err)
	}
	if valueType != registry.DWORD {
		return "", fmt.Errorf("%w: %d", errWindowsBootIDNotDWORD, valueType)
	}

	return strconv.FormatUint(id, 10), nil
}
