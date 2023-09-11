package fsnotify

import (
	"context"
	"os"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

type cnsclient interface {
	ReleaseIPs(ctx context.Context, ipconfig cns.IPConfigsRequest) error
}

type Watcher struct {
	CnsClient cnsclient
	Path      string
	Logger    *zap.Logger
}

// WatchFS starts the filesystem watcher to handle async Pod deletes.
// Blocks until the context is closed; returns underlying fsnotify errors
// if something goes fatally wrong.
func (w *Watcher) WatchFs(ctx context.Context) error {
	// Create new fs watcher.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return errors.Wrap(err, "error creating watcher")
	}
	defer watcher.Close()

	c, cancel := context.WithCancel(ctx)
	// Start listening for events.
	w.Logger.Info("listening for events from watcher")
	go func() {
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				w.Logger.Info("received event", zap.String("event", event.Name))
				if event.Has(fsnotify.Create) {
					w.Logger.Info("file created, triggering release", zap.String("event", event.Name))
					cnsReleaseErr := w.releaseIP(ctx, event.Name)
					if cnsReleaseErr != nil {
						w.Logger.Error("failed to release IP from CNS", zap.Error(cnsReleaseErr))
					}
					deleteErr := RemoveFile(event.Name, w.Path)
					if deleteErr != nil {
						w.Logger.Error("failed to remove file", zap.Error(err))
					}
				}
				if event.Has(fsnotify.Remove) {
					w.Logger.Info("file deleted", zap.String("event", event.Name))
				}
			case watcherErr, ok := <-watcher.Errors:
				if !ok {
					return
				}
				w.Logger.Error("watcher error", zap.Error(watcherErr))
			}
		}
	}()

	// Add directory where intended deletes are kept
	err = os.Mkdir(w.Path, 0o755) //nolint
	if err != nil {
		w.Logger.Error("error making directory", zap.Error(err))
	}
	err = watcher.Add(w.Path)
	if err != nil {
		w.Logger.Error("watcher add directory error", zap.Error(err))
	}

	// list the directory then call ReleaseIPs
	w.Logger.Info("listing directory deleteIDs")
	dirContents, err := os.ReadDir(w.Path)
	if err != nil {
		w.Logger.Error("error reading deleteID directory", zap.Error(err))
	} else {
		for _, file := range dirContents {
			w.Logger.Info("file to be deleted", zap.String("name", file.Name()))
			cnsReleaseErr := w.releaseIP(ctx, file.Name())
			if cnsReleaseErr != nil {
				w.Logger.Error("failed to release IP from CNS", zap.Error(cnsReleaseErr))
			}
			err := RemoveFile(file.Name(), w.Path)
			if err != nil {
				w.Logger.Error("failed to remove file", zap.Error(err))
			}
		}
	}

	<-c.Done()
	return errors.Wrap(c.Err(), "error watching directory")
}

// AddFile creates new file using the containerID as name
func AddFile(containerID, path string) error {
	filepath := path + "/" + containerID
	f, err := os.Create(filepath)
	if err != nil {
		return errors.Wrap(err, "error creating file")
	}
	return errors.Wrap(f.Close(), "error adding file to directory")
}

// RemoveFile removes the file based on containerID
func RemoveFile(containerID, path string) error {
	filepath := path + "/" + containerID

	if err := os.Remove(filepath); err != nil {
		return errors.Wrap(err, "error deleting file")
	}

	return nil
}

// call cns ReleaseIPs
func (w *Watcher) releaseIP(ctx context.Context, containerID string) error {
	ipconfigreq := &cns.IPConfigsRequest{InfraContainerID: containerID}
	return errors.Wrap(w.CnsClient.ReleaseIPs(ctx, *ipconfigreq), "error releasing IP from CNS")
}
