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
	Overlay          IpamMode = "overlay" // Nothing changes between 'v4overlay' and 'dualStackOverlay' mode, so consolidating to one
	Nodesubnet       IpamMode = "nodesubnet"
)

// Overlay consolidation plan
// First, we have v4overlay and dualstackoverlay conflists both have just 'overlay' in them
// Next, we release this CNI and conflist in AKS
// Next we will add a third 'overlay' conflist generator in CNS
// Release this CNS image
// Change AKS RP to use 'overlay' option for CNS configmap, for both v4overlay and dualstackoverlay
// Remove 'v4overlay' and 'dualstackoverlay' from ACN completely
