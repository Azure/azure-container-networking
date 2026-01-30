package fsnotify

import (
	"context"
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// Mock implementation of ReleaseIPsClient for testing
type mockReleaseIPsClient struct {
	releaseIPCalled bool
	containerIDs    []string
	podInterfaceIDs []string
}

func (m *mockReleaseIPsClient) ReleaseIPs(_ context.Context, ipconfig cns.IPConfigsRequest) error {
	m.releaseIPCalled = true
	m.containerIDs = append(m.containerIDs, ipconfig.InfraContainerID)
	m.podInterfaceIDs = append(m.podInterfaceIDs, ipconfig.PodInterfaceID)
	return nil
}

func TestReleaseAll(t *testing.T) {
	// Create a temporary directory for tests
	tempDir, err := os.MkdirTemp("", "fsnotify-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create test files
	validContainerID := "valid-container"
	validPodInterfaceID := "valid-pod-interface"
	err = AddFile(validPodInterfaceID, validContainerID, tempDir)
	require.NoError(t, err)

	// Create a directory with the name of a containerID to simulate "file is a directory" error
	invalidContainerID := "invalid-container"
	err = os.Mkdir(tempDir+"/"+invalidContainerID, 0755)
	require.NoError(t, err)

	// Create a test watcher
	mockClient := &mockReleaseIPsClient{}
	logger, _ := zap.NewDevelopment()
	w := &watcher{
		cli:           mockClient,
		path:          tempDir,
		log:           logger,
		pendingDelete: map[string]struct{}{},
	}

	// Add valid and invalid containerIDs to pendingDelete
	w.pendingDelete[validContainerID] = struct{}{}
	w.pendingDelete[invalidContainerID] = struct{}{}

	// Test releaseAll
	w.releaseAll(context.Background())

	// Verify that ReleaseIPs was called only for the valid container
	assert.True(t, mockClient.releaseIPCalled)
	assert.Equal(t, []string{validContainerID}, mockClient.containerIDs)
	assert.Equal(t, []string{validPodInterfaceID}, mockClient.podInterfaceIDs)

	// Verify that only the valid container was removed from pendingDelete
	_, validExists := w.pendingDelete[validContainerID]
	assert.False(t, validExists, "valid container should be removed from pendingDelete")

	_, invalidExists := w.pendingDelete[invalidContainerID]
	assert.True(t, invalidExists, "invalid container should still be in pendingDelete")
}

func TestAddFile(t *testing.T) {
	// Create a temporary directory for tests
	tempDir, err := os.MkdirTemp("", "fsnotify-addfile-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	type args struct {
		podInterfaceID string
		containerID    string
		path           string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "no such directory, add fail",
			args: args{
				podInterfaceID: "123",
				containerID:    "67890",
				path:           tempDir + "/nonexistent",
			},
			wantErr: true,
		},
		{
			name: "added file to directory",
			args: args{
				podInterfaceID: "345",
				containerID:    "12345",
				path:           tempDir,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := AddFile(tt.args.podInterfaceID, tt.args.containerID, tt.args.path); (err != nil) != tt.wantErr {
				t.Errorf("WatcherAddFile() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWatcherRemoveFile(t *testing.T) {
	// Create a temporary directory for tests
	tempDir, err := os.MkdirTemp("", "fsnotify-removefile-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	type args struct {
		containerID string
		path        string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "remove file fail",
			args: args{
				containerID: "12345",
				path:        tempDir + "/nonexistent",
			},
			wantErr: true,
		},
		{
			name: "remove existing file",
			args: args{
				containerID: "67890",
				path:        tempDir,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "remove existing file" {
				// Create the file to be removed
				err := AddFile("test", tt.args.containerID, tt.args.path)
				require.NoError(t, err)
			}

			if err := removeFile(tt.args.containerID, tt.args.path); (err != nil) != tt.wantErr {
				t.Errorf("WatcherRemoveFile() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
