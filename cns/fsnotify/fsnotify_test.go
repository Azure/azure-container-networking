package fsnotify

import (
	"testing"

	"github.com/Azure/azure-container-networking/cns/client"
	"go.uber.org/zap"
)

func TestWatchFs(t *testing.T) {
	type args struct {
		w *Watcher
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := WatchFs(tt.args.w); (err != nil) != tt.wantErr {
				t.Errorf("WatchFs() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWatcherAddFile(t *testing.T) {
	type args struct {
		containerID string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := WatcherAddFile(tt.args.containerID); (err != nil) != tt.wantErr {
				t.Errorf("WatcherAddFile() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWatcherRemoveFile(t *testing.T) {
	type args struct {
		containerID string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := WatcherRemoveFile(tt.args.containerID); (err != nil) != tt.wantErr {
				t.Errorf("WatcherRemoveFile() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWatcher_releaseIP(t *testing.T) {
	type fields struct {
		CnsClient *client.Client
		logger    *zap.Logger
	}
	type args struct {
		containerID string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &Watcher{
				CnsClient: tt.fields.CnsClient,
				logger:    tt.fields.logger,
			}
			w.releaseIP(tt.args.containerID)
		})
	}
}
