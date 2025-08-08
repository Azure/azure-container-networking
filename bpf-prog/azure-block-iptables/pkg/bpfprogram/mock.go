package bpfprogram

import "log"

// MockProgram provides a mock implementation of the Manager interface for testing.
type MockProgram struct {
	attached     bool
	attachError  error
	detachError  error
	attachCalled int
	detachCalled int
}

// NewMockProgram creates a new mock BPF program manager instance.
func NewMockProgram() *MockProgram {
	return &MockProgram{}
}

// SetAttachError sets the error to return from Attach() calls.
func (m *MockProgram) SetAttachError(err error) {
	m.attachError = err
}

// SetDetachError sets the error to return from Detach() calls.
func (m *MockProgram) SetDetachError(err error) {
	m.detachError = err
}

// AttachCallCount returns the number of times Attach() was called.
func (m *MockProgram) AttachCallCount() int {
	return m.attachCalled
}

// DetachCallCount returns the number of times Detach() was called.
func (m *MockProgram) DetachCallCount() int {
	return m.detachCalled
}

// Reset resets the mock's state.
func (m *MockProgram) Reset() {
	m.attached = false
	m.attachError = nil
	m.detachError = nil
	m.attachCalled = 0
	m.detachCalled = 0
}

// Attach simulates attaching the BPF program.
func (m *MockProgram) Attach() error {
	m.attachCalled++

	if m.attachError != nil {
		return m.attachError
	}

	if m.attached {
		log.Println("Mock: BPF program already attached")
		return nil
	}

	log.Println("Mock: Attaching BPF program...")
	m.attached = true
	log.Println("Mock: BPF program attached successfully")
	return nil
}

// Detach simulates detaching the BPF program.
func (m *MockProgram) Detach() error {
	m.detachCalled++

	if m.detachError != nil {
		return m.detachError
	}

	if !m.attached {
		log.Println("Mock: BPF program already detached")
		return nil
	}

	log.Println("Mock: Detaching BPF program...")
	m.attached = false
	log.Println("Mock: BPF program detached successfully")
	return nil
}

// IsAttached returns the mock's attached state.
func (m *MockProgram) IsAttached() bool {
	return m.attached
}

// Close simulates cleanup.
func (m *MockProgram) Close() {
	if err := m.Detach(); err != nil {
		log.Println("Mock: Error detaching BPF program:", err)
	}
}
