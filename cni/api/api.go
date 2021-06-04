package api

import "net"

type NetworkInterfaceInfo struct {
	PodInterfaceID string
	ContainerID    string
	IPAddresses    []net.IPNet
}

type AzureCNIState struct {
	ContainerInterfaces map[string]NetworkInterfaceInfo
}
