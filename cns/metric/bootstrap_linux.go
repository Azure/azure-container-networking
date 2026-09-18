// Copyright 2024 Microsoft. All rights reserved.
// MIT License

package metric

import (
	"os"
	"strings"
)

// readBootID returns the kernel boot identifier, a UUID stable
// across the life of a single kernel boot. Empty string indicates
// the value could not be read; callers treat that as "unknown".
func readBootID() string {
	b, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
