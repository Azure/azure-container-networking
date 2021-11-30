package types

// IPState defines the possible states an IP can be in for CNS IPAM.
// The total pool that CNS has access to are its "allocated" IPs.
type IPState string

const (
	// Available IPConfigState for allocated IPs available for CNS to use.
	Available IPState = "Available"
	// Assigned IPConfigState for allocated IPs that CNS has assigned to Pods.
	Assigned IPState = "Assigned"
	// PendingRelease IPConfigState for allocated IPs pending deallocation.
	PendingRelease IPState = "PendingRelease"
	// PendingProgramming IPConfigState for allocated IPs pending programming.
	PendingProgramming IPState = "PendingProgramming"
)
