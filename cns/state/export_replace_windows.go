// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package state

import (
	"fmt"

	"github.com/Azure/azure-container-networking/platform"
)

func durableReplace(source, destination string) error {
	return durableReplaceWith(source, destination, platform.ReplaceFile)
}

func durableReplaceWith(
	source string,
	destination string,
	replace func(string, string) error,
) error {
	if err := replace(source, destination); err != nil {
		return fmt.Errorf("write-through replace: %w", err)
	}
	return nil
}
