//go:build linux
// +build linux

package main

import (
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/bpf-prog/azure-block-iptables/pkg/bpfprogram"
	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
)

func TestHandleFileEventWithMock(t *testing.T) {
	// Create a mock Attacher
	mockAttacher := bpfprogram.NewMockProgram()

	// Create a temporary config file for testing
	configFile := "/tmp/test-iptables-allow-list"

	// Test cases
	testCases := []struct {
		name           string
		setupFile      func(string) error
		expectedAttach int
		expectedDetach int
	}{
		{
			name: "empty file triggers attach",
			setupFile: func(path string) error {
				// Create empty file
				file, err := os.Create(path)
				if err != nil {
					return errors.Wrap(err, "failed to create file")
				}
				return file.Close()
			},
			expectedAttach: 1,
			expectedDetach: 0,
		},
		{
			name: "file with content triggers detach",
			setupFile: func(path string) error {
				// Create file with content
				return os.WriteFile(path, []byte("some content"), 0o600)
			},
			expectedAttach: 0,
			expectedDetach: 1,
		},
		{
			name: "missing file triggers detach",
			setupFile: func(path string) error {
				// Remove file if it exists
				os.Remove(path)
				return nil
			},
			expectedAttach: 0,
			expectedDetach: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset mock state
			mockAttacher.Reset()

			// Setup file state
			if err := tc.setupFile(configFile); err != nil {
				t.Fatalf("Failed to setup file: %v", err)
			}
			defer os.Remove(configFile)

			// Create a fake fsnotify event
			event := fsnotify.Event{
				Name: configFile,
				Op:   fsnotify.Write,
			}

			// Call the function under test
			handleFileEvent(event, configFile, mockAttacher)

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
