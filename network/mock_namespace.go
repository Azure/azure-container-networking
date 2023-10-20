// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package network

import "github.com/pkg/errors"

var errMockEnterNamespaceFailure = errors.New("failed to enter namespace")

const failToEnterNamespaceName = "failns"

type MockNamespace struct {
	namespace string
}

type MockNamespaceClient struct{}

func NewMockNamespaceClient() *MockNamespaceClient {
	return &MockNamespaceClient{}
}

// OpenNamespace creates a new namespace object for the given netns path.
func (c *MockNamespaceClient) OpenNamespace(ns string) (NamespaceInterface, error) {
	if ns == "" {
		return nil, errFileNotExist
	}
	return &MockNamespace{namespace: ns}, nil
}

// GetCurrentThreadNamespace returns the caller thread's current namespace.
func (c *MockNamespaceClient) GetCurrentThreadNamespace() (NamespaceInterface, error) {
	return c.OpenNamespace("")
}

// Close releases the resources associated with the namespace object.
func (ns *MockNamespace) Close() error {
	return nil
}

// GetFd returns the file descriptor of the namespace.
func (ns *MockNamespace) GetFd() uintptr {
	return 1
}

func (ns *MockNamespace) GetName() string {
	return ns.namespace
}

// Enter puts the caller thread inside the namespace.
func (ns *MockNamespace) Enter() error {
	if ns.namespace == failToEnterNamespaceName {
		return errMockEnterNamespaceFailure
	}
	return nil
}

// Exit puts the caller thread to its previous namespace.
func (ns *MockNamespace) Exit() error {
	return nil
}
