package fsnotify

import (
	"context"
	"io/fs"
	"os"

	"github.com/Azure/azure-container-networking/azure-ipam/logger"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/client"
	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

const (
	directory = "/deleteIDs"
)

type Watcher struct {
	CnsClient *client.Client
	logger    *zap.Logger
}

func WatchFs(w *Watcher, path string) error {
	loggerCfg := &logger.Config{
		Level:       "debug",
		Filepath:    "/var/log/fsnotify-watcher.log",
		MaxSizeInMB: 5, // MegaBytes
		MaxBackups:  8,
	}
	// Create logger
	fsnotifyLogger, cleanup, err := logger.New(loggerCfg)
	if err != nil {
		return errors.Wrapf(err, "failed to setup fsnotify logging")
	}
	fsnotifyLogger.Debug("logger construction succeeded")
	w.logger = fsnotifyLogger
	defer cleanup()

	// Create new fs watcher.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		w.logger.Error("Error creating watcher: ", zap.Error(err))
	}
	defer watcher.Close()

	// Start listening for events.
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				w.logger.Info("Watcher: ", zap.Any("event: ", event.Name))
				if event.Has(fsnotify.Create) {
					w.logger.Info("created file: ", zap.Any("event: ", event.Name))
					w.releaseIP(event.Name, path)
				}
				if event.Has(fsnotify.Remove) {
					w.logger.Info("removed file: ", zap.Any("event: ", event.Name))
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				w.logger.Error("Watcher Error: ", zap.Error(err))
			}
		}
	}()

	// Add directory where intended deletes are kept
	dirpath := path + "/" + directory
	err = os.MkdirAll(dirpath, 0o755)
	if err != nil {
		w.logger.Error("Error making directory: ", zap.Error(err))
	}
	err = watcher.Add(dirpath)
	if err != nil {
		w.logger.Error("Watcher add directory error: ", zap.Error(err))
	}

	// list the directory then call ReleaseIPs
	w.logger.Info("Listing directory deleteIDs: ")
	dirContents, err := os.ReadDir(dirpath)
	if err != nil {
		w.logger.Error("Error reading deleteID directory", zap.Error(err))
	} else {
		w.logger.Info("TESTWATCHER: files exist in the directory")
		for _, file := range dirContents {
			w.logger.Info("TESTWATCHER: release IPs from directory")
			w.logger.Info("File to be deleted: ", zap.String("name", file.Name()))
			w.logger.Info("TESTWATCHER: path to be removed from: ", zap.String("path: ", path))
			w.releaseIP(file.Name(), path)
		}
	}

	return nil
}

// WatcherAddFile creates new file using the containerID as name
func WatcherAddFile(containerID string, path string) error {
	dirpath := path + "/" + directory
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
func WatcherRemoveFile(containerID string, path string) error {
	dirpath := path + "/" + directory
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
func (w *Watcher) releaseIP(containerID string, path string) {
	ipconfigreq := &cns.IPConfigsRequest{InfraContainerID: containerID}
	w.CnsClient.ReleaseIPs(context.Background(), *ipconfigreq)

	err := WatcherRemoveFile(containerID, path)
	if err != nil {
		w.logger.Error("Failed to remove file: ", zap.Error(err))
	} else {
		w.logger.Info("Removed file")
	}
}
