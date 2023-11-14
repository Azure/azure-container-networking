package network

import "errors"

var errFileNotExist = errors.New("no such file or directory")

type NamespaceInterface interface {
	GetFd() uintptr
	GetName() string
	Enter() error
	Exit() error
	Close() error
}

type NamespaceClientInterface interface {
	OpenNamespace(nsPath string) (NamespaceInterface, error)
	GetCurrentThreadNamespace() (NamespaceInterface, error)
}
