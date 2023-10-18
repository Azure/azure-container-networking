// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package network

import (
	"fmt"
	"os"
	"runtime"

	"github.com/Azure/azure-container-networking/netlink"

	"golang.org/x/sys/unix"
)

// Namespace represents a network namespace.
type Namespace struct {
	file   *os.File
	prevNs *Namespace
	cli    *NamespaceClient
}

type NamespaceClient struct{}

func NewNamespaceClient() *NamespaceClient {
	return &NamespaceClient{}
}

// OpenNamespace creates a new namespace object for the given netns path.
func (c *NamespaceClient) OpenNamespace(nsPath string) (NamespaceInterface, error) {
	fd, err := os.Open(nsPath)
	if err != nil {
		return nil, err
	}

	return &Namespace{file: fd, cli: c}, nil
}

// GetCurrentThreadNamespace returns the caller thread's current namespace.
func (c *NamespaceClient) GetCurrentThreadNamespace() (NamespaceInterface, error) {
	nsPath := fmt.Sprintf("/proc/%d/task/%d/ns/net", os.Getpid(), unix.Gettid())
	return c.OpenNamespace(nsPath)
}

// Close releases the resources associated with the namespace object.
func (ns *Namespace) Close() error {
	if ns.file == nil {
		return nil
	}

	err := ns.file.Close()
	if err != nil {
		return fmt.Errorf("failed to close namespace %v, err:%w", ns.file.Name(), err)
	}

	ns.file = nil

	return nil
}

// GetFd returns the file descriptor of the namespace.
func (ns *Namespace) GetFd() uintptr {
	return ns.file.Fd()
}

func (ns *Namespace) GetName() string {
	return ns.file.Name()
}

// Set sets the current namespace.
func (ns *Namespace) set() error {
	_, _, err := unix.Syscall(unix.SYS_SETNS, ns.file.Fd(), uintptr(unix.CLONE_NEWNET), 0)
	if err != 0 {
		return fmt.Errorf("failed to set namespace %v, err:%w", ns.file.Name(), err)
	}

	return nil
}

// Enter puts the caller thread inside the namespace.
func (ns *Namespace) Enter() error {
	currentNs, err := ns.cli.GetCurrentThreadNamespace()
	if err != nil {
		return err
	}

	ns.prevNs = currentNs.(*Namespace)

	runtime.LockOSThread()

	err = ns.set()
	if err != nil {
		runtime.UnlockOSThread()
		return err
	}

	// Recycle the netlink socket for the new network namespace.
	netlink.ResetSocket()

	return nil
}

// Exit puts the caller thread to its previous namespace.
func (ns *Namespace) Exit() error {
	err := ns.prevNs.set()
	if err != nil {
		return err
	}

	ns.prevNs.Close()
	ns.prevNs = nil

	runtime.UnlockOSThread()

	// Recycle the netlink socket for the new network namespace.
	netlink.ResetSocket()

	return nil
}
