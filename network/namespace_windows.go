// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package network

import (
	"errors"
)

var errWindowsImpl = errors.New("windows impl err")

// Namespace represents a network namespace.
type Namespace struct{}

type NamespaceClient struct{}

func NewNamespaceClient() *NamespaceClient {
	return &NamespaceClient{}
}

// OpenNamespace creates a new namespace object for the given netns path.
func (c *NamespaceClient) OpenNamespace(_ string) (NamespaceInterface, error) {
	return nil, errWindowsImpl
}

// GetCurrentThreadNamespace returns the caller thread's current namespace.
func (c *NamespaceClient) GetCurrentThreadNamespace() (NamespaceInterface, error) {
	return c.OpenNamespace("")
}

// Close releases the resources associated with the namespace object.
func (ns *Namespace) Close() error {
	return nil
}

// GetFd returns the file descriptor of the namespace.
func (ns *Namespace) GetFd() uintptr {
	return 0
}

func (ns *Namespace) GetName() string {
	return "windows impl"
}

// Enter puts the caller thread inside the namespace.
func (ns *Namespace) Enter() error {
	return nil
}

// Exit puts the caller thread to its previous namespace.
func (ns *Namespace) Exit() error {
	return nil
}
