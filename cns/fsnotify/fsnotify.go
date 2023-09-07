package fsnotify

import (
	"context"
	"io/fs"
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
}

func WatchFs(w *Watcher, path string, logger *zap.Logger) error {
	// Create new fs watcher.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Error("error creating watcher", zap.Error(err))
	}
	defer watcher.Close()

	// Start listening for events.
	logger.Info("listening for events from watcher")
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				logger.Info("received event", zap.String("event", event.Name))
				if event.Has(fsnotify.Create) {
					logger.Info("file created, triggering release", zap.String("event", event.Name))
					cnsReleaseErr := w.releaseIP(event.Name)
					if cnsReleaseErr != nil {
						logger.Error("failed to release IP from CNS", zap.Error(cnsReleaseErr))
					}
					deleteErr := RemoveFile(event.Name, path)
					if deleteErr != nil {
						logger.Error("failed to remove file", zap.Error(err))
					}
				}
				if event.Has(fsnotify.Remove) {
					logger.Info("file deleted", zap.String("event", event.Name))
				}
			case watcherErr, ok := <-watcher.Errors:
				if !ok {
					return
				}
				logger.Error("watcher error", zap.Error(watcherErr))
			}
		}
	}()

	// Add directory where intended deletes are kept
	err = os.MkdirAll(path, 0o755) //nolint
	if err != nil {
		logger.Error("error making directory", zap.Error(err))
	}
	err = watcher.Add(path)
	if err != nil {
		logger.Error("watcher add directory error", zap.Error(err))
	}

	// list the directory then call ReleaseIPs
	logger.Info("listing directory deleteIDs")
	dirContents, err := os.ReadDir(path)
	if err != nil {
		logger.Error("error reading deleteID directory", zap.Error(err))
	} else {
		for _, file := range dirContents {
			logger.Info("file to be deleted", zap.String("name", file.Name()))
			cnsReleaseErr := w.releaseIP(file.Name())
			if cnsReleaseErr != nil {
				logger.Error("failed to release IP from CNS", zap.Error(cnsReleaseErr))
			}
			err := RemoveFile(file.Name(), path)
			if err != nil {
				logger.Error("failed to remove file", zap.Error(err))
			}
		}
	}

	return nil
}

// AddFile creates new file using the containerID as name
func AddFile(containerID, path string) error {
	_, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return errors.Wrapf(err, "error reading directory, directory must already exist")
	}

	filepath := path + "/" + containerID
	f, err := os.Create(filepath)
	if err != nil {
		return errors.Wrapf(err, "error creating file")
	}
	f.Close()
	return nil
}

// RemoveFile removes the file based on containerID
func RemoveFile(containerID, path string) error {
	_, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return errors.Wrapf(err, "error reading directory, directory must already exist")
	}

	filepath := path + "/" + containerID

	// check the file exists
	_, fileExists := os.Stat(filepath)
	if errors.Is(fileExists, fs.ErrNotExist) {
		return nil
	}

	file, err := os.Open(filepath)
	if err != nil {
		return errors.Wrapf(err, "error opening file")
	}

	if err := os.Remove(filepath); err != nil {
		return errors.Wrapf(err, "error deleting file")
	}
	file.Close()
	return nil
}

// call cns ReleaseIPs
func (w *Watcher) releaseIP(containerID string) error {
	ipconfigreq := &cns.IPConfigsRequest{InfraContainerID: containerID}

	cnsReleaseErr := w.CnsClient.ReleaseIPs(context.Background(), *ipconfigreq)
	if cnsReleaseErr != nil {
		return errors.Wrapf(cnsReleaseErr, "error releasing IP from CNS")
	}
	return nil
}
