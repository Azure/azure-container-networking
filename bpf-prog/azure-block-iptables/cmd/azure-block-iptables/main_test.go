//go:build linux
// +build linux

package main

import (
	"testing"

	"github.com/Azure/azure-container-networking/bpf-prog/azure-block-iptables/pkg/bpfprogram"
)

func TestHandleFileEventWithMock(t *testing.T) {
	// Create a mock Attacher
	mockAttacher := bpfprogram.NewMockProgram()

	// Test cases
	testCases := []struct {
		name           string
		mode           string
		expectedAttach int
		expectedDetach int
	}{
		{
			name:           "test attach mode",
			mode:           "attach",
			expectedAttach: 1,
			expectedDetach: 0,
		},
		{
			name:           "test detach mode",
			mode:           "detach",
			expectedAttach: 0,
			expectedDetach: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset mock state
			mockAttacher.Reset()

			run(&Config{Mode: tc.mode, AttacherFactory: func() bpfprogram.Attacher { return mockAttacher }})

			// Verify expectations
			if mockAttacher.AttachCallCount() != tc.expectedAttach {
				t.Errorf("Expected %d attach calls, got %d", tc.expectedAttach, mockAttacher.AttachCallCount())
			}

			if mockAttacher.DetachCallCount() != tc.expectedDetach {
				t.Errorf("Expected %d detach calls, got %d", tc.expectedDetach, mockAttacher.DetachCallCount())
			}
		})
	}
}
