// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package state

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Azure/azure-container-networking/platform"
)

type syncDirectory interface {
	Sync() error
	Close() error
}

func durableReplace(source, destination string) error {
	return durableReplaceWith(source, destination, platform.ReplaceFile, func(path string) (syncDirectory, error) {
		return os.Open(path)
	})
}

func durableReplaceWith(
	source string,
	destination string,
	replace func(string, string) error,
	openDirectory func(string) (syncDirectory, error),
) error {
	if err := replace(source, destination); err != nil {
		return fmt.Errorf("atomic replace: %w", err)
	}
	parentPath := filepath.Dir(destination)
	parent, err := openDirectory(parentPath)
	if err != nil {
		return fmt.Errorf("opening parent directory %q: %w", parentPath, err)
	}
	if err := parent.Sync(); err != nil {
		_ = parent.Close()
		return fmt.Errorf("syncing parent directory %q: %w", parentPath, err)
	}
	if err := parent.Close(); err != nil {
		return fmt.Errorf("closing parent directory %q: %w", parentPath, err)
	}
	return nil
}
