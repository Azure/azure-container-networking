package util

type ExecutionMode string

// CNI execution modes
const (
	Default       ExecutionMode = "default"
	Baremetal     ExecutionMode = "baremetal"
	AKSSwift      ExecutionMode = "aksswift"
	V4OnlyOverlay ExecutionMode = "v4OnlyOverlay"
)
