// +build windows

package iptm

import "os"

// used for building ACN cli on windows, since this util is called by a struct which is used a response
// for the debug API
func grabIptablesFileLock(f *os.File) error {
	return nil
}
