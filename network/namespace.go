package network

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
