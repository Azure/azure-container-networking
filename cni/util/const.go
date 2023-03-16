package util

type ExecutionMode string

// CNI execution modes
const (
	Default   ExecutionMode = "default"
	Baremetal ExecutionMode = "baremetal"
	V4Swift   ExecutionMode = "v4swift"
)

type IpamMode string

// IPAM modes
const (
	V4Overlay        IpamMode = "v4overlay"
	DualStackOverlay IpamMode = "dualStackOverlay"
)
