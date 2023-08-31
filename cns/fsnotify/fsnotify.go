package fsnotify

import (
	"context"
	"io/fs"
	"os"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/client"
	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

type Watcher struct {
	CnsClient *client.Client
}

func WatchFs(w *Watcher, path, directory string, logger *zap.Logger) error {
	// Create new fs watcher.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Error("Error creating watcher: ", zap.Error(err))
	}
	defer watcher.Close()

	// Start listening for events.
	logger.Info("Listening for events from watcher")
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				logger.Info("Watcher: ", zap.Any("event: ", event.Name))
				if event.Has(fsnotify.Create) {
					logger.Info("created file: ", zap.Any("event: ", event.Name))
					w.releaseIP(event.Name, path, directory, logger)
				}
				if event.Has(fsnotify.Remove) {
					logger.Info("removed file: ", zap.Any("event: ", event.Name))
				}
			case watcherErr, ok := <-watcher.Errors:
				if !ok {
					return
				}
				logger.Error("Watcher Error: ", zap.Error(watcherErr))
			}
		}
	}()

	// Add directory where intended deletes are kept
	dirpath := path + directory
	err = os.MkdirAll(dirpath, 0o755) //nolint
	if err != nil {
		logger.Error("Error making directory: ", zap.Error(err))
	}
	err = watcher.Add(dirpath)
	if err != nil {
		logger.Error("Watcher add directory error: ", zap.Error(err))
	}

	// list the directory then call ReleaseIPs
	logger.Info("Listing directory deleteIDs: ")
	dirContents, err := os.ReadDir(dirpath)
	if err != nil {
		logger.Error("Error reading deleteID directory", zap.Error(err))
	} else {
		for _, file := range dirContents {
			logger.Info("File to be deleted: ", zap.String("name", file.Name()))
			logger.Info("Path to be removed from: ", zap.String("path: ", path))
			w.releaseIP(file.Name(), path, directory, logger)
		}
	}

	return nil
}

// WatcherAddFile creates new file using the containerID as name
func WatcherAddFile(containerID, path, directory string) error {
	dirpath := path + directory
	_, err := os.Stat(dirpath)
	if errors.Is(err, fs.ErrNotExist) {
		return errors.Wrapf(err, "Error reading directory")
	}

	filepath := dirpath + "/" + containerID
	f, err := os.Create(filepath)
	if err != nil {
		return errors.Wrapf(err, "Error creating file")
	}
	defer f.Close()
	return nil
}

// WatcherRemoveFile removes the file based on containerID
func WatcherRemoveFile(containerID, path, directory string) error {
	dirpath := path + directory
	_, err := os.Stat(dirpath)
	if errors.Is(err, fs.ErrNotExist) {
		return errors.Wrapf(err, "Error reading directory")
	}

	filepath := dirpath + "/" + containerID

	// check the file exists
	_, fileExists := os.Stat(filepath)
	if errors.Is(fileExists, fs.ErrNotExist) {
		return nil
	}

	file, err := os.Open(filepath)
	if err != nil {
		return errors.Wrapf(err, "Error opening file")
	}

	if err := os.Remove(filepath); err != nil {
		return errors.Wrapf(err, "Error deleting file")
	}
	file.Close()
	return nil
}

// call cns ReleaseIPs
func (w *Watcher) releaseIP(containerID, path, directory string, logger *zap.Logger) {
	ipconfigreq := &cns.IPConfigsRequest{InfraContainerID: containerID}

	cnsReleaseErr := w.CnsClient.ReleaseIPs(context.Background(), *ipconfigreq)
	if cnsReleaseErr != nil {
		logger.Error("failed to release IP from watcher directory")
	}

	err := WatcherRemoveFile(containerID, path, directory)
	if err != nil {
		logger.Error("Failed to remove file: ", zap.Error(err))
	}
}
