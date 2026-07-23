// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package platform

import (
	"fmt"
	"strconv"

	"golang.org/x/sys/windows/registry"
)

const (
	windowsBootIDRegistryPath = `SYSTEM\CurrentControlSet\Control\Session Manager\Memory Management\PrefetchParameters`
	windowsBootIDValueName    = "BootId"
)

type bootIDQuery func() (uint64, error)

// BootID returns the identity of the current Windows boot.
func BootID() (string, error) {
	return bootID(queryBootIDRegistry)
}

func queryBootIDRegistry() (uint64, error) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, windowsBootIDRegistryPath, registry.READ)
	if err != nil {
		return 0, err
	}
	defer key.Close()

	id, _, err := key.GetIntegerValue(windowsBootIDValueName)
	return id, err
}

func bootID(query bootIDQuery) (string, error) {
	id, err := query()
	if err != nil {
		return "", fmt.Errorf("query windows boot ID: %w", err)
	}

	return strconv.FormatUint(id, 10), nil
}
