package bpfprogram

import (
	"errors"
	"testing"
)

// TestMockProgram tests the mock implementation
func TestMockProgram(t *testing.T) {
	mock := NewMockProgram()

	// Test initial state
	if mock.IsAttached() {
		t.Error("Mock should start in detached state")
	}

	if mock.AttachCallCount() != 0 {
		t.Error("Attach call count should start at 0")
	}

	if mock.DetachCallCount() != 0 {
		t.Error("Detach call count should start at 0")
	}

	// Test attach
	err := mock.Attach()
	if err != nil {
		t.Errorf("Attach should succeed: %v", err)
	}

	if !mock.IsAttached() {
		t.Error("Mock should be attached after Attach()")
	}

	if mock.AttachCallCount() != 1 {
		t.Errorf("Expected 1 attach call, got %d", mock.AttachCallCount())
	}

	// Test attach when already attached
	err = mock.Attach()
	if err != nil {
		t.Errorf("Attach should succeed even when already attached: %v", err)
	}

	if mock.AttachCallCount() != 2 {
		t.Errorf("Expected 2 attach calls, got %d", mock.AttachCallCount())
	}

	// Test detach
	err = mock.Detach()
	if err != nil {
		t.Errorf("Detach should succeed: %v", err)
	}

	if mock.IsAttached() {
		t.Error("Mock should be detached after Detach()")
	}

	if mock.DetachCallCount() != 1 {
		t.Errorf("Expected 1 detach call, got %d", mock.DetachCallCount())
	}

	// Test detach when already detached
	err = mock.Detach()
	if err != nil {
		t.Errorf("Detach should succeed even when already detached: %v", err)
	}

	if mock.DetachCallCount() != 2 {
		t.Errorf("Expected 2 detach calls, got %d", mock.DetachCallCount())
	}

	// Test Close
	mock.Close()
	if mock.IsAttached() {
		t.Error("Mock should be detached after Close()")
	}
}

// TestMockProgramErrors tests error scenarios
func TestMockProgramErrors(t *testing.T) {
	mock := NewMockProgram()

	// Test attach error
	expectedErr := errors.New("attach error")
	mock.SetAttachError(expectedErr)

	err := mock.Attach()
	if err != expectedErr {
		t.Errorf("Expected attach error %v, got %v", expectedErr, err)
	}

	if mock.IsAttached() {
		t.Error("Mock should not be attached after failed attach")
	}

	// Test detach error
	mock.SetAttachError(nil) // Clear attach error
	mock.Attach()            // Successful attach

	expectedErr = errors.New("detach error")
	mock.SetDetachError(expectedErr)

	err = mock.Detach()
	if err != expectedErr {
		t.Errorf("Expected detach error %v, got %v", expectedErr, err)
	}

	// State should remain unchanged on error
	if !mock.IsAttached() {
		t.Error("Mock should still be attached after failed detach")
	}
}

// TestMockProgramReset tests the reset functionality
func TestMockProgramReset(t *testing.T) {
	mock := NewMockProgram()

	// Perform some operations
	mock.SetAttachError(errors.New("test"))
	mock.SetDetachError(errors.New("test"))
	mock.Attach()
	mock.Detach()

	// Reset
	mock.Reset()

	// Verify reset state
	if mock.IsAttached() {
		t.Error("Mock should be detached after reset")
	}

	if mock.AttachCallCount() != 0 {
		t.Errorf("Attach call count should be 0 after reset, got %d", mock.AttachCallCount())
	}

	if mock.DetachCallCount() != 0 {
		t.Errorf("Detach call count should be 0 after reset, got %d", mock.DetachCallCount())
	}

	// Errors should be cleared
	err := mock.Attach()
	if err != nil {
		t.Errorf("Attach should succeed after reset: %v", err)
	}

	err = mock.Detach()
	if err != nil {
		t.Errorf("Detach should succeed after reset: %v", err)
	}
}

// TestManagerFactory tests the factory function type
func TestManagerFactory(t *testing.T) {
	// Test with mock factory
	mockFactory := func() Attacher {
		return NewMockProgram()
	}

	manager := mockFactory()
	if _, ok := manager.(*MockProgram); !ok {
		t.Error("Mock factory should return MockProgram")
	}

	// Test with real factory
	realFactory := NewProgram

	manager = realFactory()
	if _, ok := manager.(*Program); !ok {
		t.Error("Real factory should return Program")
	}
}
