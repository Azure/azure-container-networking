// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package state

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDurableReplaceWindowsUsesWriteThroughReplacement(t *testing.T) {
	var source, destination string
	err := durableReplaceWith("source", "destination", func(gotSource, gotDestination string) error {
		source = gotSource
		destination = gotDestination
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, "source", source)
	assert.Equal(t, "destination", destination)

	injected := errors.New("replace failure")
	err = durableReplaceWith("source", "destination", func(string, string) error {
		return injected
	})
	require.ErrorIs(t, err, injected)
}
