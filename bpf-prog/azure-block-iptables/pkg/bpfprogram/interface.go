package bpfprogram

// Attacher defines the interface for BPF program attachment operations.
// This interface allows for dependency injection and easier testing with mock implementations.
type Attacher interface {
	// Attach attaches the BPF program to LSM hooks and pins the links and maps
	Attach() error

	// Unpins the links and maps (causes detachment)
	Detach() error

	// IsAttached returns true if the BPF program is currently attached
	IsAttached() bool

	// Close cleans up all resources
	Close()
}

// AttacherFactory defines a function type for creating Attacher instances.
// This allows for easier dependency injection in applications.
type AttacherFactory func() Attacher
