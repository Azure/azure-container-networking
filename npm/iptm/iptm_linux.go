// +build linux

package iptm

import (
	"os"

	"golang.org/x/sys/unix"
)

func grabIptablesFileLock(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
}
