package fsnotify

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWatcherAddFile(t *testing.T) {
	type args struct {
		containerID string
		path        string
		directory   string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "no such directory, add fail",
			args: args{
				containerID: "67890",
				path:        "/bad/path",
				directory:   "",
			},
			wantErr: true,
		},
		{
			name: "added file to directory",
			args: args{
				containerID: "12345",
				path:        "/path/we/want",
				directory:   "/dir",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := os.MkdirAll("/path/we/want/dir", 0o777)
			require.NoError(t, err)
			if err := WatcherAddFile(tt.args.containerID, tt.args.path, tt.args.directory); (err != nil) != tt.wantErr {
				t.Errorf("WatcherAddFile() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWatcherRemoveFile(t *testing.T) {
	type args struct {
		containerID string
		path        string
		directory   string
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
				path:        "/bad/path",
				directory:   "",
			},
			wantErr: true,
		},
		{
			name: "no such directory, add fail",
			args: args{
				containerID: "67890",
				path:        "/path/we/want",
				directory:   "/dir",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := os.MkdirAll("/path/we/want/dir/67890", 0o777)
			require.NoError(t, err)
			if err := WatcherRemoveFile(tt.args.containerID, tt.args.path, tt.args.directory); (err != nil) != tt.wantErr {
				t.Errorf("WatcherRemoveFile() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
