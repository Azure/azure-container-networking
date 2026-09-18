// Copyright 2024 Microsoft. All rights reserved.
// MIT License

package metric

// readBootID is unimplemented on Windows in v1; CNS will record
// boot state as "unknown" until a Windows-specific implementation
// is added (e.g., via Win32_OperatingSystem.LastBootUpTime or
// kernel session ID).
func readBootID() string {
	return ""
}
