package fsnotify

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAddFile(t *testing.T) {
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
				path:           "bad/path",
			},
			wantErr: true,
		},
		{
			name: "added file to directory",
			args: args{
				podInterfaceID: "345",
				containerID:    "12345",
				path:           "path/we/want",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseDir := t.TempDir()

			dirToCreate := filepath.Join(baseDir, "path", "we", "want")
			err := os.MkdirAll(dirToCreate, 0o777)
			require.NoError(t, err)

			fullPath := filepath.Join(baseDir, tt.args.path)

			if err := AddFile(tt.args.podInterfaceID, tt.args.containerID, fullPath); (err != nil) != tt.wantErr {
				t.Errorf("WatcherAddFile() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWatcherRemoveFile(t *testing.T) {
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
				path:        "bad/path",
			},
			wantErr: true,
		},
		{
			name: "no such directory, add fail",
			args: args{
				containerID: "67890",
				path:        "path/we/want",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseDir := t.TempDir()

			dirToCreate := filepath.Join(baseDir, "path", "we", "want", "67890")
			err := os.MkdirAll(dirToCreate, 0o777)
			require.NoError(t, err)

			fullPath := filepath.Join(baseDir, tt.args.path)

			if err := removeFile(tt.args.containerID, fullPath); (err != nil) != tt.wantErr {
				t.Errorf("WatcherRemoveFile() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
