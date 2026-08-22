package fsnotify

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type mockReleaseIPsClient struct {
	requests []cns.IPConfigsRequest
}

func (m *mockReleaseIPsClient) ReleaseIPs(_ context.Context, request cns.IPConfigsRequest) error {
	m.requests = append(m.requests, request)
	return nil
}

func TestWatcherReleaseAll(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(*testing.T, string, string)
		wantRelease bool
	}{
		{
			name: "releases readable file",
			setup: func(t *testing.T, path, containerID string) {
				require.NoError(t, AddFile("pod-interface-id", containerID, path))
			},
			wantRelease: true,
		},
		{
			name:  "retains missing file",
			setup: func(*testing.T, string, string) {},
		},
		{
			name: "retains unreadable directory",
			setup: func(t *testing.T, path, containerID string) {
				require.NoError(t, os.Mkdir(filepath.Join(path, containerID), 0o755))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := t.TempDir()
			containerID := "container-id"
			tt.setup(t, path, containerID)

			client := &mockReleaseIPsClient{}
			w := &watcher{
				cli:           client,
				path:          path,
				log:           zap.NewNop(),
				pendingDelete: map[string]struct{}{containerID: {}},
			}

			w.releaseAll(context.Background())

			if tt.wantRelease {
				require.Equal(t, []cns.IPConfigsRequest{{
					PodInterfaceID:   "pod-interface-id",
					InfraContainerID: containerID,
				}}, client.requests)
				_, err := os.Stat(filepath.Join(path, containerID))
				require.ErrorIs(t, err, os.ErrNotExist)
			} else {
				require.Empty(t, client.requests)
			}

			_, pending := w.pendingDelete[containerID]
			require.Equal(t, !tt.wantRelease, pending)
		})
	}
}

func TestAddFile(t *testing.T) {
	tests := []struct {
		name          string
		createDir     bool
		wantErr       bool
		wantFileValue string
	}{
		{
			name:    "fails when parent directory is missing",
			wantErr: true,
		},
		{
			name:          "writes pod interface ID",
			createDir:     true,
			wantFileValue: "pod-interface-id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state")
			if tt.createDir {
				require.NoError(t, os.Mkdir(path, 0o755))
			}

			err := AddFile("pod-interface-id", "container-id", path)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			data, err := os.ReadFile(filepath.Join(path, "container-id"))
			require.NoError(t, err)
			require.Equal(t, tt.wantFileValue, string(data))
		})
	}
}

func TestWatcherRemoveFile(t *testing.T) {
	tests := []struct {
		name       string
		createFile bool
		wantErr    bool
	}{
		{
			name:    "fails when file is missing",
			wantErr: true,
		},
		{
			name:       "removes existing file",
			createFile: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := t.TempDir()
			filePath := filepath.Join(path, "container-id")
			if tt.createFile {
				require.NoError(t, os.WriteFile(filePath, []byte("pod-interface-id"), 0o600))
			}

			err := removeFile("container-id", path)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			_, err = os.Stat(filePath)
			require.ErrorIs(t, err, os.ErrNotExist)
		})
	}
}
