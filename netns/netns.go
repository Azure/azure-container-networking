//go:build linux
// +build linux

package netns

import (
	"github.com/pkg/errors"
	"github.com/vishvananda/netns"
)

type Netns struct{}

func New() *Netns {
	return &Netns{}
}

func (f *Netns) Get() (int, error) {
	nsHandle, err := netns.Get()
	return int(nsHandle), errors.Wrap(err, "netns impl")
}

func (f *Netns) GetFromName(name string) (int, error) {
	nsHandle, err := netns.GetFromName(name)
	return int(nsHandle), errors.Wrap(err, "netns impl")
}

func (f *Netns) Set(fileDescriptor int) error {
	return errors.Wrap(netns.Set(netns.NsHandle(fileDescriptor)), "netns impl")
}

func (f *Netns) NewNamed(name string) (int, error) {
	nsHandle, err := netns.NewNamed(name)
	return int(nsHandle), errors.Wrap(err, "netns impl")
}

func (f *Netns) DeleteNamed(name string) error {
	return errors.Wrap(netns.DeleteNamed(name), "netns impl")
}

func (f *Netns) IsNamespaceEqual(fd1, fd2 int) bool {
	return netns.NsHandle(fd1).Equal(netns.NsHandle(fd2))
}

func (f *Netns) NamespaceUniqueID(fd int) string {
	return netns.NsHandle(fd).UniqueId()
}
