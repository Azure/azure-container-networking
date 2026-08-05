//go:build windows
// +build windows

package network

// setStubMasterResolution is a no-op on Windows: the secondary endpoint client
// master-resolution path is Linux-only. Provided so cross-platform tests in
// endpoint_test.go compile and run on Windows.
func setStubMasterResolution() (restore func()) {
	return func() {}
}
