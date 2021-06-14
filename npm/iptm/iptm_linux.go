// +build linux

package iptm

import (
	"golang.org/x/sys/unix"
	"os"
)

func grabIptablesFileLock(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
}
