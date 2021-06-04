package api

import "net"

type NetworkInterfaceInfo struct {
	PodName        string // todo: populate these in cni
	PodNamespace   string
	PodInterfaceID string
	ContainerID    string
	IPAddresses    []net.IPNet
}

type AzureCNIState struct {
	ContainerInterfaces map[string]NetworkInterfaceInfo
}
