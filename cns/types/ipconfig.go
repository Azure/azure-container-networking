package types

// IPState defines the possible states an IP can be in for CNS IPAM.
type IPState string

const (
	// Available IPConfigState for IPs available for CNS to use.
	// The total pool that CNS has access to are its "allocated" IPs.
	Available IPState = "Available"
	// Assigned IPConfigState for IPs that CNS has assigned to Pods.
	Assigned IPState = "Assigned"
	// PendingRelease IPConfigState for IPs pending release.
	PendingRelease IPState = "PendingRelease"
	// PendingProgramming IPConfigState for IPs pending programming.
	PendingProgramming IPState = "PendingProgramming"
)
