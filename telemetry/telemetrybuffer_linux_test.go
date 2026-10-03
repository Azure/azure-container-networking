// Copyright 2018 Microsoft. All rights reserved.
// MIT License

package telemetry

import (
	"path/filepath"
	"testing"
)

func testFDName(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "telemetry.sock")
}
